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

type Execution struct {
	Channel  string          `json:"channel"`
	Type     string          `json:"type"`
	Data     []ExecutionData `json:"data"`
	Sequence int             `json:"sequence"`
}

type ExecutionData struct {
	OrderID       string           `json:"order_id"`
	ClientOrderID string           `json:"cl_ord_id"`
	OrderUserref  int              `json:"order_userref"`
	ExecID        string           `json:"exec_id"`
	ExecType      string           `json:"exec_type"`
	TradeID       int              `json:"trade_id"`
	Symbol        string           `json:"symbol"`
	Side          string           `json:"side"`
	LastQty       *decimal.Decimal `json:"last_qty"`
	LastPrice     *decimal.Decimal `json:"last_price"`
	LiquidityInd  string           `json:"liquidity_ind"`
	Cost          *decimal.Decimal `json:"cost"`
	OrderType     string           `json:"order_type"`
	Timestamp     time.Time        `json:"timestamp"`
	OrderStatus   string           `json:"order_status"`
	CumQty        *decimal.Decimal `json:"cum_qty"`
	CumCost       *decimal.Decimal `json:"cum_cost"`
	AvgPrice      *decimal.Decimal `json:"avg_price"`
	FeeUsdEquiv   *decimal.Decimal `json:"fee_usd_equiv"`
	Fees          []ExecutionFee   `json:"fees"`
}

type ExecutionFee struct {
	Asset string  `json:"asset"`
	Qty   float64 `json:"qty"`
}

/*
NewExecution decodes an executions frame. An undecodable frame is an error:
applying a partially decoded fill would silently diverge the desk's positions.
*/
func NewExecution(buf []byte) (*Execution, error) {
	frame := &Execution{}

	if err := sonic.Unmarshal(buf, frame); err != nil {
		return nil, errnie.Err(
			errnie.UnprocessableContent,
			"[kraken.execution] invalid executions frame",
			err,
		)
	}

	return frame, nil
}

func (execution *Execution) MarshalJSON() ([]byte, error) {
	type alias Execution
	return sonic.Marshal((*alias)(execution))
}

func (execution *Execution) Action() string {
	return execution.Channel
}

func (execution *Execution) IsSuccess() bool {
	return len(execution.Data) > 0
}

func NewExecutionFromMap(model datura.Map[any]) *Execution {
	var execType string
	var orderStatus string

	if et, ok := model["exec_type"].(string); ok {
		execType = et
	}

	if os, ok := model["order_status"].(string); ok {
		orderStatus = os
	}

	if status, ok := model["status"].(string); ok && orderStatus == "" {
		orderStatus = status
	}

	if action, ok := model["action"].(string); ok {
		if action == "limit_order_placed" {
			if execType == "" {
				execType = "new"
			}
			if orderStatus == "" {
				orderStatus = "open"
			}
		}

		if action == "order_cancelled" {
			if execType == "" {
				execType = "canceled"
			}
			if orderStatus == "" {
				orderStatus = "canceled"
			}
		}
	}

	orderID, _ := model["order_id"].(string)
	clientOrderID, _ := model["cl_ord_id"].(string)
	execID, _ := model["trade_id"].(string)

	if execID == "" {
		execID, _ = model["id"].(string)
	}

	pair, _ := model["pair"].(string)
	side, _ := model["side"].(string)
	volume, _ := model["volume"].(float64)
	price, _ := model["price"].(float64)
	cost, _ := model["cost"].(float64)
	fee, _ := model["fee"].(float64)

	var timestamp time.Time

	if timeRaw, ok := model["time"].(string); ok && timeRaw != "" {
		if parsed, err := time.Parse(time.RFC3339, timeRaw); err == nil {
			timestamp = parsed
		}

		if timestamp.IsZero() {
			if parsed, err := time.Parse(time.RFC3339Nano, timeRaw); err == nil {
				timestamp = parsed
			}
		}
	}

	if timestamp.IsZero() {
		if sec, ok := model["timestamp"].(float64); ok && sec > 0 {
			timestamp = time.Unix(int64(sec), int64((sec-float64(int64(sec)))*1e9))
		}
	}

	return &Execution{
		Channel: "executions",
		Type:    "update",
		Data: []ExecutionData{{
			OrderID:       orderID,
			ClientOrderID: clientOrderID,
			ExecID:        execID,
			ExecType:      execType,
			Symbol:        pair,
			Side:          strings.ToLower(side),
			LastQty:       decimal.NewFromFloat64(volume),
			LastPrice:     decimal.NewFromFloat64(price),
			Cost:          decimal.NewFromFloat64(cost),
			OrderStatus:   orderStatus,
			CumQty:        decimal.NewFromFloat64(volume),
			CumCost:       decimal.NewFromFloat64(cost),
			AvgPrice:      decimal.NewFromFloat64(price),
			FeeUsdEquiv:   decimal.NewFromFloat64(fee),
			Timestamp:     timestamp,
		}},
	}
}

