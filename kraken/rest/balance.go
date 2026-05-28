package rest

/*
Balance holds spot wallet balances keyed by Kraken asset code.
*/
type Balance struct {
	Error  []string          `json:"error"`
	Result map[string]string `json:"result"`
}
