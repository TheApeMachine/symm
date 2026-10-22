package tokenizer

import "strings"

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

type Tokens []*Token

func (tokens Tokens) String() string {
	parts := make([]string, 0, len(tokens))

	for _, token := range tokens {
		parts = append(parts, token.String())
	}

	return strings.Join(parts, "/")
}
