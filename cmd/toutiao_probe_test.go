package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestToutiaoHARReaderOnlyAcceptsCreatorInfoGet(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sample.har")
	har := `{"log":{"entries":[
      {"startedDateTime":"2026-10-09T11:00:00Z","request":{"method":"GET","url":"https://mp.toutiao.com/mp/agw/media/get_media_info","headers":[{"name":"Cookie","value":"sessionid=synthetic-old"},{"name":"User-Agent","value":"UA"}]}},
      {"startedDateTime":"2026-10-09T11:30:00Z","request":{"method":"GET","url":"https://other.example.com/mp/agw/media/get_media_info","headers":[{"name":"Cookie","value":"other=synthetic"},{"name":"User-Agent","value":"UA"}]}},
      {"startedDateTime":"2026-10-09T12:00:00Z","request":{"method":"GET","url":"https://mp.toutiao.com/mp/agw/media/get_media_info","headers":[{"name":"Cookie","value":"sessionid=synthetic-latest"},{"name":"User-Agent","value":"UA"}]}}
    ]}}`
	if err := os.WriteFile(path, []byte(har), 0600); err != nil {
		t.Fatal(err)
	}
	session, err := toutiaoSessionFromHAR(path)
	if err != nil {
		t.Fatal(err)
	}
	if session.RequestCookieHeader != "sessionid=synthetic-latest" || session.UserAgent != "UA" || session.CookieHostSuffixes[0] != "mp.toutiao.com" {
		t.Fatal("HAR did not select the freshest creator-scoped session")
	}
}

func TestToutiaoProbeRequiresExplicitHARPathAndExplicitWriteFlag(t *testing.T) {
	for _, args := range [][]string{{"probe"}, {"probe", "--har"}, {"probe", "--har", "bogus", "--unexpected"}} {
		err := runToutiaoProbe(args, os.Stdout)
		if err == nil {
			t.Fatalf("unexpected success for args %v", args)
		}
		if strings.Contains(err.Error(), "synthetic-latest") {
			t.Fatal("secrets must never be logged")
		}
	}
}
