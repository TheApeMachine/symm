package cvd

import (
	"errors"
	"iter"
	"math"
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

			countScale := math.Max(buyCount+sellCount, 1)
			m.SetMetric("trade_count:buy", data.NewMetric[float64](
				"trade_count:buy",
				data.UnitCount,
				data.TimescaleRollingWindow,
				0,
				countScale,
			).Write(buyCount))
			m.SetMetric("trade_count:sell", data.NewMetric[float64](
				"trade_count:sell",
				data.UnitCount,
				data.TimescaleRollingWindow,
				0,
				countScale,
			).Write(sellCount))
			m.SetMetric("trade_count", data.NewMetric[float64](
				"trade_count",
				data.UnitCount,
				data.TimescaleRollingWindow,
				0,
				countScale,
			).Write(buyCount+sellCount))

			grossScale := math.Max(gross, 1)
			m.SetMetric("executed_quantity:buy", data.NewMetric[float64](
				"executed_quantity:buy",
				data.UnitVolume,
				data.TimescaleRollingWindow,
				0,
				grossScale,
			).Write(buyTotal))
			m.SetMetric("executed_quantity:sell", data.NewMetric[float64](
				"executed_quantity:sell",
				data.UnitVolume,
				data.TimescaleRollingWindow,
				0,
				grossScale,
			).Write(sellTotal))
			m.SetMetric("gross_executed_quantity", data.NewMetric[float64](
				"gross_executed_quantity",
				data.UnitVolume,
				data.TimescaleRollingWindow,
				0,
				grossScale,
			).Write(gross))
			m.SetMetric("net_executed_quantity", data.NewMetric[float64](
				"net_executed_quantity",
				data.UnitVolume,
				data.TimescaleRollingWindow,
				0,
				grossScale,
			).Write(net))
			m.SetMetric("cumulative_volume_delta", data.NewMetric[float64](
				"cumulative_volume_delta",
				data.UnitVolume,
				data.TimescaleRollingWindow,
				0,
				grossScale,
			).Write(net))

			if gross > 0 {
				m.SetMetric("signed_count_fraction", data.NewMetric[float64](
					"signed_count_fraction",
					data.UnitRatio,
					data.TimescaleRollingWindow,
					0.0,
					1.0,
				).Write((buyCount - sellCount) / (buyCount + sellCount)))
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

			grossNotionalScale := math.Max(gross, 1)
			m.SetMetric("aggressive_notional:buy", data.NewMetric[float64](
				"aggressive_notional:buy",
				data.UnitNotional,
				data.TimescaleRollingWindow,
				0,
				grossNotionalScale,
			).Write(buyTotal))
			m.SetMetric("aggressive_notional:sell", data.NewMetric[float64](
				"aggressive_notional:sell",
				data.UnitNotional,
				data.TimescaleRollingWindow,
				0,
				grossNotionalScale,
			).Write(sellTotal))
			m.SetMetric("gross_notional", data.NewMetric[float64](
				"gross_notional",
				data.UnitNotional,
				data.TimescaleRollingWindow,
				0,
				grossNotionalScale,
			).Write(gross))
			m.SetMetric("net_notional", data.NewMetric[float64](
				"net_notional",
				data.UnitNotional,
				data.TimescaleRollingWindow,
				0,
				grossNotionalScale,
			).Write(net))
			m.SetMetric("cumulative_notional_delta", data.NewMetric[float64](
				"cumulative_notional_delta",
				data.UnitNotional,
				data.TimescaleRollingWindow,
				0,
				grossNotionalScale,
			).Write(net))

			if tradeCount := m.GetMetric("trade_count").Raw; tradeCount > 0 {
				m.SetMetric("mean_trade_notional", data.NewMetric[float64](
					"mean_trade_notional",
					data.UnitNotional,
					data.TimescaleRollingWindow,
					0,
					math.Max(gross/tradeCount, 1),
				).Write(gross/tradeCount))
			}

			if gross > 0 {
				fraction := net / gross
				m.SetMetric("signed_net_fraction", data.NewMetric[float64](
					"signed_net_fraction",
					data.UnitRatio,
					data.TimescaleRollingWindow,
					0.0,
					1.0,
				).Write(fraction))

				reading := drive[float64, adaptive.BaselineReading](op.reader, &fraction)
				m.SetMetadata(data.MetadataSupport, strconv.FormatFloat(reading.Count, 'f', -1, 64))

				if reading.HasPrior {
					dispersion := math.Max(reading.Dispersion, 1e-6)
					m.SetMetric("signed_net_fraction_baseline", data.NewMetric[float64](
						"signed_net_fraction_baseline",
						data.UnitRatio,
						data.TimescaleRollingWindow,
						0.0,
						1.0,
					).Write(reading.Baseline))
					m.SetMetric("signed_net_fraction_divergence", data.NewMetric[float64](
						"signed_net_fraction_divergence",
						data.UnitRatio,
						data.TimescaleInstantaneous,
						0.0,
						dispersion,
					).Write(reading.Residual))
					m.SetMetric("signed_net_fraction_zscore", data.NewMetric[float64](
						"signed_net_fraction_zscore",
						data.UnitZScore,
						data.TimescaleRollingWindow,
						0.0,
						1.0,
					).Write(reading.ZScore))
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

			if !op.from.After(m.At) {
				m.From = op.from
			}

			epochSec := float64(op.from.UnixNano()) / float64(time.Second)
			m.SetMetric("cvd_epoch_from", data.NewMetric[float64](
				"cvd_epoch_from",
				data.UnitSecond,
				data.TimescaleEpoch,
				epochSec,
				1.0,
			).Write(epochSec))

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

			tradeRate := tradeCount / elapsed
			m.SetMetric("trade_rate", data.NewMetric[float64](
				"trade_rate",
				data.UnitTradeRate,
				data.TimescalePerSecond,
				0.0,
				math.Max(tradeRate, 1.0),
			).Write(tradeRate))

			grossRate := gross / elapsed
			m.SetMetric("gross_notional_rate", data.NewMetric[float64](
				"gross_notional_rate",
				data.UnitNotionalRate,
				data.TimescalePerSecond,
				0.0,
				math.Max(grossRate, 1.0),
			).Write(grossRate))

			netRate := net / elapsed
			m.SetMetric("net_notional_rate", data.NewMetric[float64](
				"net_notional_rate",
				data.UnitNotionalRate,
				data.TimescalePerSecond,
				0.0,
				math.Max(math.Abs(netRate), 1.0),
			).Write(netRate))

			buyRate := m.GetMetric("aggressive_notional:buy").Raw / elapsed
			m.SetMetric("buy_notional_rate", data.NewMetric[float64](
				"buy_notional_rate",
				data.UnitNotionalRate,
				data.TimescalePerSecond,
				0.0,
				math.Max(grossRate, 1.0),
			).Write(buyRate))

			sellRate := m.GetMetric("aggressive_notional:sell").Raw / elapsed
			m.SetMetric("sell_notional_rate", data.NewMetric[float64](
				"sell_notional_rate",
				data.UnitNotionalRate,
				data.TimescalePerSecond,
				0.0,
				math.Max(grossRate, 1.0),
			).Write(sellRate))

			rate := net / elapsed
			observation := temporal.Observation{Value: rate, At: m.At.UnixNano()}
			velocity := drive[temporal.Observation, temporal.VelocityReading](op.velocity, &observation)

			if velocity.Defined {
				m.SetMetric("net_notional_rate_velocity", data.NewMetric[float64](
					"net_notional_rate_velocity",
					data.UnitVelocity,
					data.TimescalePerSecond,
					0.0,
					math.Max(math.Abs(velocity.Rate), 1e-6),
				).Write(velocity.Rate))
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
