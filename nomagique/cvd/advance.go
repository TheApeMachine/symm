package cvd

import (
	"errors"
	"iter"
	"strconv"
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

			quantity := m.GetMetric("qty").Raw
			side, _ := m.GetProvenance("side")
			buy := side == "buy"

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

			m.WriteMetric("trade_count:buy", buyCount)
			m.WriteMetric("trade_count:sell", sellCount)
			m.WriteMetric("trade_count", buyCount+sellCount)
			m.WriteMetric("executed_quantity:buy", buyTotal)
			m.WriteMetric("executed_quantity:sell", sellTotal)
			m.WriteMetric("gross_executed_quantity", gross)
			m.WriteMetric("net_executed_quantity", net)
			m.WriteMetric("cumulative_volume_delta", net)

			if gross > 0 {
				m.WriteMetric("signed_count_fraction", (buyCount-sellCount)/(buyCount+sellCount))
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

			notional := m.GetMetric("price").Raw * m.GetMetric("qty").Raw
			side, _ := m.GetProvenance("side")
			buy := side == "buy"

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

			m.EnsureMetadata()

			m.WriteMetric("aggressive_notional:buy", buyTotal)
			m.WriteMetric("aggressive_notional:sell", sellTotal)
			m.WriteMetric("gross_notional", gross)
			m.WriteMetric("net_notional", net)
			m.WriteMetric("cumulative_notional_delta", net)

			if tradeCount := m.GetMetric("trade_count").Raw; tradeCount > 0 {
				m.WriteMetric("mean_trade_notional", gross/tradeCount)
			}

			if gross > 0 {
				fraction := net / gross
				m.WriteMetric("signed_net_fraction", fraction)

				reading := drive[float64, adaptive.BaselineReading](op.reader, &fraction)
				m.SetMetadata(data.MetadataSupport, strconv.FormatFloat(reading.Count, 'f', -1, 64))

				if reading.HasPrior {
					m.WriteMetric("signed_net_fraction_baseline", reading.Baseline)
					m.WriteMetric("signed_net_fraction_divergence", reading.Residual)
					m.WriteMetric("signed_net_fraction_zscore", reading.ZScore)
					m.SetMetadata(data.MetadataDivergence, strconv.FormatFloat(reading.Residual, 'f', -1, 64))

					if reading.VarianceDefined {
						m.SetMetadata(data.MetadataNoiseVariance, strconv.FormatFloat(reading.Variance, 'f', -1, 64))
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

			if op.from.IsZero() || m.At.Before(op.from) {
				op.from = m.At
			}

			m.From = op.from

			m.WriteMetric("cvd_epoch_from", float64(op.from.UnixNano())/float64(time.Second))

			elapsed := m.At.Sub(op.from).Seconds()

			if elapsed <= 0 {
				if !yield(arriving) {
					return
				}

				continue
			}

			tradeCount := m.GetMetric("trade_count").Raw
			gross := m.GetMetric("gross_notional").Raw
			net := m.GetMetric("net_notional").Raw

			m.WriteMetric("trade_rate", tradeCount/elapsed)
			m.WriteMetric("gross_notional_rate", gross/elapsed)
			m.WriteMetric("net_notional_rate", net/elapsed)
			m.WriteMetric("buy_notional_rate", m.GetMetric("aggressive_notional:buy").Raw/elapsed)
			m.WriteMetric("sell_notional_rate", m.GetMetric("aggressive_notional:sell").Raw/elapsed)

			rate := net / elapsed
			observation := temporal.Observation{Value: rate, At: m.At.UnixNano()}
			velocity := drive[temporal.Observation, temporal.VelocityReading](op.velocity, &observation)

			if velocity.Defined {
				m.WriteMetric("net_notional_rate_velocity", velocity.Rate)
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
