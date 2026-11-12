package kraken

import (
	"fmt"
	"strings"

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
RateLimited reports EAPI rate-limit text nested in Kraken's heterogeneous error
slices so network backoff can defer nonce bumps and retries.
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
		default:
			if rateLimitedText(fmt.Sprint(entry)) {
				return true
			}
		}
	}

	return false
}

/*
rateLimitedText matches Kraken's English rate-limit substring case-insensitively.
*/
func rateLimitedText(line string) bool {
	lower := strings.ToLower(line)

	return strings.Contains(lower, "rate limit")
}