/*
ExecutionSubscription requests the authenticated executions stream on the
private transport, live websocket or Paper alike.
*/
type ExecutionSubscription struct {
	Token string
}

/*
NewExecutionSubscription binds the current authenticated websocket token.
*/
func NewExecutionSubscription(token string) ExecutionSubscription {
	return ExecutionSubscription{Token: token}
}

/*
MarshalJSON encodes Kraken's private executions subscription request.
*/
func (subscription ExecutionSubscription) MarshalJSON() ([]byte, error) {
	return sonic.Marshal(map[string]any{
		"method": "subscribe",
		"params": map[string]any{
			"channel": "executions",
			"token":   subscription.Token,
		},
	})
}

/*
ToMeasurement converts the Execution into a Measurement frame.
If it has multiple data items, they are attached as peers.
*/
func (execution *Execution) ToMeasurement(epochs ...int64) *data.Measurement {
	if execution == nil {
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

	channel := execution.Channel

	if channel == "" {
		channel = "executions"
	}

	msgType := execution.Type

	if msgType == "" {
		msgType = "update"
	}

	if len(execution.Data) == 1 {
		return execution.Data[0].ToMeasurement(epoch)
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

	for _, item := range execution.Data {
		itemMeas := item.ToMeasurement(epoch)

		if itemMeas != nil {
			meas.Peers(itemMeas)
		}
	}

	meas.Finalize()
	return meas
}

/*
ToMeasurement converts the ExecutionData into a structured Measurement.
*/
func (exec *ExecutionData) ToMeasurement(epochs ...int64) *data.Measurement {
	if exec == nil {
		return nil
	}

	epoch := system.Epoch.Load()

	if len(epochs) > 0 && epochs[0] > 0 {
		epoch = epochs[0]
	}

	now := time.Now().UTC()

	if !exec.Timestamp.IsZero() {
		now = exec.Timestamp.UTC()
	}

	if epoch <= 0 {
		epoch = now.UnixNano()
	}

	meas := data.NewMeasurement(
		epoch,
		exec.Symbol,
		"executions",
		now,
		now,
		&data.StringEntry{
			Key:   "order_id",
			Value: exec.OrderID,
		},
		&data.StringEntry{
			Key:   "cl_ord_id",
			Value: exec.ClientOrderID,
		},
		&data.StringEntry{
			Key:   "exec_id",
			Value: exec.ExecID,
		},
		&data.StringEntry{
			Key:   "exec_type",
			Value: exec.ExecType,
		},
		&data.StringEntry{
			Key:   "side",
			Value: exec.Side,
		},
		&data.StringEntry{
			Key:   "order_type",
			Value: exec.OrderType,
		},
		&data.StringEntry{
			Key:   "order_status",
			Value: exec.OrderStatus,
		},
		&data.StringEntry{
			Key:   "liquidity_ind",
			Value: exec.LiquidityInd,
		},
	)

	var metrics []*data.Metric

	if exec.LastPrice != nil {
		metrics = append(metrics, data.NewExactMetric(
			"last_price", exec.LastPrice, data.UnitPrice, data.TimescaleInstantaneous,
		))
	}

	if exec.LastQty != nil {
		metrics = append(metrics, data.NewExactMetric(
			"last_qty", exec.LastQty, data.UnitQuantity, data.TimescaleInstantaneous,
		))
	}

	if exec.Cost != nil {
		metrics = append(metrics, data.NewExactMetric(
			"cost", exec.Cost, data.UnitCurrency, data.TimescaleInstantaneous,
		))
	}

	if exec.CumQty != nil {
		metrics = append(metrics, data.NewExactMetric(
			"cum_qty", exec.CumQty, data.UnitQuantity, data.TimescaleInstantaneous,
		))
	}

	if exec.CumCost != nil {
		metrics = append(metrics, data.NewExactMetric(
			"cum_cost", exec.CumCost, data.UnitCurrency, data.TimescaleInstantaneous,
		))
	}

	if exec.AvgPrice != nil {
		metrics = append(metrics, data.NewExactMetric(
			"avg_price", exec.AvgPrice, data.UnitPrice, data.TimescaleInstantaneous,
		))
	}

	if exec.FeeUsdEquiv != nil {
		metrics = append(metrics, data.NewExactMetric(
			"fee_usd", exec.FeeUsdEquiv, data.UnitCurrency, data.TimescaleInstantaneous,
		))
	}

	if len(metrics) > 0 {
		meas.Write(metrics...)
	}

	meas.Finalize()
	return meas
}

/*
NewExecutionFromMeasurement reconstructs an Execution from a Measurement.
*/
func NewExecutionFromMeasurement(meas *data.Measurement) (*Execution, error) {
	if meas == nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation, "[kraken.execution] cannot convert nil measurement to execution", nil,
		))
	}

	peers := meas.Peers()

	if len(peers) > 0 {
		execution := &Execution{
			Channel: "executions",
			Type:    "update",
			Data:    make([]ExecutionData, 0, len(peers)),
		}

		for entry := range meas.Meta() {
			if entry == nil {
				continue
			}

			if entry.Key == "channel" && entry.Value != "" {
				execution.Channel = entry.Value
			}

			if entry.Key == "type" && entry.Value != "" {
				execution.Type = entry.Value
			}
		}

		for _, peer := range peers {
			itemData, err := NewExecutionDataFromMeasurement(peer)

			if err == nil && itemData != nil {
				execution.Data = append(execution.Data, *itemData)
			}
		}

		return execution, nil
	}

	singleData, err := NewExecutionDataFromMeasurement(meas)

	if err != nil {
		return nil, err
	}

	return &Execution{
		Channel: "executions",
		Type:    "update",
		Data:    []ExecutionData{*singleData},
	}, nil
}

