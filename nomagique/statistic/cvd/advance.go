package cvd

import (
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
	*core.PrimitiveError

	buyQty    core.Primitive
	sellQty   core.Primitive
	grossQty  core.Primitive
	netQty    core.Primitive
	buyCount  core.Primitive
	sellCount core.Primitive
}

func NewQuantity() *Quantity {
	return &Quantity{PrimitiveError: core.NewPrimitiveError(), buyQty: statistic.NewSum(),
		sellQty:   statistic.NewSum(),
		grossQty:  statistic.NewSum(),
		netQty:    statistic.NewSum(),
		buyCount:  statistic.NewSum(),
		sellCount: statistic.NewSum(),
	}
}

func (quantity *Quantity) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			m := *(**data.Measurement[float64])(arriving)

			if m.Err != nil {
				if !yield(arriving) {
					return
				}

				continue
			}

			currentQuantity := m.Metrics["qty"].Raw
			buy := m.Provenance["side"] == "buy"

			one, zero := 1.0, 0.0
			signed := currentQuantity
			if !buy {
				signed = -currentQuantity
			}

			net := drive[float64, float64](quantity.netQty, &signed)
			gross := drive[float64, float64](quantity.grossQty, &currentQuantity)

			buyCount, sellCount := 0.0, 0.0
			buyTotal, sellTotal := 0.0, 0.0

			if buy {
				buyCount = drive[float64, float64](quantity.buyCount, &one)
				sellCount = drive[float64, float64](quantity.sellCount, &zero)
				buyTotal = drive[float64, float64](quantity.buyQty, &currentQuantity)
				sellTotal = drive[float64, float64](quantity.sellQty, &zero)
			}

			if !buy {
				buyCount = drive[float64, float64](quantity.buyCount, &zero)
				sellCount = drive[float64, float64](quantity.sellCount, &one)
				buyTotal = drive[float64, float64](quantity.buyQty, &zero)
				sellTotal = drive[float64, float64](quantity.sellQty, &currentQuantity)
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

/*
Notional owns the signed executed-notional arithmetic and its causal view:
each arrival's aggressor-signed price×quantity accumulates into the running
sums, the fractions derive from the sums, and the signed net fraction is
tracked against its own adaptive baseline.
*/
type Notional struct {
	*core.PrimitiveError

	from   time.Time
	at     time.Time
	buy    core.Primitive
	sell   core.Primitive
	gross  core.Primitive
	net    core.Primitive
	reader core.Primitive
}

func NewNotional() *Notional {
	return &Notional{PrimitiveError: core.NewPrimitiveError(), buy: statistic.NewSum(),
		sell:   statistic.NewSum(),
		gross:  statistic.NewSum(),
		net:    statistic.NewSum(),
		reader: adaptive.NewBaseline(adaptive.NewWindow()),
	}
}

func (notional *Notional) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			m := *(**data.Measurement[float64])(arriving)

			if m.Err != nil {
				if !yield(arriving) {
					return
				}

				continue
			}

			currentNotional := m.Metrics["price"].Raw * m.Metrics["qty"].Raw
			buy := m.Provenance["side"] == "buy"

			zero := 0.0
			signed := currentNotional
			if !buy {
				signed = -currentNotional
			}

			net := drive[float64, float64](notional.net, &signed)
			gross := drive[float64, float64](notional.gross, &currentNotional)

			buyTotal, sellTotal := 0.0, 0.0

			if buy {
				buyTotal = drive[float64, float64](notional.buy, &currentNotional)
				sellTotal = drive[float64, float64](notional.sell, &zero)
			}

			if !buy {
				buyTotal = drive[float64, float64](notional.buy, &zero)
				sellTotal = drive[float64, float64](notional.sell, &currentNotional)
			}

			if m.Metadata == nil {
				m.Metadata = make(map[string]string, 3)
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

				reading := drive[float64, adaptive.BaselineReading](notional.reader, &fraction)
				m.Metadata[data.MetadataSupport] = strconv.FormatFloat(reading.Count, 'f', -1, 64)

				if reading.HasPrior {
					m.Metrics["signed_net_fraction_baseline"] = m.Metrics["signed_net_fraction_baseline"].Write(reading.Baseline)
					m.Metrics["signed_net_fraction_divergence"] = m.Metrics["signed_net_fraction_divergence"].Write(reading.Residual)
					m.Metrics["signed_net_fraction_zscore"] = m.Metrics["signed_net_fraction_zscore"].Write(reading.ZScore)
					m.Metadata[data.MetadataDivergence] = strconv.FormatFloat(reading.Residual, 'f', -1, 64)

					if reading.VarianceDefined {
						m.Metadata[data.MetadataNoiseVariance] = strconv.FormatFloat(reading.Variance, 'f', -1, 64)
					}
				}
			}

			if !yield(arriving) {
				return
			}
		}
	}
}

/*
Rates owns the elapsed-time derivations of the cumulative accounting: rates
per second and the net notional rate's velocity.
*/
type Rates struct {
	*core.PrimitiveError

	from     time.Time
	velocity core.Primitive
}

func NewRates() *Rates {
	return &Rates{PrimitiveError: core.NewPrimitiveError(), velocity: temporal.NewVelocity()}
}

func (rates *Rates) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			m := *(**data.Measurement[float64])(arriving)

			if m.Err != nil {
				if !yield(arriving) {
					return
				}

				continue
			}

			if rates.from.IsZero() || m.At.Before(rates.from) {
				rates.from = m.At
			}

			m.From = rates.from

			m.Metrics["cvd_epoch_from"] = m.Metrics["cvd_epoch_from"].Write(float64(rates.from.UnixNano()) / float64(time.Second))

			elapsed := m.At.Sub(rates.from).Seconds()

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
			velocity := drive[temporal.Observation, temporal.VelocityReading](rates.velocity, &observation)

			if velocity.Defined {
				m.Metrics["net_notional_rate_velocity"] = m.Metrics["net_notional_rate_velocity"].Write(velocity.Rate)
			}

			if !yield(arriving) {
				return
			}
		}
	}
}
