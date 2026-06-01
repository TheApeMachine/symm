package paper

import "context"

/*
TokenProvider satisfies balances subscribe auth for the simulated connection.
*/
type TokenProvider struct{}

func NewTokenProvider() *TokenProvider {
	return &TokenProvider{}
}

func (provider *TokenProvider) Token(context.Context) (string, error) {
	return "paper", nil
}
