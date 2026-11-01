package tokenizer

import "github.com/theapemachine/nomagique/runtime"

type Token = runtime.Token
type Tokens = runtime.Tokens

func NewToken(symbols []string, brightness float64) Token {
	return runtime.NewToken(symbols, brightness)
}

func NewTokens() Tokens {
	return runtime.NewTokens()
}
