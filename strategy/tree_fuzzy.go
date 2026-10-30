package strategy

import "strings"

/*
pathSimilarity scores how closely two slash-separated path spellings align
(segment equality from the root). Used when no exact prefix is stored yet.
*/
func pathSimilarity(live, storedKey string) float64 {
	live = strings.TrimSuffix(strings.TrimSpace(live), "/")
	stored := strings.TrimSpace(storedKey)
	if i := strings.LastIndex(stored, "/"); i >= 0 {
		suffix := stored[i+1:]
		if suffix == "enter.bin" || suffix == "hold.bin" || suffix == "exit.bin" {
			stored = stored[:i]
		}
	}

	if live == "" || stored == "" {
		return 0
	}
	if live == stored {
		return 1
	}

	a := strings.Split(live, "/")
	b := strings.Split(stored, "/")
	maxLen := len(a)
	if len(b) > maxLen {
		maxLen = len(b)
	}
	if maxLen == 0 {
		return 0
	}

	matches := 0
	for i := 0; i < len(a) && i < len(b); i++ {
		if a[i] == b[i] {
			matches++
		}
	}
	return float64(matches) / float64(maxLen)
}
