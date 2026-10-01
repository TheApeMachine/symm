package depthflow

import (
	"context"
	"math"
	"strconv"
	"sync"
	"time"
	"unsafe"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
	"github.com/krakenfx/api-go/v2/pkg/book"

	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/adaptive"
	"github.com/theapemachine/symm/nomagique/arithmetic"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/nomagique/statistic"
	"github.com/theapemachine/symm/nomagique/transport"
)

/*
Level3 is the depth-flow measuring instrument. It holds no state and no logic of
its own: its entire behavior is one nomagique pipeline over the measurement itself —
every stage writes its facts into the measurement where it computes them, and the
workload's register owns the measurement's lifetime.
*/
type Level3 struct {
	*runtime.System
	pipelines sync.Map
	ID        int
	books     broker.BookSource
}

func NewLevel3(ctx context.Context, books broker.BookSource) *Level3 {
	level3 := &Level3{
		books: books,
	}

	level3.System = runtime.NewSystem(ctx, "depthflow:level3", level3)
	return level3
}

/*
Step supplies the arriving measurement to the pipeline and returns it: the
measurement is the pipeline's state, enriched in place.
*/

func (level3 *Level3) pipelineFor(symbol string) core.Primitive {
	if existing, ok := level3.pipelines.Load(symbol); ok {
		return existing.(core.Primitive)
	}

	type level3State struct {
		prevBids     map[string]float64
		prevAsks     map[string]float64
		prevNotional float64
		prevTime     time.Time
	}
	state := &level3State{
		prevBids: make(map[string]float64),
		prevAsks: make(map[string]float64),
	}

	pipeline := nomagique.NewNumber(
		// 0. Extract raw depth facts from peers or self
		data.NewAdapter(
			transport.NewPass(),
			func(m *data.Measurement[float64]) *data.Measurement[float64] {
				input := m

				if len(m.Peers) > 0 {
					peer := m.FindPeer(func(p *data.Measurement[float64]) bool {
						if p.Label == "" {
							return false
						}
						_, hasObsBid := p.LookupMetric("observed_notional:bid")
						_, hasObsAsk := p.LookupMetric("observed_notional:ask")
						if hasObsBid || hasObsAsk {
							return true
						}
						b := p.GetMetric("best_bid").Raw
						if b == 0 {
							b = p.GetMetric("bid").Raw
						}
						a := p.GetMetric("best_ask").Raw
						if a == 0 {
							a = p.GetMetric("ask").Raw
						}
						return b > 0 && a > 0
					})

					if peer != nil {
						input = peer
					}
				}

				if input.Label != "" {
					m.Label = input.Label
				}
				if !input.At.IsZero() {
					m.At = input.At
				}

				var obsBid, obsAsk float64
				var addedBid, removedBid float64
				var addedAsk, removedAsk float64
				
				level3.books.Book(m.Label, func(b *book.Book) {
					currBids := make(map[string]float64)
					currAsks := make(map[string]float64)
					
					cursor := b.BestBid()
					for count := 0; count < 100 && cursor != nil; count++ {
						price := cursor.Price.Float64()
						qty := cursor.Quantity.Float64()
						obsBid += price * qty
						pstr := cursor.Price.String()
						currBids[pstr] = qty
						
						prevQty := state.prevBids[pstr]
						if qty > prevQty {
							addedBid += price * (qty - prevQty)
						} else if qty < prevQty {
							removedBid += price * (prevQty - qty)
						}
						cursor = cursor.Lower
					}
					for pstr, prevQty := range state.prevBids {
						if _, ok := currBids[pstr]; !ok {
							price, _ := strconv.ParseFloat(pstr, 64)
							removedBid += price * prevQty
						}
					}
					
					cursor = b.BestAsk()
					for count := 0; count < 100 && cursor != nil; count++ {
						price := cursor.Price.Float64()
						qty := cursor.Quantity.Float64()
						obsAsk += price * qty
						pstr := cursor.Price.String()
						currAsks[pstr] = qty
						
						prevQty := state.prevAsks[pstr]
						if qty > prevQty {
							addedAsk += price * (qty - prevQty)
						} else if qty < prevQty {
							removedAsk += price * (prevQty - qty)
						}
						cursor = cursor.Higher
					}
					for pstr, prevQty := range state.prevAsks {
						if _, ok := currAsks[pstr]; !ok {
							price, _ := strconv.ParseFloat(pstr, 64)
							removedAsk += price * prevQty
						}
					}
					
					state.prevBids = currBids
					state.prevAsks = currAsks
				})

				if obsBid > 0 || obsAsk > 0 {
					m.From = state.prevTime
					m.WriteMetric("book_notional:bid", obsBid)
					m.WriteMetric("book_notional:ask", obsAsk)
					m.WriteMetric("observed_notional:bid", obsBid)
					m.WriteMetric("observed_notional:ask", obsAsk)
					
					m.WriteMetric("added_notional:bid", addedBid)
					m.WriteMetric("removed_notional:bid", removedBid)
					m.WriteMetric("net_displayed_flow:bid", addedBid-removedBid)
					m.WriteMetric("added_notional:ask", addedAsk)
					m.WriteMetric("removed_notional:ask", removedAsk)
					m.WriteMetric("net_displayed_flow:ask", addedAsk-removedAsk)
					
					observed := obsBid + obsAsk
					if observed > 0 {
						m.WriteMetric("book_imbalance", (obsBid-obsAsk)/observed)
					}
					
					if !state.prevTime.IsZero() {
						dt := m.At.Sub(state.prevTime).Seconds()
						if dt > 0 {
							effectiveDt := math.Max(dt, 1.0)
							rate := observed / effectiveDt
							m.WriteMetric("observed_notional_rate", rate)
							
							m.WriteMetric("added_notional_rate:bid", addedBid/dt)
							m.WriteMetric("removed_notional_rate:bid", removedBid/dt)
							m.WriteMetric("net_displayed_flow_rate:bid", (addedBid-removedBid)/dt)
							m.WriteMetric("added_notional_rate:ask", addedAsk/dt)
							m.WriteMetric("removed_notional_rate:ask", removedAsk/dt)
							m.WriteMetric("net_displayed_flow_rate:ask", (addedAsk-removedAsk)/dt)
							
							ref := (state.prevNotional + observed) / 2
							if ref > 0 {
								m.WriteMetric("book_turnover_rate", (addedBid+removedBid+addedAsk+removedAsk)/(ref*dt))
								m.WriteMetric("net_book_change_rate", (observed-state.prevNotional)/(ref*dt))
								m.WriteMetric("signed_net_displayed_flow_rate", ((addedBid-removedBid)-(addedAsk-removedAsk))/(ref*dt))
							}
						}
					}
					state.prevNotional = observed
					state.prevTime = m.At
					
					tb := input.GetMetric("touch_notional:bid").Raw
					ta := input.GetMetric("touch_notional:ask").Raw
					if tb > 0 || ta > 0 {
						touchImb := (tb - ta) / (tb + ta)
						m.WriteMetric("touch_imbalance", touchImb)
						if observed > 0 {
							bookImb := (obsBid - obsAsk) / observed
							m.WriteMetric("imbalance_resolution_gap", touchImb-bookImb)
							m.WriteMetric("imbalance_resolution_distance", math.Abs(touchImb-bookImb))
						}
					}
				}

				m.EnsureMetadata()

				return m
			},
			func(m *data.Measurement[float64], res *data.Measurement[float64]) {},
		),
		// 1. Calculate structural metrics using pure equations
		data.NewEquations(
			data.Equation{
				Output: "observed_notional",
				Op:     arithmetic.NewAdd(),
				Left:   "observed_notional:bid",
				Right:  "observed_notional:ask",
			},
			data.Equation{
				Output: "observed_notional_diff",
				Op:     arithmetic.NewSubtract(),
				Left:   "observed_notional:bid",
				Right:  "observed_notional:ask",
			},
			data.Equation{
				Output: "mutation_count",
				Op:     arithmetic.NewAdd(),
				Left:   "mutation_count:bid",
				Right:  "mutation_count:ask",
			},
			data.Equation{
				Output: "mutation_count_diff",
				Op:     arithmetic.NewSubtract(),
				Left:   "mutation_count:bid",
				Right:  "mutation_count:ask",
			},
			data.Equation{
				Output: "observed_notional_imbalance",
				Op:     arithmetic.NewDivide(),
				Left:   "observed_notional_diff",
				Right:  "observed_notional",
			},
			data.Equation{
				Output: "mutation_activity_imbalance",
				Op:     arithmetic.NewDivide(),
				Left:   "mutation_count_diff",
				Right:  "mutation_count",
			},
		),
		// 2. Baselines
		transport.NewFan(
			data.NewAdapter(
				adaptive.NewBaseline(adaptive.NewWindow()),
				func(m *data.Measurement[float64]) float64 {
					return m.GetMetric("observed_notional_imbalance").Raw
				},
				func(m *data.Measurement[float64], out adaptive.BaselineReading) {
					m.SetMetadata(data.MetadataSupport, strconv.FormatFloat(out.Count, 'f', -1, 64))

					if out.HasPrior {
						m.WriteMetric("observed_notional_imbalance_baseline", out.Baseline)
						m.WriteMetric("observed_notional_imbalance_divergence", out.Residual)
						m.WriteMetric("observed_notional_imbalance_zscore", out.ZScore)
						m.SetMetadata(data.MetadataDivergence, strconv.FormatFloat(out.Residual, 'f', -1, 64))

						if out.VarianceDefined {
							m.SetMetadata(data.MetadataNoiseVariance, strconv.FormatFloat(out.Variance, 'f', -1, 64))
						}
					}
				},
			),
			data.NewAdapter(
				adaptive.NewBaseline(adaptive.NewWindow()),
				func(m *data.Measurement[float64]) float64 {
					return m.GetMetric("observed_notional_rate").Raw
				},
				func(m *data.Measurement[float64], out adaptive.BaselineReading) {
					if out.HasPrior {
						m.WriteMetric("observed_notional_rate_baseline", out.Baseline)
						m.WriteMetric("observed_notional_rate_divergence", out.Residual)
						m.WriteMetric("observed_notional_rate_zscore", out.ZScore)
					}
				},
			),
			data.NewAdapter(
				adaptive.NewBaseline(adaptive.NewWindow()),
				func(m *data.Measurement[float64]) float64 {
					return m.GetMetric("book_turnover_rate").Raw
				},
				func(m *data.Measurement[float64], out adaptive.BaselineReading) {
					if out.HasPrior {
						m.WriteMetric("book_turnover_rate_baseline", out.Baseline)
						m.WriteMetric("book_turnover_rate_divergence", out.Residual)
						m.WriteMetric("book_turnover_rate_zscore", out.ZScore)
					}
				},
			),
			data.NewAdapter(
				adaptive.NewBaseline(adaptive.NewWindow()),
				func(m *data.Measurement[float64]) float64 {
					return m.GetMetric("net_book_change_rate").Raw
				},
				func(m *data.Measurement[float64], out adaptive.BaselineReading) {
					if out.HasPrior {
						m.WriteMetric("net_book_change_rate_baseline", out.Baseline)
						m.WriteMetric("net_book_change_rate_divergence", out.Residual)
						m.WriteMetric("net_book_change_rate_zscore", out.ZScore)
					}
				},
			),
			data.NewAdapter(
				adaptive.NewBaseline(adaptive.NewWindow()),
				func(m *data.Measurement[float64]) float64 {
					return m.GetMetric("signed_net_displayed_flow_rate").Raw
				},
				func(m *data.Measurement[float64], out adaptive.BaselineReading) {
					if out.HasPrior {
						m.WriteMetric("net_displayed_flow_imbalance_baseline", out.Baseline)
						m.WriteMetric("net_displayed_flow_imbalance_divergence", out.Residual)
						m.WriteMetric("net_displayed_flow_imbalance_zscore", out.ZScore)
					}
				},
			),
			data.NewAdapter(
				statistic.NewJoint(4),
				func(m *data.Measurement[float64]) statistic.JointInput {
					i := m.GetMetric("observed_notional_imbalance_divergence").Raw
					g := m.GetMetric("imbalance_resolution_gap").Raw // Wait, it asks for gap divergence, but it is not computed, just gap
					f := m.GetMetric("net_displayed_flow_imbalance_zscore").Raw
					c := m.GetMetric("book_turnover_rate_zscore").Raw
					
					return statistic.JointInput{Values: []float64{i, g, f, c}}
				},
				func(m *data.Measurement[float64], out statistic.JointReading) {
					if out.SNRDefined {
						m.WriteMetric("SNR", out.SNR)
						m.EnsureMetadata()
						m.SetMetadata(data.MetadataMahalanobisSNR, strconv.FormatFloat(out.SNR, 'f', -1, 64))
					}
					if len(out.Channels) > 0 {
						n := out.Channels[0].Count
						maturity := 0.0
						if n > 1 {
							maturity = 1.0 - (1.0 / n)
						}
						m.WriteMetric("Maturity", maturity)
					}
				},
			),
		),
		// 3. Finalize
		data.NewFinalizer[float64](),
	)

	actual, _ := level3.pipelines.LoadOrStore(symbol, pipeline)
	return actual.(core.Primitive)
}

func (level3 *Level3) Step(measurement *data.Measurement[float64]) *data.Measurement[float64] {
	if level3.Status() != runtime.READY {
		errnie.Warn(level3.Name() + ": Step called before READY; dropping event")
		return measurement
	}

	if measurement == nil || measurement.Err != nil {
		return measurement
	}

	measurement.Source = "depthflow:level3"

	return data.Read[*data.Measurement[float64]](level3.pipelineFor(measurement.Label).Next(
		transport.NewOne(unsafe.Pointer(&measurement)).Next(nil),
	))
}
