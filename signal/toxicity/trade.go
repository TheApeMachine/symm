package toxicity

import (
	"context"
	"sync"

	"strconv"
	"time"
	"unsafe"

	"github.com/theapemachine/errnie"

	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/adaptive"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/nomagique/transport"
)

/*
Trade matches incoming trades against the symbol's book touch. It holds no
state and no logic of its own: its entire behavior is one nomagique pipeline
over the measurement itself — every stage writes its facts into the measurement
where it computes them, and the workload's register owns the measurement's lifetime.
*/
type Trade struct {
	*runtime.System
	pipelines sync.Map
	ID        int
}

func NewTrade(ctx context.Context) *Trade {
	trade := &Trade{}

	trade.System = runtime.NewSystem(ctx, "toxicity:trade", trade)
	return trade
}

func (trade *Trade) pipelineFor(symbol string) core.Primitive {
	if existing, ok := trade.pipelines.Load(symbol); ok {
		return existing.(core.Primitive)
	}

	type tradeState struct {
		bracketQty         float64
		matchedBidQty      float64
		matchedAskQty      float64
		touchFillBidQty    float64
		touchFillAskQty    float64
		hasPrevTime        bool
		prevTime           time.Time
		bidBaseline        core.Primitive
		askBaseline        core.Primitive
		bidFractionSamples int
		askFractionSamples int
	}

	state := &tradeState{
		bidBaseline: adaptive.NewBaseline(adaptive.NewWindow()),
		askBaseline: adaptive.NewBaseline(adaptive.NewWindow()),
	}

	pipeline := nomagique.NewNumber(
		data.NewAdapter(
			transport.NewPass(),
			func(m *data.Measurement[float64]) *data.Measurement[float64] {
				input := m
				if len(m.Peers) > 0 {
					peer := m.FindPeer(func(p *data.Measurement[float64]) bool {
						_, hasP := p.LookupMetric("price")
						_, hasQ := p.LookupMetric("qty")
						return hasP && hasQ && p.Label != ""
					})
					if peer != nil {
						input = peer
					}
				}

				price := input.GetMetric("price").Raw
				qty := input.GetMetric("qty").Raw

				if price <= 0 || qty <= 0 {
					return m
				}

				bidPrice := input.GetMetric("best_price:bid").Raw
				if bidPrice == 0 {
					bidPrice = input.GetMetric("best_bid").Raw
				}
				if bidPrice == 0 {
					bidPrice = input.GetMetric("bid").Raw
				}

				askPrice := input.GetMetric("best_price:ask").Raw
				if askPrice == 0 {
					askPrice = input.GetMetric("best_ask").Raw
				}
				if askPrice == 0 {
					askPrice = input.GetMetric("ask").Raw
				}

				bidQty := input.GetMetric("touch_quantity:bid").Raw
				if bidQty == 0 {
					bidQty = input.GetMetric("bid_qty").Raw
				}

				askQty := input.GetMetric("touch_quantity:ask").Raw
				if askQty == 0 {
					askQty = input.GetMetric("ask_qty").Raw
				}

				if bidPrice == 0 || askPrice == 0 {
					touchPeer := m.FindPeer(func(p *data.Measurement[float64]) bool {
						if p.Label == "" {
							return false
						}
						b := p.GetMetric("best_price:bid").Raw
						if b == 0 {
							b = p.GetMetric("best_bid").Raw
						}
						if b == 0 {
							b = p.GetMetric("bid").Raw
						}
						a := p.GetMetric("best_price:ask").Raw
						if a == 0 {
							a = p.GetMetric("best_ask").Raw
						}
						if a == 0 {
							a = p.GetMetric("ask").Raw
						}
						return b > 0 && a > 0
					})

					if touchPeer != nil {
						if bidPrice == 0 {
							bidPrice = touchPeer.GetMetric("best_price:bid").Raw
							if bidPrice == 0 {
								bidPrice = touchPeer.GetMetric("best_bid").Raw
							}
							if bidPrice == 0 {
								bidPrice = touchPeer.GetMetric("bid").Raw
							}
						}
						if askPrice == 0 {
							askPrice = touchPeer.GetMetric("best_price:ask").Raw
							if askPrice == 0 {
								askPrice = touchPeer.GetMetric("best_ask").Raw
							}
							if askPrice == 0 {
								askPrice = touchPeer.GetMetric("ask").Raw
							}
						}
						if bidQty == 0 {
							bidQty = touchPeer.GetMetric("touch_quantity:bid").Raw
							if bidQty == 0 {
								bidQty = touchPeer.GetMetric("bid_qty").Raw
							}
						}
						if askQty == 0 {
							askQty = touchPeer.GetMetric("touch_quantity:ask").Raw
							if askQty == 0 {
								askQty = touchPeer.GetMetric("ask_qty").Raw
							}
						}
					}
				}

				side := input.Provenance["side"]
				inBracket := (price >= bidPrice && price <= askPrice)
				if inBracket {
					state.bracketQty += qty
				}

				var bidFillFrac, askFillFrac float64

				if side == "sell" && price == bidPrice {
					state.matchedBidQty += qty
					state.touchFillBidQty += qty
					if bidQty > 0 {
						bidFillFrac = state.touchFillBidQty / bidQty
					}
				}

				if side == "buy" && price == askPrice {
					state.matchedAskQty += qty
					state.touchFillAskQty += qty
					if askQty > 0 {
						askFillFrac = state.touchFillAskQty / askQty
					}
				}

				var bidRate, askRate float64
				var hasRate bool

				if state.hasPrevTime {
					dt := input.At.Sub(state.prevTime).Seconds()
					if dt > 0 {
						bidRate = state.touchFillBidQty / dt
						askRate = state.touchFillAskQty / dt
						hasRate = true
					}
				}

				state.prevTime = input.At
				state.hasPrevTime = true

				var bidReading, askReading adaptive.BaselineReading

				if bidFillFrac > 0 {
					for rPtr := range state.bidBaseline.Next(transport.NewOne(unsafe.Pointer(&bidFillFrac)).Next(nil)) {
						bidReading = *(*adaptive.BaselineReading)(rPtr)
					}
					state.bidFractionSamples++
				}

				if askFillFrac > 0 {
					for rPtr := range state.askBaseline.Next(transport.NewOne(unsafe.Pointer(&askFillFrac)).Next(nil)) {
						askReading = *(*adaptive.BaselineReading)(rPtr)
					}
					state.askFractionSamples++
				}

				m.WriteMetric("bracket_trade_quantity", state.bracketQty)
				m.WriteMetric("matched_touch_trade_quantity:bid", state.matchedBidQty)
				m.WriteMetric("matched_touch_trade_quantity:ask", state.matchedAskQty)
				m.WriteMetric("touch_fill_quantity:bid", state.touchFillBidQty)
				m.WriteMetric("touch_fill_quantity:ask", state.touchFillAskQty)
				m.WriteMetric("touch_fill_fraction:bid", bidFillFrac)
				m.WriteMetric("touch_fill_fraction:ask", askFillFrac)

				if hasRate {
					m.WriteMetric("touch_fill_rate:bid", bidRate)
					m.WriteMetric("touch_fill_rate:ask", askRate)
				}

				if bidFillFrac > 0 && bidReading.HasPrior {
					m.WriteMetric("fill_fraction_baseline:bid", bidReading.Baseline)
					m.WriteMetric("fill_fraction_divergence:bid", bidReading.Residual)
					m.WriteMetric("fill_fraction_zscore:bid", bidReading.ZScore)
				}

				if askFillFrac > 0 && askReading.HasPrior {
					m.WriteMetric("fill_fraction_baseline:ask", askReading.Baseline)
					m.WriteMetric("fill_fraction_divergence:ask", askReading.Residual)
					m.WriteMetric("fill_fraction_zscore:ask", askReading.ZScore)
				}

				m.EnsureMetadata()

				if state.bidFractionSamples >= 3 {
					m.SetMetadata(data.MetadataSupport, strconv.FormatFloat(bidReading.Count, 'f', -1, 64))
					m.SetMetadata(data.MetadataDivergence, strconv.FormatFloat(bidReading.Residual, 'f', -1, 64))

					if bidReading.VarianceDefined {
						m.SetMetadata(data.MetadataNoiseVariance, strconv.FormatFloat(bidReading.Variance, 'f', -1, 64))
					}
				}

				if state.askFractionSamples >= 3 {
					m.SetMetadata(data.MetadataSupport, strconv.FormatFloat(askReading.Count, 'f', -1, 64))
					m.SetMetadata(data.MetadataDivergence, strconv.FormatFloat(askReading.Residual, 'f', -1, 64))

					if askReading.VarianceDefined {
						m.SetMetadata(data.MetadataNoiseVariance, strconv.FormatFloat(askReading.Variance, 'f', -1, 64))
					}
				}

				m.Label = input.Label
				m.At = input.At
				return m
			},
			func(m *data.Measurement[float64], out *data.Measurement[float64]) {},
		),
	)

	actual, _ := trade.pipelines.LoadOrStore(symbol, pipeline)
	return actual.(core.Primitive)
}

/*
Step supplies the arriving measurement to the pipeline and returns it: the
measurement is the pipeline's state, enriched in place.
*/
func (trade *Trade) Step(measurement *data.Measurement[float64]) *data.Measurement[float64] {
	if trade.Status() != runtime.READY {
		errnie.Warn(trade.Name() + ": Step called before READY; dropping event")
		return measurement
	}

	if measurement == nil || measurement.Err != nil {
		return measurement
	}

	if measurement.Err != nil {
		return measurement
	}

	measurement.Source = "toxicity:trade"

	res := data.Read[*data.Measurement[float64]](trade.pipelineFor(measurement.Label).Next(
		transport.NewOne(unsafe.Pointer(&measurement)).Next(nil),
	))

	if res == nil {
		return nil
	}

	res.Finalize()
	return res
}
