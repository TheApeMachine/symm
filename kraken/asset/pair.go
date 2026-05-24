package asset

type Pair struct {
	Altname            string      `json:"altname"`
	Wsname             string      `json:"wsname"`
	AclassBase         string      `json:"aclass_base"`
	Base               string      `json:"base"`
	AclassQuote        string      `json:"aclass_quote"`
	Quote              string      `json:"quote"`
	Lot                string      `json:"lot"`
	CostDecimals       int         `json:"cost_decimals"`
	PairDecimals       int         `json:"pair_decimals"`
	LotDecimals        int         `json:"lot_decimals"`
	LotMultiplier      int         `json:"lot_multiplier"`
	LeverageBuy        []int       `json:"leverage_buy"`
	LeverageSell       []int       `json:"leverage_sell"`
	Fees               [][]float64 `json:"fees"`
	FeesMaker          [][]float64 `json:"fees_maker"`
	FeeVolumeCurrency  string      `json:"fee_volume_currency"`
	MarginCall         int         `json:"margin_call"`
	MarginStop         int         `json:"margin_stop"`
	Ordermin           string      `json:"ordermin"`
	Costmin            string      `json:"costmin"`
	TickSize           string      `json:"tick_size"`
	Status             string      `json:"status"`
	ExecutionVenue     string      `json:"execution_venue"`
	LongPositionLimit  int         `json:"long_position_limit"`
	ShortPositionLimit int         `json:"short_position_limit"`
}

func NewPair(base, quote string) *Pair {
	return &Pair{Base: base, Quote: quote}
}

/*
Symbol returns the websocket display name for one pair.
*/
func Symbol(pair Pair) string {
	if pair.Wsname != "" {
		return pair.Wsname
	}

	return pair.Altname
}
