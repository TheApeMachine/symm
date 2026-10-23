package tokenizer

import (
	"fmt"
	"strings"

	"github.com/bytedance/sonic"
)

/*
Token is one variable-length cut over the Sequitur grammar stream, as
determined by MDL boundary detection. It is the unit the model accumulates
into a path and the unit that labels a trie edge.

A token's identity is its bracketed symbol sequence: [R01-R02] is a chord of
two region terminals; [M1-R03] is a rule reference followed by a terminal.
The brackets and hyphens make the token self-delimiting within a path joined
by "/" — a single trie edge is one token, never its constituent symbols.
*/
type Token struct {
	// Symbols are the grammar symbols in this cut, in order.
	Symbols []string

	// Confidence is the model's entry gate for this token, in [0, 1].
	// It combines the MDL boundary gain (how strong the structural cut was)
	// with the aggregate brightness of the regions the symbols cover.
	// A path with threshold T only triggers when the token's confidence >= T.
	Confidence float64

	// Brightness is the aggregate brightness of the grid regions that
	// contributed to this token's symbols. Carried for observability and
	// for the dashboard's token ribbon.
	Brightness float64

	// Meta carries the MDL boundary event tags for this cut.
	Meta SegmentMeta
}

// NewToken constructs a token from a sequence of grammar symbols, a confidence
// reading, and an aggregate brightness.
func NewToken(symbols []string, confidence, brightness float64, meta SegmentMeta) Token {
	if len(symbols) == 0 {
		return Token{}
	}
	return Token{
		Symbols:    symbols,
		Confidence: confidence,
		Brightness: brightness,
		Meta:       meta,
	}
}

// NewTokenFromRegion is the pre-grammar fast path: a single region token
// with no rule structure yet (the grammar has not yet grouped it). Used
// during the warm-up before the first MDL boundary fires.
func NewTokenFromRegion(key uint8, brightness float64) Token {
	sym := fmt.Sprintf("R%02d", key)
	return Token{
		Symbols:    []string{sym},
		Confidence: brightness,
		Brightness: brightness,
	}
}

// String is the token's trie-edge label: a single symbol is bare (R01),
// multiple symbols are bracketed and hyphen-joined ([R01-R02-M3]).
func (token Token) String() string {
	if len(token.Symbols) == 0 {
		return ""
	}
	if len(token.Symbols) == 1 {
		return token.Symbols[0]
	}
	return "[" + strings.Join(token.Symbols, "-") + "]"
}

// Tokens is a sequence of tokens forming one symbol's accumulated path.
type Tokens []Token

// String joins the tokens with "/" to form the full path key.
func (tokens Tokens) String() string {
	parts := make([]string, 0, len(tokens))
	for _, t := range tokens {
		s := t.String()
		if s != "" {
			parts = append(parts, s)
		}
	}
	return strings.Join(parts, "/")
}

// StringList returns the token labels as a plain slice for the dashboard's
// CognitionMatch.tokens field.
func (tokens Tokens) StringList() []string {
	out := make([]string, 0, len(tokens))
	for _, t := range tokens {
		out = append(out, t.String())
	}
	return out
}

/*
MarshalJSON serialises the token as its symbol list — the wire shape the
dashboard reads for the token ribbon and strength gauge.
*/
func (token Token) MarshalJSON() ([]byte, error) {
	return sonic.Marshal(token.Symbols)
}

func (token *Token) UnmarshalJSON(data []byte) error {
	return sonic.Unmarshal(data, &token.Symbols)
}
