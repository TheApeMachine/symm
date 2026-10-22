package tokenizer

import (
	"fmt"
	"strings"

	"github.com/bytedance/sonic"
	"github.com/theapemachine/symm/nomagique/store"
)

type Token struct {
	Chord      []string
	Threshold  float64
	Brightness float64
}

func NewToken(region *store.Region) Token {
	return Token{
		Chord:      []string{fmt.Sprintf("R%02d", region.Key)},
		Threshold:  0.0,
		Brightness: region.Brightness,
	}
}

func (token Token) String() string {
	return strings.Join(token.Chord, "/")
}

/*
JSON is the wire shape the dashboard's training ingest reads: a token is
its path — the sentence is the joined paths — with the energy fields
carried for anything that needs them.
*/
func (token Token) MarshalJSON() ([]byte, error) {
	return sonic.Marshal(token.Chord)
}

func (token *Token) UnmarshalJSON(p []byte) error {
	return sonic.Unmarshal(p, &token.Chord)
}

type Tokens []Token

func NewTokens(regions []*store.Region) Tokens {
	tokens := make(Tokens, 0, len(regions))

	for _, region := range regions {
		tokens = append(tokens, NewToken(region))
	}

	return tokens
}

func (tokens Tokens) String() string {
	parts := make([]string, 0, len(tokens))

	for _, token := range tokens {
		parts = append(parts, token.String())
	}

	return strings.Join(parts, "/")
}
