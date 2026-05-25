package instrument

type Params struct {
	Channel  string `json:"channel"`
	Snapshot bool   `json:"snapshot"`
}

type Subscribe struct {
	Method string `json:"method"`
	Params any    `json:"params"`
}

func NewSubscribe() *Subscribe {
	return &Subscribe{
		Method: "subscribe",
		Params: Params{
			Channel:  "instrument",
			Snapshot: true,
		},
	}
}
