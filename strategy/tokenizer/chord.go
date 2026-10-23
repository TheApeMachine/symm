package tokenizer

import "sort"

/*
ChordToken is the stable per-tick path edge: the sorted, deduplicated lit
region terminals for one symbol on one tick. Policy keys are built from
these edges, not from Sequitur digram accidents.
*/
func ChordToken(regionTokens []Token) Token {
	if len(regionTokens) == 0 {
		return Token{}
	}

	symbols := make([]string, 0, len(regionTokens))
	var brightness float64

	for _, region := range regionTokens {
		if region.Brightness > brightness {
			brightness = region.Brightness
		}

		for _, sym := range region.Symbols {
			symbols = append(symbols, sym)
		}
	}

	if len(symbols) == 0 {
		return Token{}
	}

	sort.Strings(symbols)
	deduped := symbols[:0]

	for i, sym := range symbols {
		if i == 0 || sym != symbols[i-1] {
			deduped = append(deduped, sym)
		}
	}

	if brightness < 0 {
		brightness = 0
	}

	if brightness > 1 {
		brightness = 1
	}

	return NewToken(deduped, brightness, brightness, 0)
}
