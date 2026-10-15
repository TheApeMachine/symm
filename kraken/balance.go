package kraken

import (
	"strings"
	"time"

	"github.com/bytedance/sonic"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/system"
)

/*
	PaperBalance is the native payload returned by `kraken paper balance --output json`.

It keeps the full paper wallet rows while exposing a direct Kraken-style decimal
map for callers that only need per-asset totals.
*/
type PaperBalance struct {
	Balances map[string]PaperBalanceData `json:"balances"`
	Mode     string                      `json:"mode"`
}

/*
PaperBalanceData stores the native paper wallet row so available, reserved, and
total amounts survive the CLI decode without float-to-string guesswork later.
*/
type PaperBalanceData struct {
	Available *decimal.Decimal `json:"available"`
	Reserved  *decimal.Decimal `json:"reserved"`
	Total     *decimal.Decimal `json:"total"`
}

type Balance struct {
	Channel   string        `json:"channel"`
	Data      []BalanceData `json:"data"`
	Type      string        `json:"type"`
	Sequence  int64         `json:"sequence"`
	Timestamp time.Time     `json:"timestamp"`
}

type BalanceData struct {
	Asset      string           `json:"asset"`
	AssetClass string           `json:"asset_class"`
	Balance    *decimal.Decimal `json:"balance"`
	Available  *decimal.Decimal `json:"available,omitempty"`
	Reserved   *decimal.Decimal `json:"reserved,omitempty"`
	Wallets    []Wallet         `json:"wallets"`
}

type Wallet struct {
	Type    string           `json:"type"`
	ID      string           `json:"id"`
	Balance *decimal.Decimal `json:"balance"`
}

/*
BalanceSubscription requests the authenticated wallet stream through the same
transport abstraction used by public market subscriptions.
*/
type BalanceSubscription struct {
	Token string
}

/*
NewBalanceSubscription binds the current authenticated websocket token.
*/
func NewBalanceSubscription(token string) BalanceSubscription {
	return BalanceSubscription{Token: token}
}

/*
MarshalJSON encodes Kraken's private balances subscription request.
*/
func (subscription BalanceSubscription) MarshalJSON() ([]byte, error) {
	return sonic.Marshal(map[string]any{
		"method": "subscribe",
		"params": map[string]any{
			"channel": "balances",
			"token":   subscription.Token,
		},
	})
}

/*
NewBalance decodes a streamed balances frame. An undecodable frame is an
error, never a partially filled wallet that would overwrite the account state.
*/
func NewBalance(buf []byte) (*Balance, error) {
	var balance Balance

	if err := sonic.Unmarshal(buf, &balance); err != nil {
		return nil, errnie.Err(
			errnie.UnprocessableContent,
			"[kraken.balance] invalid balances frame",
			err,
		)
	}

	return &balance, nil
}

func (balance *Balance) MarshalJSON() ([]byte, error) {
	type alias Balance
	return sonic.Marshal((*alias)(balance))
}

func (balance *Balance) Action() string {
	return "balance"
}

func (balance *Balance) IsSuccess() bool {
	return len(balance.Data) > 0
}

/*
NewPaperBalance decodes the native paper wallet payload into the same Balance
observation shape as the real REST endpoint, keeping each row's available,
reserved, and total amounts as separate Data fields.
*/
func NewPaperBalance(buf []byte) *Balance {
	var paper PaperBalance

	if err := sonic.Unmarshal(buf, &paper); err != nil {
		errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"invalid paper balance",
			err,
		))
	}

	out := Balance{
		Channel:   "balances",
		Data:      []BalanceData{},
		Type:      "snapshot",
		Timestamp: time.Now(),
	}

	for asset, row := range paper.Balances {
		out.Data = append(out.Data, BalanceData{
			Asset:      asset,
			AssetClass: "currency",
			Balance:    row.Total,
			Available:  row.Available,
			Reserved:   row.Reserved,
			Wallets: []Wallet{
				{
					Type:    "spot",
					ID:      "main",
					Balance: row.Total,
				},
			},
		})
	}

	return &out
}

var zero = decimal.NewFromFloat64(0)

/*
NewBalanceFromMap reshapes the venue's asset-to-total map into the Balance
observation consumed downstream, synthesizing one Data row per asset.
*/
func NewBalanceFromMap(model map[string]*decimal.Decimal) *Balance {
	out := Balance{
		Channel:   "balances",
		Data:      []BalanceData{},
		Type:      "snapshot",
		Sequence:  0,
		Timestamp: time.Now(),
	}

	for asset, amount := range model {
		out.Data = append(out.Data, BalanceData{
			Asset:      asset,
			AssetClass: "currency",
			Balance:    amount,
			Available:  amount,
			Reserved:   zero,
			Wallets: []Wallet{
				{
					Type:    "spot",
					ID:      "main",
					Balance: amount,
				},
			},
		})
	}

	return &out
}

