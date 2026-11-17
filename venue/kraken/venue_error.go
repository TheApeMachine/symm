package kraken

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/bytedance/sonic"
)

/*
VenueErrors lifts Kraken REST error arrays from a raw body so private REST
pollers can branch on rate limit vs hard failure without re-parsing ad hoc.
*/
func VenueErrors(body []byte) []any {
	if len(body) == 0 {
		return nil
	}

	var frame struct {
		Error []any `json:"error"`
	}

	if err := sonic.Unmarshal(body, &frame); err != nil {
		return nil
	}

	return frame.Error
}

/*
RateLimited reports EAPI:Rate limit exceeded (counter past the account max).
*/
func RateLimited(errs ...any) bool {
	for _, entry := range errs {
		switch value := entry.(type) {
		case []any:
			if RateLimited(value...) {
				return true
			}
		case []string:
			for _, line := range value {
				if rateLimitedText(line) {
					return true
				}
			}
		case error:
			if value != nil && rateLimitedText(value.Error()) {
				return true
			}
		default:
			if rateLimitedText(fmt.Sprint(entry)) {
				return true
			}
		}
	}

	return false
}

func rateLimitedText(line string) bool {
	// REST call counter only — not EOrder:Rate limit exceeded (trading engine).
	return strings.Contains(strings.ToLower(line), "eapi:rate limit exceeded")
}

/*
ThrottleUntil reports EService: Throttled: <unix> and the time to retry after.
*/
func ThrottleUntil(errs ...any) (time.Time, bool) {
	for _, entry := range errs {
		switch value := entry.(type) {
		case []any:
			if until, ok := ThrottleUntil(value...); ok {
				return until, true
			}
		case []string:
			for _, line := range value {
				if until, ok := throttleUntilText(line); ok {
					return until, true
				}
			}
		case error:
			if value != nil {
				if until, ok := throttleUntilText(value.Error()); ok {
					return until, true
				}
			}
		default:
			if until, ok := throttleUntilText(fmt.Sprint(entry)); ok {
				return until, true
			}
		}
	}

	return time.Time{}, false
}

func throttleUntilText(line string) (time.Time, bool) {
	lower := strings.ToLower(line)
	idx := strings.Index(lower, "throttled:")

	if idx < 0 {
		return time.Time{}, false
	}

	rest := strings.TrimSpace(line[idx+len("throttled:"):])
	sec, err := strconv.ParseInt(rest, 10, 64)

	if err != nil || sec <= 0 {
		return time.Time{}, false
	}

	return time.Unix(sec, 0).UTC(), true
}
