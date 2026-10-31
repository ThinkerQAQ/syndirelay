package bridge

import (
	"strconv"
	"strings"
)

// mediumChromeMajor reads the major version from an actual browser session's
// User-Agent to keep related request headers internally consistent.
// This does not alter or impersonate the TLS fingerprint.
func mediumChromeMajor(userAgent string) int {
	const marker = "Chrome/"
	index := strings.Index(userAgent, marker)
	if index < 0 {
		return 0
	}
	value := userAgent[index+len(marker):]
	if dot := strings.IndexByte(value, '.'); dot >= 0 {
		value = value[:dot]
	}
	major, _ := strconv.Atoi(value)
	return major
}
