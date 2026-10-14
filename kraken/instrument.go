package kraken

import (
	"strconv"
	"time"

	"github.com/bytedance/sonic"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/system"
)

type Instrument struct {
	Channel string `json:"channel"`
	Type    string `json:"type"`
	Data    struct {
		Pairs []InstrumentPair `json:"pairs"`
	} `json:"data"`
}

func NewInstrument(buf []byte) *Instrument {
	instrument := &Instrument{}
	errnie.Error(sonic.Unmarshal(buf, instrument))
	return instrument
}

/*
InstrumentPair carries Kraken's execution rules. Fixed-point price, cost, and
quantity boundaries remain Decimal so sizing never reconstructs venue rules
from binary floating-point values.
*/
type InstrumentPair struct {
	Symbol             string           `json:"symbol"`
	Base               string           `json:"base"`
	Quote              string           `json:"quote"`
	Status             string           `json:"status"`
	QtyPrecision       int              `json:"qty_precision"`
	QtyIncrement       *decimal.Decimal `json:"qty_increment"`
	PricePrecision     int              `json:"price_precision"`
	CostPrecision      int              `json:"cost_precision"`
	Marginable         bool             `json:"marginable"`
	HasIndex           bool             `json:"has_index"`
	CostMin            *decimal.Decimal `json:"cost_min"`
	MarginInitial      *decimal.Decimal `json:"margin_initial"`
	PositionLimitLong  int              `json:"position_limit_long"`
	PositionLimitShort int              `json:"position_limit_short"`
	TickSize           *decimal.Decimal `json:"tick_size"`
	PriceIncrement     *decimal.Decimal `json:"price_increment"`
	QtyMin             *decimal.Decimal `json:"qty_min"`
}

type InstrumentSubscription struct {
	Method string                       `json:"method"`
	Params InstrumentSubscriptionParams `json:"params"`
}

type InstrumentSubscriptionParams struct {
	Channel string `json:"channel"`
}

func NewInstrumentSubscription() InstrumentSubscription {
	return InstrumentSubscription{
		Method: "subscribe",
		Params: InstrumentSubscriptionParams{
			Channel: "instrument",
		},
	}
}

func (subscription InstrumentSubscription) MarshalJSON() ([]byte, error) {
	type alias InstrumentSubscription
	return sonic.Marshal((*alias)(&subscription))
}

/*
ToMeasurement converts the Instrument snapshot into a snapshot Measurement
with the individual pair measurements attached as peers.
*/
func (instrument *Instrument) ToMeasurement(epochs ...int64) *data.Measurement {
	if instrument == nil {
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

	channel := instrument.Channel

	if channel == "" {
		channel = "instrument"
	}

	msgType := instrument.Type

	if msgType == "" {
		msgType = "snapshot"
	}

	snapshotMeas := data.NewMeasurement(
		epoch,
		msgType,
		channel,
		now,
		now,
		&data.StringEntry{
			Key:   "type",
			Value: msgType,
		},
		&data.StringEntry{
			Key:   "channel",
			Value: channel,
		},
		&data.StringEntry{
			Key:   "pairs_count",
			Value: strconv.Itoa(len(instrument.Data.Pairs)),
		},
	)

	for _, pair := range instrument.Data.Pairs {
		pairMeas := pair.ToMeasurement(epoch)

		if pairMeas != nil {
			snapshotMeas.Peers(pairMeas)
		}
	}

	snapshotMeas.Finalize()

	return snapshotMeas
}

/*
NewInstrumentFromMeasurement reconstructs an Instrument snapshot from a snapshot Measurement.
*/
func NewInstrumentFromMeasurement(meas *data.Measurement) (*Instrument, error) {
	if meas == nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation, "[kraken.instrument] cannot convert nil measurement to instrument", nil,
		))
	}

	instrument := &Instrument{
		Channel: "instrument",
		Type:    "snapshot",
	}

	for entry := range meas.Meta() {
		if entry == nil {
			continue
		}

		if entry.Key == "channel" && entry.Value != "" {
			instrument.Channel = entry.Value
		}

		if entry.Key == "type" && entry.Value != "" {
			instrument.Type = entry.Value
		}
	}

	peers := meas.Peers()
	instrument.Data.Pairs = make([]InstrumentPair, 0, len(peers))

	for _, peerMeas := range peers {
		pair, err := NewInstrumentPairFromMeasurement(peerMeas)

		if err == nil && pair != nil {
			instrument.Data.Pairs = append(instrument.Data.Pairs, *pair)
		}
	}

	return instrument, nil
}

