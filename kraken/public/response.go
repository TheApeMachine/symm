package public

type Response struct {
	Error  []string      `json:"error"`
	Result any           `json:"result"`
}
