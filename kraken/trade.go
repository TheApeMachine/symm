package kraken

import (
	"time"

	"github.com/bytedance/sonic"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/errnie"
)

type Trade struct {
	Channel string      `json:"channel"`
	Type    string      `json:"type"`
	Data    []TradeData `json:"data"`
}

type TradeData struct {
	Symbol    string          `json:"symbol"`
	Side      string          `json:"side"`
	Price     decimal.Decimal `json:"price"`
	Qty       float64         `json:"qty"`
	OrderType string          `json:"ord_type"`
	TradeID   int64           `json:"trade_id"`
	Timestamp time.Time       `json:"timestamp"`
}

func NewTrade(buf []byte) *Trade {
	var trade Trade

	if err := sonic.Unmarshal(buf, &trade); err != nil {
		errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"invalid trade",
			err,
		))
	}

	return &trade
}

func (trade *Trade) Action() string {
	return "trade"
}

type TradeSubscription struct {
	Pairs []string
}

func NewTradeSubscription(pairs []string) TradeSubscription {
	return TradeSubscription{Pairs: pairs}
}

func (subscription TradeSubscription) MarshalJSON() ([]byte, error) {
	return sonic.Marshal(map[string]any{
		"method": "subscribe",
		"params": map[string]any{
			"channel": "trade",
			"symbol":  subscription.Pairs,
		},
	})
}

type TradeVolume struct {
	Error  []string          `json:"error"`
	Result TradeVolumeResult `json:"result"`
}

type TradeVolumeResult struct {
	Currency   string                     `json:"currency"`
	AssetClass string                     `json:"asset_class"`
	Volume     string                     `json:"volume"`
	Inputs     TradeVolumeInputs          `json:"inputs"`
	Fees       map[string]TradeVolumeFees `json:"fees"`
	FeesMaker  map[string]TradeVolumeFees `json:"fees_maker"`
}

type TradeVolumeInputs struct {
	DomainSpotVolume30D    string `json:"domain_spot_volume_30d"`
	DomainFuturesVolume30D string `json:"domain_futures_volume_30d"`
	DomainAssetsOnPlatform string `json:"domain_assets_on_platform"`
}

type TradeVolumeFees struct {
	Fee        string `json:"fee"`
	MinFee     string `json:"min_fee"`
	MaxFee     string `json:"max_fee"`
	NextFee    string `json:"next_fee"`
	TierVolume string `json:"tier_volume"`
	NextVolume string `json:"next_volume"`
}

func NewTradeVolume(buf []byte) *TradeVolume {
	var tradeVolume TradeVolume

	if err := sonic.Unmarshal(buf, &tradeVolume); err != nil {
		errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"invalid trade volume",
			err,
		))

		return nil
	}

	return &tradeVolume
}

func (tradeVolume *TradeVolume) Action() string {
	return "trade_volume"
}

func (tradeVolume *TradeVolume) IsSuccess() bool {
	return len(tradeVolume.Error) == 0
}

type TradeVolumeRequest struct {
	Pair             []TradeVolumeRequestPair `json:"pair"`
	FeeInfo          bool                     `json:"fee-info"`
	RebaseMultiplier string                   `json:"rebase_multiplier"`
}

type TradeVolumeRequestPair struct {
	Asset  string `json:"asset"`
	Aclass string `json:"aclass"`
}

type TradeVolumeRequestPairs []TradeVolumeRequestPair

func NewTradeVolumeRequestPairs(symbols []string) TradeVolumeRequestPairs {
	pairs := make(TradeVolumeRequestPairs, len(symbols))

	for i, symbol := range symbols {
		pairs[i] = TradeVolumeRequestPair{
			Asset:  symbol,
			Aclass: "forex",
		}
	}

	return pairs
}

func NewTradeVolumeRequest(symbols []string) *TradeVolumeRequest {
	return &TradeVolumeRequest{
		Pair:             NewTradeVolumeRequestPairs(symbols),
		FeeInfo:          true,
		RebaseMultiplier: "rebased",
	}
}

func (request *TradeVolumeRequest) MarshalJSON() ([]byte, error) {
	type alias TradeVolumeRequest
	return sonic.Marshal((*alias)(request))
}
