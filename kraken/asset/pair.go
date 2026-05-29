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
TakerFeePctOr returns this pair's taker fee percent for a given 30-day traded
volume (in the pair's fee_volume_currency), reading Kraken's tiered Fees
schedule. When the schedule is absent -- e.g. the pair came from the WebSocket
instrument feed, which carries no fees, and the REST AssetPairs enrichment has
not populated it -- the supplied fallback is returned instead. Pass volume 0
for a small/paper account to get the bottom (highest) tier.
*/
func (pair Pair) TakerFeePctOr(volume, fallback float64) float64 {
	return feeAtVolume(pair.Fees, volume, fallback)
}

/*
MakerFeePctOr is TakerFeePctOr for the maker (resting limit) schedule.
*/
func (pair Pair) MakerFeePctOr(volume, fallback float64) float64 {
	return feeAtVolume(pair.FeesMaker, volume, fallback)
}

/*
feeAtVolume resolves a Kraken fee schedule -- a list of [volumeThreshold,
percent] rows sorted ascending by threshold -- to the percent that applies at
the given 30-day volume. It picks the last row whose threshold the volume has
reached. A malformed or empty schedule yields the fallback.
*/
func feeAtVolume(schedule [][]float64, volume, fallback float64) float64 {
	percent := fallback
	matched := false

	for _, row := range schedule {
		if len(row) < 2 {
			continue
		}

		if volume < row[0] {
			break
		}

		percent = row[1]
		matched = true
	}

	if !matched {
		return fallback
	}

	return percent
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
