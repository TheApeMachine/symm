package sentiment

import (
	"context"
	"math"
	"sort"
	"strconv"
	"sync"
	"unsafe"

	"github.com/theapemachine/errnie"

	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/adaptive"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/crosssection"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/nomagique/statistic"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/transport"
)

type Ticker struct {
	*runtime.System
	arena     *data.ArenaOwner
	pipelines sync.Map
	prices    *store.Latest[string, float64]
	changes   *store.Latest[string, data.CrossMember]
	ID        int
}

func NewTicker(ctx context.Context, arena *data.ArenaOwner) *Ticker {
	ticker := &Ticker{
		arena:   arena,
		prices:  store.NewLatest[string, float64](),
		changes: store.NewLatest[string, data.CrossMember](),
	}

	ticker.System = runtime.NewSystem(ctx, "sentiment:ticker", ticker)
	return ticker
}

func (ticker *Ticker) Source() string {
	return "sentiment"
}

func (ticker *Ticker) Arena() *data.ArenaOwner {
	return ticker.arena
}

func (ticker *Ticker) pipelineFor(symbol string) core.Primitive {
	if existing, ok := ticker.pipelines.Load(symbol); ok {
		return existing.(core.Primitive)
	}

	pipeline := nomagique.NewNumber(
		data.NewMetricGate("last"),
		crosssection.NewUpdateMember("last", ticker.prices, ticker.changes),
		crosssection.NewStampPeers(ticker.changes),
		data.NewAdapter(
			transport.NewPass(),
			func(m *data.Measurement[float64]) *data.Measurement[float64] {
				if len(m.Peers) == 0 {
					return m
				}

				positive, negative, zero := 0.0, 0.0, 0.0
				var extremeKey string
				var maxAbsChange float64

				var changes []float64
				var absChanges []float64

				var sameDir, oppDir, zeroDir float64

				var focalChange float64
				var hasFocal bool

				for _, peer := range m.Peers {
					if peer.Label == m.Label {
						focalChange = peer.GetMetric("change").Raw
						hasFocal = true
					}
				}

				var maxAsofAge, medianAsofAge, medianFromAge float64
				var asofAges []float64
				var fromAges []float64

				for _, peer := range m.Peers {
					change := peer.GetMetric("change").Raw
					absChange := math.Abs(change)

					changes = append(changes, change)
					absChanges = append(absChanges, absChange)

					if absChange > maxAbsChange || extremeKey == "" {
						maxAbsChange = absChange
						extremeKey = peer.Label
					}

					switch {
					case change > 0:
						positive++
					case change < 0:
						negative++
					default:
						zero++
					}

					if hasFocal {
						if (change > 0 && focalChange > 0) || (change < 0 && focalChange < 0) {
							sameDir++
						} else if (change > 0 && focalChange < 0) || (change < 0 && focalChange > 0) {
							oppDir++
						} else {
							zeroDir++
						}
					}

					asofAge := m.At.Sub(peer.At).Seconds()
					fromAge := m.At.Sub(peer.From).Seconds()

					if asofAge > maxAsofAge {
						maxAsofAge = asofAge
					}

					asofAges = append(asofAges, asofAge)
					fromAges = append(fromAges, fromAge)
				}

				valid := positive + negative + zero
				if valid > 0 {
					m.WriteMetric("valid_member_count", valid)
					m.WriteMetric("positive_count", positive)
					m.WriteMetric("negative_count", negative)
					m.WriteMetric("zero_count", zero)

					advanceFraction := positive / valid
					declineFraction := negative / valid
					m.WriteNormalized("advance_fraction", advanceFraction)
					m.WriteNormalized("decline_fraction", declineFraction)
					m.WriteNormalized("unchanged_fraction", zero/valid)
					m.WriteNormalized("advance_decline_spread", advanceFraction-declineFraction)

					m.WriteNormalized("signed_fraction", (positive-negative)/valid) // legacy support

					m.WriteNormalized("directional_participation", (positive+negative)/valid)

					dirConsensus := 0.0
					if positive+negative > 0 {
						dirConsensus = math.Abs(positive-negative) / (positive + negative)
					}
					m.WriteNormalized("directional_consensus", dirConsensus)
					m.WriteNormalized("directional_agreement", dirConsensus*((positive+negative)/valid))

					if extremeKey != "" {
						m.EnsureMetadata()
						m.SetProvenance("extreme_key", extremeKey)
					}

					sort.Float64s(changes)
					sort.Float64s(absChanges)
					sort.Float64s(asofAges)
					sort.Float64s(fromAges)

					mid := len(changes) / 2
					var medianReturn, medianAbsReturn float64
					if len(changes)%2 == 0 {
						medianReturn = (changes[mid-1] + changes[mid]) / 2.0
						medianAbsReturn = (absChanges[mid-1] + absChanges[mid]) / 2.0
						medianAsofAge = (asofAges[mid-1] + asofAges[mid]) / 2.0
						medianFromAge = (fromAges[mid-1] + fromAges[mid]) / 2.0
					} else {
						medianReturn = changes[mid]
						medianAbsReturn = absChanges[mid]
						medianAsofAge = asofAges[mid]
						medianFromAge = fromAges[mid]
					}

					m.WriteMetric("signed_median", medianReturn)
					m.WriteMetric("median_return", medianReturn)
					m.WriteMetric("median_absolute_return", medianAbsReturn)
					m.WriteMetric("largest_absolute_return", maxAbsChange)
					m.WriteMetric("median_asof_age_seconds", medianAsofAge)
					m.WriteMetric("median_from_age_seconds", medianFromAge)
					m.WriteMetric("max_asof_age_seconds", maxAsofAge)

					var sumAbs, sumSq float64
					for _, c := range absChanges {
						sumAbs += c
						sumSq += c * c
					}

					meanAbs := sumAbs / valid
					rms := math.Sqrt(sumSq / valid)
					m.WriteMetric("mean_absolute_return", meanAbs)
					m.WriteMetric("rms_return", rms)

					var sumMad, sumMagMad float64
					for _, c := range changes {
						sumMad += math.Abs(c - medianReturn)
					}
					for _, c := range absChanges {
						sumMagMad += math.Abs(c - medianAbsReturn)
					}

					m.WriteMetric("return_mad", sumMad/valid)
					m.WriteMetric("magnitude_mad", sumMagMad/valid)

					q1Idx := len(changes) / 4
					q3Idx := (len(changes) * 3) / 4
					m.WriteMetric("return_interquartile_range", changes[q3Idx]-changes[q1Idx])

					if sumAbs > 0 {
						m.WriteNormalized("largest_move_share", maxAbsChange/sumAbs)
					}

					if hasFocal {
						m.WriteMetric("absolute_return", math.Abs(focalChange))

						m.WriteMetric("same_direction_peer_count", sameDir)
						m.WriteMetric("opposite_direction_peer_count", oppDir)
						m.WriteMetric("zero_return_peer_count", zeroDir)

						m.WriteNormalized("same_direction_peer_fraction", sameDir/valid)
						m.WriteNormalized("opposite_direction_peer_fraction", oppDir/valid)
						m.WriteNormalized("zero_return_peer_fraction", zeroDir/valid)
					}
				}

				return m
			},
			func(m *data.Measurement[float64], out *data.Measurement[float64]) {},
		),
		transport.NewFan(
			data.NewAdapter(
				adaptive.NewBaseline(adaptive.NewWindow()),
				func(m *data.Measurement[float64]) float64 {
					return m.GetMetric("median_return").Raw
				},
				func(m *data.Measurement[float64], out adaptive.BaselineReading) {
					if out.HasPrior {
						m.WriteMetric("median_return_baseline", out.Baseline)
						m.WriteMetric("median_return_divergence", out.Residual)
						m.WriteStandardized("median_return_zscore", out.ZScore)
					}
				},
			),
			data.NewAdapter(
				adaptive.NewBaseline(adaptive.NewWindow()),
				func(m *data.Measurement[float64]) float64 {
					return m.GetMetric("advance_fraction").Raw
				},
				func(m *data.Measurement[float64], out adaptive.BaselineReading) {
					if out.HasPrior {
						m.WriteMetric("breadth_baseline", out.Baseline)
						m.WriteMetric("breadth_divergence", out.Residual)
						m.WriteStandardized("breadth_zscore", out.ZScore)
					}
				},
			),
			data.NewAdapter(
				adaptive.NewBaseline(adaptive.NewWindow()),
				func(m *data.Measurement[float64]) float64 {
					return m.GetMetric("median_absolute_return").Raw
				},
				func(m *data.Measurement[float64], out adaptive.BaselineReading) {
					if out.HasPrior {
						m.WriteMetric("median_absolute_return_baseline", out.Baseline)
						if out.Baseline > 0 {
							m.WriteMetric("median_absolute_return_ratio", m.GetMetric("median_absolute_return").Raw/out.Baseline)
						}
						m.WriteStandardized("median_absolute_return_zscore", out.ZScore)
					}
				},
			),
			data.NewAdapter(
				adaptive.NewBaseline(adaptive.NewWindow()),
				func(m *data.Measurement[float64]) float64 {
					return m.GetMetric("return_mad").Raw
				},
				func(m *data.Measurement[float64], out adaptive.BaselineReading) {
					if out.HasPrior {
						m.WriteMetric("return_dispersion_baseline", out.Baseline)
						if out.Baseline > 0 {
							m.WriteMetric("return_dispersion_ratio", m.GetMetric("return_mad").Raw/out.Baseline)
						}
						m.WriteStandardized("return_dispersion_zscore", out.ZScore)
					}
				},
			),
			data.NewAdapter(
				statistic.NewJoint(4),
				func(m *data.Measurement[float64]) statistic.JointInput {
					mb := m.GetMetric("median_return_zscore").Raw
					br := m.GetMetric("breadth_zscore").Raw
					ma := m.GetMetric("median_absolute_return_zscore").Raw
					rd := m.GetMetric("return_dispersion_zscore").Raw
					if mb == 0 && br == 0 && ma == 0 && rd == 0 {
						return statistic.JointInput{Values: nil}
					}
					return statistic.JointInput{Values: []float64{mb, br, ma, rd}}
				},
				func(m *data.Measurement[float64], out statistic.JointReading) {
					if out.SNRDefined && out.SNR < 1/math.Sqrt(2.220446049250313e-16) {
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
						m.WriteNormalized("Maturity", maturity)
					}
				},
			),
		),
		data.NewFinalizer[float64](),
	)

	actual, _ := ticker.pipelines.LoadOrStore(symbol, pipeline)
	return actual.(core.Primitive)
}

