package kraken

import (
	"fmt"
	"strings"
	"time"

	"github.com/bytedance/sonic"
	"github.com/shopspring/decimal"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/market"
)

type Trade struct {
	Channel  string      `json:"channel"`
	Type     string      `json:"type"`
	Data     []TradeData `json:"data"`
	onChange func(market.MarketType)
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

func NewTrade() *Trade {
	return &Trade{
		Channel: "trade",
		Type:    "update",
		Data:    make([]TradeData, 0),
	}
}

func (trade *Trade) Request() map[string]any {
	return map[string]any{
		"method": "subscribe",
		"params": map[string]any{
			"channel": "trade",
		},
	}
}

func (trade *Trade) OnChange(fn func(market.MarketType)) {
	trade.onChange = fn
}

func (trade *Trade) MarshalJSON() ([]byte, error) {
	type alias Trade
	return sonic.Marshal((*alias)(trade))
}

func (trade *Trade) UnmarshalJSON(data []byte) error {
	type alias Trade
	var decoded alias

	if err := sonic.Unmarshal(data, &decoded); err != nil {
		return err
	}

	onChange := trade.onChange
	*trade = Trade(decoded)
	trade.onChange = onChange

	if trade.Data == nil {
		trade.Data = make([]TradeData, 0)
	}

	if trade.onChange != nil {
		trade.onChange(trade)
	}

	return nil
}

func (trade *Trade) ToMarket(other market.MarketType) market.MarketType {
	book, ok := other.(*market.Trade)

	if !ok || book == nil || trade == nil {
		return other
	}

	rows := make([]*market.TradeData, 0, len(trade.Data))

	for _, row := range trade.Data {
		rows = append(rows, &market.TradeData{
			Symbol:    row.Symbol,
			Side:      market.Side(row.Side),
			Price:     row.Price,
			Qty:       decimal.NewFromFloat(row.Qty),
			OrderType: market.OrderType(row.OrderType),
			TradeID:   row.TradeID,
			Timestamp: row.Timestamp,
		})
	}

	return market.NewTrade(trade.Channel, trade.Type, rows).ToMarket(book)
}

func (trade *Trade) FromMarket(other market.MarketType) market.MarketType {
	book, ok := other.(*market.Trade)

	if !ok || book == nil || trade == nil {
		return trade
	}

	trade.Channel = book.Channel
	trade.Type = book.Type
	trade.Data = trade.Data[:0]

	for _, row := range book.Data {
		if row == nil {
			continue
		}

		qty, _ := row.Qty.Float64()
		trade.Data = append(trade.Data, TradeData{
			Symbol:    row.Symbol,
			Side:      string(row.Side),
			Price:     row.Price,
			Qty:       qty,
			OrderType: string(row.OrderType),
			TradeID:   row.TradeID,
			Timestamp: row.Timestamp,
		})
	}

	return trade
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
	Error  []any             `json:"error"`
	Result TradeVolumeResult `json:"result"`
}

func ParseTradeVolume(buf []byte) (*TradeVolumeResult, error) {
	tradeVolume := &TradeVolume{}

	if err := sonic.Unmarshal(buf, tradeVolume); err != nil {
		return nil, errnie.Err(
			errnie.UnprocessableContent,
			"invalid trade volume response",
			err,
		)
	}

	if len(tradeVolume.Error) > 0 {
		return nil, errnie.Err(
			errnie.UnprocessableContent,
			"kraken trade volume rejected",
			fmt.Errorf("%v", tradeVolume.Error),
		)
	}

	if len(tradeVolume.Result.Fees) == 0 {
		return nil, errnie.Err(
			errnie.UnprocessableContent,
			"kraken trade volume returned no pair fees (pair must be requested)",
			nil,
		)
	}

	return &tradeVolume.Result, nil
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

type TradeSubscription struct {
	Method string                  `json:"method"`
	Params TradeSubscriptionParams `json:"params"`
}

type TradeSubscriptionParams struct {
	Channel string   `json:"channel"`
	Symbol  []string `json:"symbol"`
}

func NewTradeSubscription(symbols []string) TradeSubscription {
	return TradeSubscription{
		Method: "subscribe",
		Params: TradeSubscriptionParams{
			Channel: "trade",
			Symbol:  symbols,
		},
	}
}

func NewTradeUnsubscription(symbols []string) TradeSubscription {
	return TradeSubscription{
		Method: "unsubscribe",
		Params: TradeSubscriptionParams{
			Channel: "trade",
			Symbol:  symbols,
		},
	}
}

func (subscription TradeSubscription) MarshalJSON() ([]byte, error) {
	type alias TradeSubscription
	return sonic.Marshal((*alias)(&subscription))
}
