package kraken

import (
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/system"
)

/*
TradeBalanceResult mirrors Kraken's private `/0/private/TradeBalance` result.
*/
type TradeBalanceResult struct {
	AvailableCash     *decimal.Decimal `json:"-"`
	NetFunding        *decimal.Decimal `json:"-"`
	FundingReason     string           `json:"-"`
	ValuationComplete *bool            `json:"-"`
	EquivalentBalance *decimal.Decimal `json:"eb"`
	TradeBalance      *decimal.Decimal `json:"tb"`
	MarginAmount      *decimal.Decimal `json:"m"`
	UnrealizedPnL     *decimal.Decimal `json:"n"`
	CostBasis         *decimal.Decimal `json:"c"`
	Valuation         *decimal.Decimal `json:"v"`
	Equity            *decimal.Decimal `json:"e"`
	FreeMargin        *decimal.Decimal `json:"mf"`
	MarginFreeOrders  *decimal.Decimal `json:"mfo,omitempty"`
	MarginLevel       *decimal.Decimal `json:"ml,omitempty"`
	UnexecutedValue   *decimal.Decimal `json:"uv,omitempty"`
}

type TradeBalance struct {
	Error  []string           `json:"error"`
	Result TradeBalanceResult `json:"result"`
}

/*
AssetAlias maps venue legacy currency prefixes to standard codes and vice versa.
*/
func AssetAlias(asset string) string {
	switch asset {
	case "USD":
		return "ZUSD"
	case "ZUSD":
		return "USD"
	case "EUR":
		return "ZEUR"
	case "ZEUR":
		return "EUR"
	case "GBP":
		return "ZGBP"
	case "ZGBP":
		return "GBP"
	case "CAD":
		return "ZCAD"
	case "ZCAD":
		return "CAD"
	case "JPY":
		return "ZJPY"
	case "ZJPY":
		return "JPY"
	case "BTC":
		return "XXBT"
	case "XXBT":
		return "BTC"
	case "XBT":
		return "BTC"
	case "ETH":
		return "XETH"
	case "XETH":
		return "ETH"
	default:
		return ""
	}
}

/*
ToMeasurement converts the TradeBalanceResult and associated wallet into a structured Measurement.
*/
func (tb *TradeBalanceResult) ToMeasurement(wallet *Balance, quote string, epochs ...int64) *data.Measurement {
	if tb == nil {
		return nil
	}

	epoch := system.Epoch.Load()

	if len(epochs) > 0 && epochs[0] > 0 {
		epoch = epochs[0]
	}

	now := time.Now().UTC()

	if epoch <= 0 {
		epoch = now.UnixNano()
	}

	meas := data.NewMeasurement(
		epoch,
		"balance",
		"balance",
		now,
		now,
		&data.StringEntry{
			Key:   "channel",
			Value: "balance",
		},
		&data.StringEntry{
			Key:   "quote",
			Value: quote,
		},
	)

	var metrics []*data.Metric

	if tb.UnrealizedPnL != nil {
		metrics = append(metrics, data.NewExactMetric(
			"unrealized",
			tb.UnrealizedPnL,
			data.UnitCurrency,
			data.TimescaleInstantaneous,
		))
	}

	if tb.Equity != nil {
		metrics = append(metrics, data.NewExactMetric(
			"equity",
			tb.Equity,
			data.UnitCurrency,
			data.TimescaleInstantaneous,
		))
	}

	var quoteCash *decimal.Decimal

	if wallet != nil {
		for _, entry := range wallet.Data {
			alias := AssetAlias(quote)

			if entry.Asset == quote || (alias != "" && entry.Asset == alias) {
				if entry.Available != nil {
					quoteCash = entry.Available
					break
				}

				quoteCash = entry.Balance
				break
			}
		}
	}

	if quoteCash == nil && tb.AvailableCash != nil {
		quoteCash = tb.AvailableCash
	}

	if quoteCash != nil {
		metrics = append(metrics, data.NewExactMetric(
			"cash",
			quoteCash,
			data.UnitCurrency,
			data.TimescaleInstantaneous,
		))
	}

	if wallet != nil {
		for _, entry := range wallet.Data {
			if entry.Available == nil {
				continue
			}

			alias := AssetAlias(quote)

			if entry.Asset == quote || (alias != "" && entry.Asset == alias) {
				continue
			}

			metrics = append(metrics, data.NewExactMetric(
				"balance:"+entry.Asset,
				entry.Available,
				data.UnitCurrency,
				data.TimescaleInstantaneous,
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
NewTradeBalanceResultFromMeasurement reconstructs a TradeBalanceResult from a Measurement.
*/
func NewTradeBalanceResultFromMeasurement(meas *data.Measurement) (*TradeBalanceResult, error) {
	if meas == nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation, "[kraken.trade_balance] cannot convert nil measurement to trade balance", nil,
		))
	}

	tb := &TradeBalanceResult{}

	if entry := data.Pull(meas.Read("unrealized")); entry != nil && entry.Metric != nil {
		tb.UnrealizedPnL = entry.Metric.Exact()
	}

	if entry := data.Pull(meas.Read("equity")); entry != nil && entry.Metric != nil {
		tb.Equity = entry.Metric.Exact()
	}

	if entry := data.Pull(meas.Read("cash")); entry != nil && entry.Metric != nil {
		tb.AvailableCash = entry.Metric.Exact()
	}

	return tb, nil
}
