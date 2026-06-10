package market

import (
	"fmt"
	"strconv"
)

/*
Pair is one tradable market's REST metadata from GET /public/AssetPairs.
See https://docs.kraken.com/api/docs/rest-api/get-tradable-asset-pairs
*/
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

/*
AssetPairs is the /public/AssetPairs result keyed by Kraken internal pair name.

Each entry is the full trading contract for one market: naming and base/quote
assets, the volume-tiered maker and taker fee schedule, price/lot/cost decimals
with minimum order size and tick size, margin terms, and trading status. This is
the definitive source of what a pair actually costs and how it must be traded --
real fee tiers rather than assumptions, and the exact rounding and minimums every
order has to respect.
See https://docs.kraken.com/api/docs/rest-api/get-tradable-asset-pairs
*/
type AssetPairs map[string]*Pair

/*
PairByWsname returns the pair whose wsname matches the WebSocket v2 symbol (e.g. BTC/USD).
*/
func (pairs AssetPairs) PairByWsname(wsname string) (*Pair, error) {
	for _, pair := range pairs {
		if pair == nil {
			continue
		}

		if pair.Wsname == wsname {
			return pair, nil
		}
	}

	return nil, fmt.Errorf("kraken asset pairs: wsname %q not found", wsname)
}

/*
FeePercentAtVolume returns the fee percent for a tiered schedule at the given
30-day volume in the pair's fee_volume_currency.
*/
func FeePercentAtVolume(tiers [][]float64, volume float64) (float64, error) {
	if len(tiers) == 0 {
		return 0, fmt.Errorf("kraken asset pairs: fee schedule is empty")
	}

	if len(tiers[0]) < 2 {
		return 0, fmt.Errorf("kraken asset pairs: fee tier is malformed")
	}

	selected := tiers[0][1]

	for _, tier := range tiers {
		if len(tier) < 2 {
			return 0, fmt.Errorf("kraken asset pairs: fee tier is malformed")
		}

		if volume >= tier[0] {
			selected = tier[1]
		}
	}

	return selected, nil
}

/*
TakerFeePercent returns the published taker fee percent for one volume level.
*/
func (pair *Pair) TakerFeePercent(volume float64) (float64, error) {
	if pair == nil {
		return 0, fmt.Errorf("kraken asset pairs: nil pair")
	}

	return FeePercentAtVolume(pair.Fees, volume)
}

/*
MakerFeePercent returns the published maker fee percent for one volume level.
*/
func (pair *Pair) MakerFeePercent(volume float64) (float64, error) {
	if pair == nil {
		return 0, fmt.Errorf("kraken asset pairs: nil pair")
	}

	return FeePercentAtVolume(pair.FeesMaker, volume)
}

/*
TakerFeeRate returns the decimal taker fee rate (0.0026 for 0.26%).
*/
func (pair *Pair) TakerFeeRate(volume float64) (float64, error) {
	percent, err := pair.TakerFeePercent(volume)

	if err != nil {
		return 0, err
	}

	return percent / 100, nil
}

/*
MakerFeeRate returns the decimal maker fee rate.
*/
func (pair *Pair) MakerFeeRate(volume float64) (float64, error) {
	percent, err := pair.MakerFeePercent(volume)

	if err != nil {
		return 0, err
	}

	return percent / 100, nil
}

/*
TickSizeFloat parses the published tick size for one pair.
*/
func (pair *Pair) TickSizeFloat() (float64, error) {
	if pair == nil {
		return 0, fmt.Errorf("kraken asset pairs: nil pair")
	}

	if pair.TickSize == "" {
		return 0, fmt.Errorf("kraken asset pairs: tick size missing for %s", pair.Wsname)
	}

	tickSize, err := strconv.ParseFloat(pair.TickSize, 64)

	if err != nil {
		return 0, fmt.Errorf("kraken asset pairs: tick size for %s: %w", pair.Wsname, err)
	}

	if tickSize <= 0 {
		return 0, fmt.Errorf("kraken asset pairs: tick size must be positive for %s", pair.Wsname)
	}

	return tickSize, nil
}
