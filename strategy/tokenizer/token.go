package tokenizer

import (
	"fmt"
	"strings"

	"github.com/bytedance/sonic"
	"github.com/theapemachine/symm/nomagique/store"
)

/*
Token is one MDL segment over the region observation stream. Sequitur
 supplies rule labels (M1, …) for the same span when digrams compressed.
*/
type Token struct {
	Symbols    []string    `json:"symbols"`
	Brightness float64     `json:"brightness"`
	Meta       SegmentMeta `json:"-"`
}

func NewToken(symbols []string, brightness float64, meta SegmentMeta) Token {
	if len(symbols) == 0 {
		return Token{}
	}

	out := make([]string, len(symbols))
	copy(out, symbols)

	return Token{
		Symbols:    out,
		Brightness: brightness,
		Meta:       meta,
	}
}

func TokenFromSegment(symbols []string, segment []*store.Region, meta SegmentMeta) Token {
	var brightness float64

	for _, region := range segment {
		if region == nil {
			continue
		}

		if region.Brightness > brightness {
			brightness = region.Brightness
		}
	}

	if len(symbols) == 0 {
		for _, region := range segment {
			if region != nil {
				symbols = append(symbols, RegionLabel(region.Key))
			}
		}
	}

	return NewToken(symbols, brightness, meta)
}

func (token Token) String() string {
	if len(token.Symbols) == 0 {
		return ""
	}

	if len(token.Symbols) == 1 {
		return token.Symbols[0]
	}

	return "[" + strings.Join(token.Symbols, "-") + "]"
}

type Tokens []Token

func (tokens Tokens) String() string {
	parts := make([]string, 0, len(tokens))
	for _, t := range tokens {
		if s := t.String(); s != "" {
			parts = append(parts, s)
		}
	}
	return strings.Join(parts, "/")
}

func (tokens Tokens) StringList() []string {
	out := make([]string, 0, len(tokens))
	for _, t := range tokens {
		out = append(out, t.String())
	}
	return out
}

func (token Token) MarshalJSON() ([]byte, error) {
	return sonic.Marshal(token.Symbols)
}

func (token *Token) UnmarshalJSON(data []byte) error {
	return sonic.Unmarshal(data, &token.Symbols)
}

func RegionLabel(key uint8) string {
	return fmt.Sprintf("R%02d", key)
}

// CutHasEnterBand reports whether the token names an enter-band region (R00–R02).
func CutHasEnterBand(token Token) bool {
	return cutHasBand(token, 0, 2)
}

// CutHasExitBand reports whether the token names an exit-band region (R03–R05).
func CutHasExitBand(token Token) bool {
	return cutHasBand(token, 3, 5)
}

func cutHasBand(token Token, lo, hi uint8) bool {
	for _, symbol := range token.Symbols {
		var key int

		if _, err := fmt.Sscanf(symbol, "R%d", &key); err != nil {
			continue
		}

		if key >= int(lo) && key <= int(hi) {
			return true
		}
	}

	return false
}
