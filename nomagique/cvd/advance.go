package cvd

import (
	"errors"
	"iter"
	"time"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/adaptive"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/statistic"
	"github.com/theapemachine/symm/nomagique/temporal"
)

/*
Quantity owns the signed executed-quantity arithmetic: each arrival's
aggressor-signed quantity accumulates into the running sums, and the sums'
facts are written where they are computed.
*/
type Quantity struct {
	err       error
	buyQty    core.Primitive
	sellQty   core.Primitive
	grossQty  core.Primitive
	netQty    core.Primitive
	buyCount  core.Primitive
	sellCount core.Primitive
}

func NewQuantity() core.Primitive {
	return &Quantity{
		buyQty:    statistic.NewSum(),
		sellQty:   statistic.NewSum(),
		grossQty:  statistic.NewSum(),
		netQty:    statistic.NewSum(),
		buyCount:  statistic.NewSum(),
		sellCount: statistic.NewSum(),
	}
}

func (op *Quantity) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			m := *(**data.Measurement[float64])(arriving)

			if m.Err != nil {
				if !yield(arriving) {
					return
				}

				continue
			}

			quantity := m.Metrics["qty"].Raw
			buy := m.Provenance["side"] == "buy"

			one, zero := 1.0, 0.0
			signed := quantity
			if !buy {
				signed = -quantity
			}

			net := drive[float64, float64](op.netQty, &signed)
			gross := drive[float64, float64](op.grossQty, &quantity)

			buyCount, sellCount := 0.0, 0.0
			buyTotal, sellTotal := 0.0, 0.0

			if buy {
				buyCount = drive[float64, float64](op.buyCount, &one)
				sellCount = drive[float64, float64](op.sellCount, &zero)
				buyTotal = drive[float64, float64](op.buyQty, &quantity)
				sellTotal = drive[float64, float64](op.sellQty, &zero)
			}

			if !buy {
				buyCount = drive[float64, float64](op.buyCount, &zero)
				sellCount = drive[float64, float64](op.sellCount, &one)
				buyTotal = drive[float64, float64](op.buyQty, &zero)
				sellTotal = drive[float64, float64](op.sellQty, &quantity)
			}

			m.Metrics["trade_count:buy"] = m.Metrics["trade_count:buy"].Write(buyCount)
			m.Metrics["trade_count:sell"] = m.Metrics["trade_count:sell"].Write(sellCount)
			m.Metrics["trade_count"] = m.Metrics["trade_count"].Write(buyCount + sellCount)
			m.Metrics["executed_quantity:buy"] = m.Metrics["executed_quantity:buy"].Write(buyTotal)
			m.Metrics["executed_quantity:sell"] = m.Metrics["executed_quantity:sell"].Write(sellTotal)
			m.Metrics["gross_executed_quantity"] = m.Metrics["gross_executed_quantity"].Write(gross)
			m.Metrics["net_executed_quantity"] = m.Metrics["net_executed_quantity"].Write(net)
			m.Metrics["cumulative_volume_delta"] = m.Metrics["cumulative_volume_delta"].Write(net)

			if gross > 0 {
				m.Metrics["signed_count_fraction"] = m.Metrics["signed_count_fraction"].Write((buyCount - sellCount) / (buyCount + sellCount))
			}

			if !yield(arriving) {
				return
			}
		}
	}
}

func (op *Quantity) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	return op.err
}

/*
Notional owns the signed executed-notional arithmetic and its causal view:
each arrival's aggressor-signed price×quantity accumulates into the running
sums, the fractions derive from the sums, and the signed net fraction is
tracked against its own adaptive baseline.
*/
type Notional struct {
	err    error
	from   time.Time
	at     time.Time
	buy    core.Primitive
	sell   core.Primitive
	gross  core.Primitive
	net    core.Primitive
	reader core.Primitive
}

func NewNotional() core.Primitive {
	return &Notional{
		buy:    statistic.NewSum(),
		sell:   statistic.NewSum(),
		gross:  statistic.NewSum(),
		net:    statistic.NewSum(),
		reader: adaptive.NewBaseline(adaptive.NewWindow()),
	}
}