func NewTradeBalanceFromMap(model datura.Map[any]) TradeBalanceResult {
	currentValue := decimal.NewFromFloat64(model["current_value"].(float64))
	unrealizedPnL := decimal.NewFromFloat64(model["unrealized_pnl"].(float64))
	tradeBalance := currentValue.Sub(unrealizedPnL)

	zero := decimal.NewFromInt64(0)
	var funding *decimal.Decimal

	if amount, found := model["starting_balance"].(float64); found {
		funding = decimal.NewFromFloat64(amount)
	}

	complete, known := model["valuation_complete"].(bool)

	var valuation *bool

	if known {
		valuation = &complete
	}

	return TradeBalanceResult{
		NetFunding:        funding,
		ValuationComplete: valuation,
		EquivalentBalance: currentValue,
		TradeBalance:      tradeBalance,
		MarginAmount:      zero,
		UnrealizedPnL:     unrealizedPnL,
		CostBasis:         zero,
		Valuation:         zero,
		Equity:            currentValue,
		FreeMargin:        currentValue,
		MarginFreeOrders:  currentValue,
		UnexecutedValue:   zero,
	}
}

/*
ToMeasurement converts the Balance into a structured Measurement
without any serialized raw shortcuts.
*/
func (balance *Balance) ToMeasurement(epochs ...int64) *data.Measurement {
	if balance == nil {
		return nil
	}

	epoch := system.Epoch.Load()

	if len(epochs) > 0 && epochs[0] > 0 {
		epoch = epochs[0]
	}

	now := time.Now().UTC()

	if !balance.Timestamp.IsZero() {
		now = balance.Timestamp.UTC()
	}

	if epoch <= 0 {
		epoch = now.UnixNano()
	}

	channel := balance.Channel

	if channel == "" {
		channel = "balances"
	}

	msgType := balance.Type

	if msgType == "" {
		msgType = "snapshot"
	}

	meas := data.NewMeasurement(
		epoch,
		msgType,
		channel,
		now,
		now,
		&data.StringEntry{
			Key:   "channel",
			Value: channel,
		},
		&data.StringEntry{
			Key:   "type",
			Value: msgType,
		},
	)

	var metrics []*data.Metric

	for _, row := range balance.Data {
		if row.Balance != nil {
			metrics = append(metrics, data.NewExactMetric(
				"balance:"+row.Asset, row.Balance, data.UnitCurrency, data.TimescaleInstantaneous,
			))
		}

		if row.Available != nil {
			metrics = append(metrics, data.NewExactMetric(
				"available:"+row.Asset, row.Available, data.UnitCurrency, data.TimescaleInstantaneous,
			))
		}

		if row.Reserved != nil {
			metrics = append(metrics, data.NewExactMetric(
				"reserved:"+row.Asset, row.Reserved, data.UnitCurrency, data.TimescaleInstantaneous,
			))
		}
	}

	if len(metrics) > 0 {
		meas.Write(metrics...)
	}

	meas.Finalize()
	return meas
}

/*
NewBalanceFromMeasurement reconstructs a Balance from a Measurement.
*/
func NewBalanceFromMeasurement(meas *data.Measurement) (*Balance, error) {
	if meas == nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation, "[kraken.balance] cannot convert nil measurement to balance", nil,
		))
	}

	bal := &Balance{
		Channel:   "balances",
		Type:      "snapshot",
		Timestamp: meas.At,
		Data:      []BalanceData{},
	}

	for entry := range meas.Meta() {
		if entry == nil {
			continue
		}

		if entry.Key == "channel" && entry.Value != "" {
			bal.Channel = entry.Value
		}

		if entry.Key == "type" && entry.Value != "" {
			bal.Type = entry.Value
		}
	}

	assetMap := make(map[string]*BalanceData)

	for entry := range meas.Read() {
		if entry == nil || entry.Metric == nil || entry.Metric.Exact() == nil {
			continue
		}

		key := entry.Key
		val := entry.Metric.Exact()

		if after, ok := strings.CutPrefix(key, "balance:"); ok {
			asset := after
			row, ok := assetMap[asset]

			if !ok {
				row = &BalanceData{Asset: asset, AssetClass: "currency"}
				assetMap[asset] = row
			}

			row.Balance = val
		}

		if after, ok := strings.CutPrefix(key, "available:"); ok {
			asset := after
			row, ok := assetMap[asset]

			if !ok {
				row = &BalanceData{Asset: asset, AssetClass: "currency"}
				assetMap[asset] = row
			}

			row.Available = val
		}

		if after, ok := strings.CutPrefix(key, "reserved:"); ok {
			asset := after
			row, ok := assetMap[asset]

			if !ok {
				row = &BalanceData{Asset: asset, AssetClass: "currency"}
				assetMap[asset] = row
			}

			row.Reserved = val
		}
	}

	for _, row := range assetMap {
		bal.Data = append(bal.Data, *row)
	}

	return bal, nil
}
