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

/*
Trade is the public trade websocket tape signal disruptors attach to — distinct
from private Execution fills on the desk tier.
*/
type Trade struct {
	Channel  string      `json:"channel"`
	Type     string      `json:"type"`
	Data     []TradeData `json:"data"`
	onChange func(market.MarketType)
}

/*
TradeData is one print on the public trade channel.
*/
type TradeData struct {
	Symbol    string          `json:"symbol"`
	Side      string          `json:"side"`
	Price     decimal.Decimal `json:"price"`
	Qty       float64         `json:"qty"`
	OrderType string          `json:"ord_type"`
	TradeID   int64           `json:"trade_id"`
	Timestamp time.Time       `json:"timestamp"`
}

/*
NewTrade prepares empty Data for the first public trade snapshot.
*/
func NewTrade() *Trade {
	return &Trade{
		Channel: "trade",
		Type:    "update",
		Data:    make([]TradeData, 0),
	}
}

/*
Request is the public subscribe envelope for the trade channel.
*/
func (trade *Trade) Request() map[string]any {
	return map[string]any{
		"method": "subscribe",
		"params": map[string]any{
			"channel": "trade",
		},
	}
}

/*
OnChange wires runtime envelope trade after each public decode.
*/
func (trade *Trade) OnChange(fn func(market.MarketType)) {
	trade.onChange = fn
}

/*
MarshalJSON aliases Trade so onChange stays off the wire.
*/
func (trade *Trade) MarshalJSON() ([]byte, error) {
	type alias Trade
	return sonic.Marshal((*alias)(trade))
}

/*
UnmarshalJSON preserves onChange and notifies disruptor pipeline after Data fills.
*/
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

/*
ToMarket projects prints into nomagique Trade for envelope fan-out.
*/
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

/*
FromMarket copies nomagique trade rows back for cache replay.
*/
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

/*
TradeVolumeInput is Kraken fee-tier volume accounting inputs on TradeVolume REST.
*/
type TradeVolumeInput struct {
	DomainSpotVolume30D    string `json:"domain_spot_volume_30d"`
	DomainAssetsOnPlatform string `json:"domain_assets_on_platform"`
	DomainFuturesVolume30D string `json:"domain_futures_volume_30d"`
}

/*
TradeVolumeFee is per-pair fee schedule row inside TradeVolume REST.
*/
type TradeVolumeFee struct {
	Fee               *decimal.Decimal `json:"fee"`
	Minfee            *decimal.Decimal `json:"minfee"`
	Maxfee            *decimal.Decimal `json:"maxfee"`
	Nextfee           *decimal.Decimal `json:"nextfee"`
	Tiervolume        string           `json:"tiervolume"`
	Nextvolume        string           `json:"nextvolume"`
	Nextfuturesvolume string           `json:"nextfuturesvolume"`
}

/*
TradeVolumeTier is one maker/taker tier threshold on the fee schedule.
*/
type TradeVolumeTier struct {
	MakerFee            *decimal.Decimal `json:"maker_fee"`
	TakerFee            *decimal.Decimal `json:"taker_fee"`
	Active              bool             `json:"active,omitempty"`
	MinSpotVolume       string           `json:"min_spot_volume,omitempty"`
	MinFuturesVolume    string           `json:"min_futures_volume,omitempty"`
	MinAssetsOnPlatform string           `json:"min_assets_on_platform,omitempty"`
}

/*
TradeVolumeSchedule groups tiers for one pair class on TradeVolume REST.
*/
type TradeVolumeSchedule struct {
	Pair  string            `json:"pair"`
	Class string            `json:"class"`
	Tiers []TradeVolumeTier `json:"tiers"`
}

/*
TradeVolumeResult is the decoded TradeVolume REST payload for fee estimation.
*/
type TradeVolumeResult struct {
	Currency   string                    `json:"currency"`
	AssetClass string                    `json:"asset_class"`
	Volume     string                    `json:"volume"`
	Inputs     TradeVolumeInput          `json:"inputs"`
	Fees       map[string]TradeVolumeFee `json:"fees"`
	FeesMaker  map[string]TradeVolumeFee `json:"fees_maker"`
	Schedules  []TradeVolumeSchedule     `json:"schedules"`
}

/*
TradeVolume is the Kraken REST envelope wrapping TradeVolumeResult.
*/
type TradeVolume struct {
	Error  []any             `json:"error"`
	Result TradeVolumeResult `json:"result"`
}

/*
ParseTradeVolume validates REST trade volume responses before desk fee math.
*/
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

/*
TradeVolumeRequest names pairs for fee_schedule on private TradeVolume REST.
*/
type TradeVolumeRequest struct {
	Pair        string `json:"pair"`
	FeeSchedule bool   `json:"fee_schedule"`
}

/*
NewTradeVolumeRequest joins symbols for one Kraken TradeVolume query.
*/
func NewTradeVolumeRequest(symbols []string) *TradeVolumeRequest {
	return &TradeVolumeRequest{
		Pair:        strings.Join(symbols, ","),
		FeeSchedule: true,
	}
}

/*
MarshalJSON aliases TradeVolumeRequest for signed REST bodies.
*/
func (request *TradeVolumeRequest) MarshalJSON() ([]byte, error) {
	type alias TradeVolumeRequest
	return sonic.Marshal((*alias)(request))
}

/*
TradeSubscription is the public ws subscribe envelope for trade prints.
*/
type TradeSubscription struct {
	Method string                  `json:"method"`
	Params TradeSubscriptionParams `json:"params"`
}

/*
TradeSubscriptionParams lists symbols on the trade channel subscribe.
*/
type TradeSubscriptionParams struct {
	Channel string   `json:"channel"`
	Symbol  []string `json:"symbol"`
}

/*
NewTradeSubscription builds public trade subscribe for signal ingress.
*/
func NewTradeSubscription(symbols []string) TradeSubscription {
	return TradeSubscription{
		Method: "subscribe",
		Params: TradeSubscriptionParams{
			Channel: "trade",
			Symbol:  symbols,
		},
	}
}

/*
NewTradeUnsubscription drops symbols from the public trade socket.
*/
func NewTradeUnsubscription(symbols []string) TradeSubscription {
	return TradeSubscription{
		Method: "unsubscribe",
		Params: TradeSubscriptionParams{
			Channel: "trade",
			Symbol:  symbols,
		},
	}
}

/*
MarshalJSON aliases TradeSubscription for dial-time sonic encode.
*/
func (subscription TradeSubscription) MarshalJSON() ([]byte, error) {
	type alias TradeSubscription
	return sonic.Marshal((*alias)(&subscription))
}