func (op *Notional) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			m := *(**data.Measurement[float64])(arriving)

			if m.Err != nil {
				if !yield(arriving) {
					return
				}

				continue
			}

			notional := m.Metrics["price"].Raw * m.Metrics["qty"].Raw
			buy := m.Provenance["side"] == "buy"

			zero := 0.0
			signed := notional
			if !buy {
				signed = -notional
			}

			net := drive[float64, float64](op.net, &signed)
			gross := drive[float64, float64](op.gross, &notional)

			buyTotal, sellTotal := 0.0, 0.0

			if buy {
				buyTotal = drive[float64, float64](op.buy, &notional)
				sellTotal = drive[float64, float64](op.sell, &zero)
			}

			if !buy {
				buyTotal = drive[float64, float64](op.buy, &zero)
				sellTotal = drive[float64, float64](op.sell, &notional)
			}

			if m.Metadata == nil {
				m.Metadata = make(map[string]float64, 3)
			}

			m.Metrics["aggressive_notional:buy"] = m.Metrics["aggressive_notional:buy"].Write(buyTotal)
			m.Metrics["aggressive_notional:sell"] = m.Metrics["aggressive_notional:sell"].Write(sellTotal)
			m.Metrics["gross_notional"] = m.Metrics["gross_notional"].Write(gross)
			m.Metrics["net_notional"] = m.Metrics["net_notional"].Write(net)
			m.Metrics["cumulative_notional_delta"] = m.Metrics["cumulative_notional_delta"].Write(net)

			if tradeCount := m.Metrics["trade_count"].Raw; tradeCount > 0 {
				m.Metrics["mean_trade_notional"] = m.Metrics["mean_trade_notional"].Write(gross / tradeCount)
			}

			if gross > 0 {
				fraction := net / gross
				m.Metrics["signed_net_fraction"] = m.Metrics["signed_net_fraction"].Write(fraction)

				reading := drive[float64, adaptive.BaselineReading](op.reader, &fraction)

				if reading.HasPrior {
					m.Metrics["signed_net_fraction_baseline"] = m.Metrics["signed_net_fraction_baseline"].Write(reading.Baseline)
					m.Metrics["signed_net_fraction_divergence"] = m.Metrics["signed_net_fraction_divergence"].Write(reading.Residual)
					m.Metrics["signed_net_fraction_zscore"] = m.Metrics["signed_net_fraction_zscore"].Write(reading.ZScore)
					m.Metadata[data.MetadataDivergence] = reading.Residual

					if reading.VarianceDefined {
						m.Metadata[data.MetadataNoiseVariance] = reading.Variance
					}
				}
			}

			if !yield(arriving) {
				return
			}
		}
	}
}

func (op *Notional) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	return op.err
}

/*
Rates owns the elapsed-time derivations of the cumulative accounting: rates
per second and the net notional rate's velocity.
*/
type Rates struct {
	err      error
	from     time.Time
	velocity core.Primitive
}

func NewRates() core.Primitive {
	return &Rates{velocity: temporal.NewVelocity()}
}

func (op *Rates) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			m := *(**data.Measurement[float64])(arriving)

			if m.Err != nil {
				if !yield(arriving) {
					return
				}

				continue
			}

			if op.from.IsZero() {
				op.from = m.At
			}

			m.From = op.from

			m.Metrics["cvd_epoch_from"] = m.Metrics["cvd_epoch_from"].Write(float64(op.from.UnixNano()) / float64(time.Second))

			elapsed := m.At.Sub(op.from).Seconds()

			if elapsed <= 0 {
				if !yield(arriving) {
					return
				}

				continue
			}

			tradeCount := m.Metrics["trade_count"].Raw
			gross := m.Metrics["gross_notional"].Raw
			net := m.Metrics["net_notional"].Raw

			m.Metrics["trade_rate"] = m.Metrics["trade_rate"].Write(tradeCount / elapsed)
			m.Metrics["gross_notional_rate"] = m.Metrics["gross_notional_rate"].Write(gross / elapsed)
			m.Metrics["net_notional_rate"] = m.Metrics["net_notional_rate"].Write(net / elapsed)
			m.Metrics["buy_notional_rate"] = m.Metrics["buy_notional_rate"].Write(m.Metrics["aggressive_notional:buy"].Raw / elapsed)
			m.Metrics["sell_notional_rate"] = m.Metrics["sell_notional_rate"].Write(m.Metrics["aggressive_notional:sell"].Raw / elapsed)

			rate := net / elapsed
			observation := temporal.Observation{Value: rate, At: m.At.UnixNano()}
			velocity := drive[temporal.Observation, temporal.VelocityReading](op.velocity, &observation)

			if velocity.Defined {
				m.Metrics["net_notional_rate_velocity"] = m.Metrics["net_notional_rate_velocity"].Write(velocity.Rate)
			}

			if !yield(arriving) {
				return
			}
		}
	}
}

func (op *Rates) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	return op.err
}