func (ticker *Ticker) Step(prior *data.Measurement[float64]) *data.Measurement[float64] {
	if ticker.Status() != runtime.READY {
		errnie.Warn(ticker.Name() + ": Step called before READY; dropping event")
		return nil
	}

	if prior == nil || prior.Label == "" {
		return nil
	}

	price := quotedPrice(prior)
	if price <= 0 {
		return nil
	}

	out := ticker.arena.NewMeasurement(ticker.Source())
	out.Label = prior.Label
	out.SeqIdx = prior.SeqIdx
	out.At = prior.At
	out.From = prior.From
	out.Peers = []*data.Measurement[float64]{prior}

	out.WriteMetric("last", price)

	if channel, hasCh := prior.GetProvenance("channel"); hasCh {
		out.SetProvenance("channel", channel)
	}

	res := data.Read[*data.Measurement[float64]](ticker.pipelineFor(out.Label).Next(
		transport.NewOne(unsafe.Pointer(&out)).Next(nil),
	))

	if res == nil {
		return out
	}

	return res
}

func quotedPrice(measurement *data.Measurement[float64]) float64 {
	for _, key := range []string{"last", "last_price", "price"} {
		if metric, ok := measurement.LookupMetric(key); ok && metric.Raw > 0 {
			return metric.Raw
		}
	}

	return 0
}