/*
NewExecutionDataFromMeasurement reconstructs ExecutionData from a Measurement.
*/
func NewExecutionDataFromMeasurement(meas *data.Measurement) (*ExecutionData, error) {
	if meas == nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation, "[kraken.execution] cannot convert nil measurement to execution data", nil,
		))
	}

	meta := make(map[string]string)

	for entry := range meas.Meta() {
		if entry != nil && entry.Key != "" {
			meta[entry.Key] = entry.Value
		}
	}

	symbol := meas.Label

	if s, ok := meta["symbol"]; ok && s != "" {
		symbol = s
	}

	exec := &ExecutionData{
		Symbol:        symbol,
		OrderID:       meta["order_id"],
		ClientOrderID: meta["cl_ord_id"],
		ExecID:        meta["exec_id"],
		ExecType:      meta["exec_type"],
		Side:          meta["side"],
		OrderType:     meta["order_type"],
		OrderStatus:   meta["order_status"],
		LiquidityInd:  meta["liquidity_ind"],
		Timestamp:     meas.At,
	}

	if entry := data.Pull(meas.Read("last_price")); entry != nil && entry.Metric != nil {
		exec.LastPrice = entry.Metric.Exact()
	}

	if entry := data.Pull(meas.Read("last_qty")); entry != nil && entry.Metric != nil {
		exec.LastQty = entry.Metric.Exact()
	}

	if entry := data.Pull(meas.Read("cost")); entry != nil && entry.Metric != nil {
		exec.Cost = entry.Metric.Exact()
	}

	if entry := data.Pull(meas.Read("cum_qty")); entry != nil && entry.Metric != nil {
		exec.CumQty = entry.Metric.Exact()
	}

	if entry := data.Pull(meas.Read("cum_cost")); entry != nil && entry.Metric != nil {
		exec.CumCost = entry.Metric.Exact()
	}

	if entry := data.Pull(meas.Read("avg_price")); entry != nil && entry.Metric != nil {
		exec.AvgPrice = entry.Metric.Exact()
	}

	if entry := data.Pull(meas.Read("fee_usd")); entry != nil && entry.Metric != nil {
		exec.FeeUsdEquiv = entry.Metric.Exact()
	}

	return exec, nil
}