/*
ToMeasurement converts the InstrumentPair into a structured Measurement
without any serialized raw shortcuts.
*/
func (pair *InstrumentPair) ToMeasurement(epochs ...int64) *data.Measurement {
	if pair == nil {
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
		pair.Symbol,
		"instrument",
		now,
		now,
		&data.StringEntry{
			Key:   "symbol",
			Value: pair.Symbol,
		},
		&data.StringEntry{
			Key:   "base",
			Value: pair.Base,
		},
		&data.StringEntry{
			Key:   "quote",
			Value: pair.Quote,
		},
		&data.StringEntry{
			Key:   "status",
			Value: pair.Status,
		},
		&data.StringEntry{
			Key:   "qty_precision",
			Value: strconv.Itoa(pair.QtyPrecision),
		},
		&data.StringEntry{
			Key:   "price_precision",
			Value: strconv.Itoa(pair.PricePrecision),
		},
		&data.StringEntry{
			Key:   "cost_precision",
			Value: strconv.Itoa(pair.CostPrecision),
		},
		&data.StringEntry{
			Key:   "marginable",
			Value: strconv.FormatBool(pair.Marginable),
		},
		&data.StringEntry{
			Key:   "has_index",
			Value: strconv.FormatBool(pair.HasIndex),
		},
		&data.StringEntry{
			Key:   "pos_limit_long",
			Value: strconv.Itoa(pair.PositionLimitLong),
		},
		&data.StringEntry{
			Key:   "pos_limit_short",
			Value: strconv.Itoa(pair.PositionLimitShort),
		},
	)

	var metrics []*data.Metric

	if pair.CostMin != nil {
		metrics = append(metrics, data.NewExactMetric(
			"cost_min", pair.CostMin, data.UnitPrice, data.TimescaleInstantaneous,
		))
	}

	if pair.QtyMin != nil {
		metrics = append(metrics, data.NewExactMetric(
			"qty_min", pair.QtyMin, data.UnitQuantity, data.TimescaleInstantaneous,
		))
	}

	if pair.QtyIncrement != nil {
		metrics = append(metrics, data.NewExactMetric(
			"qty_increment", pair.QtyIncrement, data.UnitQuantity, data.TimescaleInstantaneous,
		))
	}

	if pair.MarginInitial != nil {
		metrics = append(metrics, data.NewExactMetric(
			"margin_initial", pair.MarginInitial, data.UnitRatio, data.TimescaleInstantaneous,
		))
	}

	if pair.TickSize != nil {
		metrics = append(metrics, data.NewExactMetric(
			"tick_size", pair.TickSize, data.UnitPrice, data.TimescaleInstantaneous,
		))
	}

	if pair.PriceIncrement != nil {
		metrics = append(metrics, data.NewExactMetric(
			"price_increment", pair.PriceIncrement, data.UnitPrice, data.TimescaleInstantaneous,
		))
	}

	if len(metrics) > 0 {
		meas.Write(metrics...)
	}

	meas.Finalize()

	return meas
}

/*
NewInstrumentPairFromMeasurement reconstructs an InstrumentPair from a Measurement.
*/
func NewInstrumentPairFromMeasurement(meas *data.Measurement) (*InstrumentPair, error) {
	if meas == nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation, "[kraken.instrument] cannot convert nil measurement to pair", nil,
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

	if symbol == "" {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation, "[kraken.instrument] measurement missing symbol", nil,
		))
	}

	pair := &InstrumentPair{
		Symbol: symbol,
		Base:   meta["base"],
		Quote:  meta["quote"],
		Status: meta["status"],
	}

	if val, ok := meta["qty_precision"]; ok {
		num, convErr := strconv.Atoi(val)

		if convErr == nil {
			pair.QtyPrecision = num
		}
	}

	if val, ok := meta["price_precision"]; ok {
		num, convErr := strconv.Atoi(val)

		if convErr == nil {
			pair.PricePrecision = num
		}
	}

	if val, ok := meta["cost_precision"]; ok {
		num, convErr := strconv.Atoi(val)

		if convErr == nil {
			pair.CostPrecision = num
		}
	}

	if val, ok := meta["marginable"]; ok {
		pair.Marginable = val == "true"
	}

	if val, ok := meta["has_index"]; ok {
		pair.HasIndex = val == "true"
	}

	if val, ok := meta["pos_limit_long"]; ok {
		num, convErr := strconv.Atoi(val)

		if convErr == nil {
			pair.PositionLimitLong = num
		}
	}

	if val, ok := meta["pos_limit_short"]; ok {
		num, convErr := strconv.Atoi(val)

		if convErr == nil {
			pair.PositionLimitShort = num
		}
	}

	if entry := data.Pull(meas.Read("cost_min")); entry != nil && entry.Metric != nil {
		pair.CostMin = entry.Metric.Exact()
	}

	if entry := data.Pull(meas.Read("qty_min")); entry != nil && entry.Metric != nil {
		pair.QtyMin = entry.Metric.Exact()
	}

	if entry := data.Pull(meas.Read("qty_increment")); entry != nil && entry.Metric != nil {
		pair.QtyIncrement = entry.Metric.Exact()
	}

	if entry := data.Pull(meas.Read("margin_initial")); entry != nil && entry.Metric != nil {
		pair.MarginInitial = entry.Metric.Exact()
	}

	if entry := data.Pull(meas.Read("tick_size")); entry != nil && entry.Metric != nil {
		pair.TickSize = entry.Metric.Exact()
	}

	if entry := data.Pull(meas.Read("price_increment")); entry != nil && entry.Metric != nil {
		pair.PriceIncrement = entry.Metric.Exact()
	}

	return pair, nil
}
