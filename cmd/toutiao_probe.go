package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/ThinkerQAQ/ThinkerQAQ.github.io/tools/blogctl/publisher"
)

type toutiaoHAR struct {
	Log struct {
		Entries []struct {
			StartedDateTime string `json:"startedDateTime"`
			Request         struct {
				Method  string `json:"method"`
				URL     string `json:"url"`
				Headers []struct {
					Name  string `json:"name"`
					Value string `json:"value"`
				} `json:"headers"`
			} `json:"request"`
		} `json:"entries"`
	} `json:"log"`
}

func toutiaoSessionFromHAR(path string) (publisher.Session, error) {
	info, err := os.Stat(path)
	if err != nil {
		return publisher.Session{}, err
	}
	if info.IsDir() || info.Size() > 24<<20 {
		return publisher.Session{}, errors.New("HAR must be a file smaller than 24 MB")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return publisher.Session{}, err
	}
	var har toutiaoHAR
	if err = json.Unmarshal(data, &har); err != nil {
		return publisher.Session{}, errors.New("HAR JSON is invalid")
	}
	var bestTime string
	var best publisher.Session
	for _, entry := range har.Log.Entries {
		if entry.Request.Method != "GET" {
			continue
		}
		parsed, err := url.Parse(entry.Request.URL)
		if err != nil || parsed.Scheme != "https" || parsed.Hostname() != "mp.toutiao.com" ||
			parsed.Path != "/mp/agw/media/get_media_info" {
			continue
		}
		var cookie, ua string
		for _, header := range entry.Request.Headers {
			switch strings.ToLower(header.Name) {
			case "cookie":
				cookie = header.Value
			case "user-agent":
				ua = header.Value
			}
		}
		if cookie == "" || ua == "" || len(cookie) > 32768 || strings.ContainsAny(cookie, "\r\n") {
			continue
		}
		if best.RequestCookieHeader == "" || entry.StartedDateTime > bestTime {
			bestTime = entry.StartedDateTime
			best = publisher.Session{RequestCookieHeader: cookie, UserAgent: ua,
				CookieHostSuffixes: []string{"mp.toutiao.com"}}
		}
	}
	if best.RequestCookieHeader == "" {
		return publisher.Session{}, errors.New("HAR contains no captured authenticated Toutiao creator request")
	}
	return best, nil
}

func runToutiaoProbe(args []string, out io.Writer) error {
	if len(args) < 1 || args[0] != "probe" {
		return errors.New("usage: blogctl toutiao probe --har <path> [--confirm-create-draft]")
	}
	var harPath string
	confirm := false
	for i := 1; i < len(args); i++ {
		switch args[i] {
		case "--har":
			if i+1 >= len(args) {
				return errors.New("--har requires a file path")
			}
			harPath = args[i+1]
			i++
		case "--confirm-create-draft":
			confirm = true
		default:
			return fmt.Errorf("unknown Toutiao probe option %q", args[i])
		}
	}
	if harPath == "" {
		return errors.New("--har is required")
	}
	session, err := toutiaoSessionFromHAR(harPath)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	result, err := publisher.ProbeToutiaoHTTP(ctx, &http.Client{Timeout: 12 * time.Second}, session, confirm)
	// Output ONLY non-secret structured status. Never print cookies or CSRF tokens.
	encoded, _ := json.Marshal(result)
	fmt.Fprintln(out, string(encoded))
	if err != nil {
		return fmt.Errorf("Toutiao native HTTP probe failed: %w", err)
	}
	return nil
}
