package kraken

import (
	"strings"
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

type TradeVolumeInput struct {
	DomainSpotVolume30D    string `json:"domain_spot_volume_30d"`
	DomainAssetsOnPlatform string `json:"domain_assets_on_platform"`
	DomainFuturesVolume30D string `json:"domain_futures_volume_30d"`
}

type TradeVolumeFee struct {
	Fee               *decimal.Decimal `json:"fee"`
	Minfee            *decimal.Decimal `json:"minfee"`
	Maxfee            *decimal.Decimal `json:"maxfee"`
	Nextfee           *decimal.Decimal `json:"nextfee"`
	Tiervolume        string           `json:"tiervolume"`
	Nextvolume        string           `json:"nextvolume"`
	Nextfuturesvolume string           `json:"nextfuturesvolume"`
}

type TradeVolumeTier struct {
	MakerFee            *decimal.Decimal `json:"maker_fee"`
	TakerFee            *decimal.Decimal `json:"taker_fee"`
	Active              bool             `json:"active,omitempty"`
	MinSpotVolume       string           `json:"min_spot_volume,omitempty"`
	MinFuturesVolume    string           `json:"min_futures_volume,omitempty"`
	MinAssetsOnPlatform string           `json:"min_assets_on_platform,omitempty"`
}

type TradeVolumeSchedule struct {
	Pair  string            `json:"pair"`
	Class string            `json:"class"`
	Tiers []TradeVolumeTier `json:"tiers"`
}

type TradeVolumeResult struct {
	Currency   string                    `json:"currency"`
	AssetClass string                    `json:"asset_class"`
	Volume     string                    `json:"volume"`
	Inputs     TradeVolumeInput          `json:"inputs"`
	Fees       map[string]TradeVolumeFee `json:"fees"`
	FeesMaker  map[string]TradeVolumeFee `json:"fees_maker"`
	Schedules  []TradeVolumeSchedule     `json:"schedules"`
}

type TradeVolume struct {
	Error  []interface{}     `json:"error"`
	Result TradeVolumeResult `json:"result"`
}

func NewTradeVolume(buf []byte) *TradeVolumeResult {
	tradeVolume := &TradeVolume{}

	if err := sonic.Unmarshal(buf, tradeVolume); err != nil {
		errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"invalid trade volume",
			err,
		))

		return nil
	}

	return &tradeVolume.Result
}

func (tradeVolume *TradeVolume) Action() string {
	return "trade_volume"
}

func (tradeVolume *TradeVolume) IsSuccess() bool {
	return len(tradeVolume.Error) == 0
}

type TradeVolumeRequest struct {
	Pair        string `json:"pair"`
	FeeSchedule bool   `json:"fee_schedule"`
}

func NewTradeVolumeRequest(symbols []string) *TradeVolumeRequest {
	return &TradeVolumeRequest{
		Pair:        strings.Join(symbols, ","),
		FeeSchedule: true,
	}
}

func (request *TradeVolumeRequest) MarshalJSON() ([]byte, error) {
	type alias TradeVolumeRequest
	return sonic.Marshal((*alias)(request))
}
