package tokenizer

import (
	"strings"

	"github.com/bytedance/sonic"
)

type Token struct {
	Path       []string
	Threshold  float64
	Brightness float64
}

func NewToken(path []string, threshold float64, brightness float64) *Token {
	return &Token{
		Path:       path,
		Threshold:  threshold,
		Brightness: brightness,
	}
}

func (token *Token) String() string {
	return strings.Join(token.Path, "/")
}

/*
JSON is the wire shape the dashboard's training ingest reads: a token is
its path — the sentence is the joined paths — with the energy fields
carried for anything that needs them.
*/
func (token *Token) MarshalJSON() ([]byte, error) {
	return sonic.Marshal(token.Path)
}

func (token *Token) UnmarshalJSON(p []byte) error {
	return sonic.Unmarshal(p, &token.Path)
}

type Tokens []*Token

func (tokens Tokens) String() string {
	parts := make([]string, 0, len(tokens))

	for _, token := range tokens {
		parts = append(parts, token.String())
	}

	return strings.Join(parts, "/")
}
