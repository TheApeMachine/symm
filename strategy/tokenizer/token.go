package tokenizer

import "github.com/theapemachine/nomagique/runtime"

/*
Token aliases runtime.Token so strategy code imports one tokenizer package for path cuts.
*/
type Token = runtime.Token
/*
Tokens aliases runtime.Tokens — the model envelope Paths field uses this shape.
*/
type Tokens = runtime.Tokens

/*
NewToken builds one path segment token; brightness lives on the tail for Predict.
*/
func NewToken(symbols []string, brightness float64) Token {
	return runtime.NewToken(symbols, brightness)
}

/*
NewTokens is the empty cut returned when grid action regions are dark this tick.
*/
func NewTokens() Tokens {
	return runtime.NewTokens()
}
