package hindsight

import (
	"math"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

/*
quoted builds one captured ticker observation with both sides of the touch
quoted, at an explicit capture sequence, so a test series is addressed by the
same identity the real tape uses.
*/
func quoted(sequence uint64, at time.Time, midpoint, spread, depth float64) Observation {
	return Observation{
		Capture: CaptureIdentity{
			Run:            "run-test",
			Sequence:       CaptureSequence(sequence),
			Stream:         "spot.public",
			StreamEpoch:    1,
			StreamSequence: sequence,
		},
		Symbol:     "TEST/USD",
		Kind:       "ticker",
		ReceivedAt: at,
		VenueAt:    at,
		HasBid:     true,
		Bid:        midpoint - spread/2,
		BidQty:     depth / 2,
		HasAsk:     true,
		Ask:        midpoint + spread/2,
		AskQty:     depth / 2,
	}
}

/*
ramp appends count observations walking value from start to end with a small
alternating perturbation, so the series has a measurable dispersion instead of
the degenerate zero dispersion of a perfectly smooth line.
*/
func ramp(series []Observation, start, end float64, count int, spread, depth float64) []Observation {
	origin := uint64(len(series)) + 1
	base := time.Date(2026, 8, 30, 12, 0, 0, 0, time.UTC)

	for step := range count {
		fraction := float64(step+1) / float64(count)
		value := start + (end-start)*fraction

		if step%2 == 0 {
			value *= 1.0002
		} else {
			value *= 0.9998
		}

		sequence := origin + uint64(step)
		series = append(series, quoted(
			sequence,
			base.Add(time.Duration(sequence)*time.Second),
			value,
			spread,
			depth,
		))
	}

	return series
}

func excursionSeries() []Observation {
	series := make([]Observation, 0, 400)
	series = ramp(series, 100, 100, 100, 0.05, 10)
	series = ramp(series, 100, 120, 100, 0.05, 10)
	series = ramp(series, 120, 100, 100, 0.05, 10)
	series = ramp(series, 100, 105, 100, 0.05, 10)

	return series
}

func TestDiscoverEpisodesExcursionTest(t *testing.T) {
	Convey("Given a captured series holding a rise and a fall", t, func() {
		discovery := DiscoverEpisodes("TEST/USD", excursionSeries(), DefaultDiscoveryPolicy())

		Convey("The declared selector is reported with the result", func() {
			So(discovery.Coordinate, ShouldEqual, CoordinateMidpoint)
			So(discovery.Policy.ExcursionSigmas, ShouldEqual, 3.0)
			So(discovery.QualifyingMove, ShouldBeGreaterThan, 0)
			So(discovery.InsufficientData, ShouldBeFalse)
		})

		Convey("The qualifying move is derived from the symbol's own dispersion", func() {
			So(discovery.HasSigma, ShouldBeTrue)
			So(discovery.Sigma, ShouldBeGreaterThan, 0)
		})

		var upward, downward *Episode

		for index := range discovery.Episodes {
			switch discovery.Episodes[index].Kind {
			case EpisodeUpwardExcursion:
				if upward == nil {
					upward = &discovery.Episodes[index]
				}
			case EpisodeDownwardExcursion:
				if downward == nil {
					downward = &discovery.Episodes[index]
				}
			}
		}

		Convey("The rise is discovered as an upward excursion", func() {
			So(upward, ShouldNotBeNil)
			So(upward.ObservedExcursion, ShouldBeGreaterThan, 0.15)
			So(upward.HasObservedExcursion, ShouldBeTrue)
		})

		Convey("The fall is discovered as a downward excursion", func() {
			So(downward, ShouldNotBeNil)
			So(downward.ObservedExcursion, ShouldBeLessThan, -0.10)
		})

		Convey("An excursion carries an anchor and an extremum addressed by capture identity", func() {
			So(upward, ShouldNotBeNil)

			anchor, found := upward.Reference(ReferenceAnchor)
			So(found, ShouldBeTrue)
			So(anchor.Capture.Valid(), ShouldBeTrue)
			So(anchor.Capture.Sequence, ShouldEqual, upward.FromSequence)

			peak, found := upward.Reference(ReferencePeak)
			So(found, ShouldBeTrue)
			So(peak.Capture.Sequence, ShouldEqual, upward.ToSequence)
			So(peak.Value, ShouldBeGreaterThan, anchor.Value)
		})

		Convey("Opposite consecutive legs are discovered as a reversal", func() {
			var reversal *Episode

			for index := range discovery.Episodes {
				if discovery.Episodes[index].Kind == EpisodeReversal {
					reversal = &discovery.Episodes[index]
					break
				}
			}

			So(reversal, ShouldNotBeNil)

			pivot, found := reversal.Reference(ReferenceReversal)
			So(found, ShouldBeTrue)
			So(pivot.Capture.Sequence, ShouldBeGreaterThan, reversal.FromSequence)
			So(pivot.Capture.Sequence, ShouldBeLessThan, reversal.ToSequence)
		})
	})
}

func TestDiscoverEpisodesOrderingTest(t *testing.T) {
	Convey("Given a captured series whose venue times contradict capture order", t, func() {
		series := excursionSeries()
		reference := DiscoverEpisodes("TEST/USD", series, DefaultDiscoveryPolicy())

		skewed := make([]Observation, len(series))
		copy(skewed, series)

		// Venue time now runs backwards while capture order is unchanged, and
		// the slice itself is handed over shuffled.
		base := time.Date(2026, 8, 30, 12, 0, 0, 0, time.UTC)

		for index := range skewed {
			skewed[index].VenueAt = base.Add(
				time.Duration(len(skewed)-index) * time.Second,
			)
		}

		for index := 0; index+1 < len(skewed); index += 2 {
			skewed[index], skewed[index+1] = skewed[index+1], skewed[index]
		}

		discovery := DiscoverEpisodes("TEST/USD", skewed, DefaultDiscoveryPolicy())

		Convey("Discovery follows capture sequence, never venue time", func() {
			So(len(discovery.Episodes), ShouldEqual, len(reference.Episodes))

			for index := range discovery.Episodes {
				So(discovery.Episodes[index].Kind, ShouldEqual, reference.Episodes[index].Kind)
				So(
					discovery.Episodes[index].FromSequence,
					ShouldEqual,
					reference.Episodes[index].FromSequence,
				)
				So(
					math.Abs(discovery.Episodes[index].ObservedExcursion-
						reference.Episodes[index].ObservedExcursion),
					ShouldBeLessThan,
					1e-12,
				)
			}
		})
	})
}

func TestDiscoverEpisodesUndefinedTest(t *testing.T) {
	Convey("Given captured observations that never quoted a touch", t, func() {
		series := excursionSeries()

		for index := range series {
			if index%3 != 0 {
				continue
			}

			series[index].HasBid = false
			series[index].HasAsk = false
		}

		discovery := DiscoverEpisodes("TEST/USD", series, DefaultDiscoveryPolicy())

		Convey("Unquoted observations are reported undefined, never folded in as zero", func() {
			So(discovery.Undefined, ShouldBeGreaterThan, 0)
			So(discovery.Defined, ShouldEqual, discovery.Observations-discovery.Undefined)

			for _, episode := range discovery.Episodes {
				So(math.Abs(episode.ObservedExcursion), ShouldBeLessThan, 1)
			}
		})
	})
}

func TestDiscoverEpisodesQuietSeriesTest(t *testing.T) {
	Convey("Given a captured series that never moved", t, func() {
		series := make([]Observation, 0, 400)
		series = ramp(series, 100, 100, 400, 0.05, 10)

		discovery := DiscoverEpisodes("TEST/USD", series, DefaultDiscoveryPolicy())

		Convey("No price excursion is manufactured out of quantisation noise", func() {
			for _, episode := range discovery.Episodes {
				So(episode.Kind, ShouldNotEqual, EpisodeUpwardExcursion)
				So(episode.Kind, ShouldNotEqual, EpisodeDownwardExcursion)
				So(episode.Kind, ShouldNotEqual, EpisodeReversal)
			}
		})
	})
}

func TestDiscoverEpisodesInsufficientDataTest(t *testing.T) {
	Convey("Given fewer observations than the selector requires", t, func() {
		series := make([]Observation, 0, 8)
		series = ramp(series, 100, 200, 8, 0.05, 10)

		discovery := DiscoverEpisodes("TEST/USD", series, DefaultDiscoveryPolicy())

		Convey("The selector reports insufficient support instead of guessing", func() {
			So(discovery.InsufficientData, ShouldBeTrue)
			So(discovery.Episodes, ShouldBeEmpty)
		})
	})
}

func TestDiscoverEpisodesSpreadTest(t *testing.T) {
	Convey("Given a captured series whose quoted spread blows out", t, func() {
		series := make([]Observation, 0, 400)
		series = ramp(series, 100, 100, 300, 0.02, 10)
		series = ramp(series, 100, 100, 100, 2.00, 10)

		discovery := DiscoverEpisodes("TEST/USD", series, DefaultDiscoveryPolicy())

		Convey("The blow-out is discovered as a spread expansion", func() {
			var expansion *Episode

			for index := range discovery.Episodes {
				if discovery.Episodes[index].Kind == EpisodeSpreadExpansion {
					expansion = &discovery.Episodes[index]
					break
				}
			}

			So(expansion, ShouldNotBeNil)
			So(expansion.HasRatio, ShouldBeTrue)
			So(expansion.Ratio, ShouldBeGreaterThanOrEqualTo, discovery.Policy.SpreadRatio)

			onset, found := expansion.Reference(ReferenceShockOnset)
			So(found, ShouldBeTrue)
			So(onset.Capture.Sequence, ShouldEqual, expansion.FromSequence)
		})
	})
}

func TestDiscoverEpisodesDepthTest(t *testing.T) {
	Convey("Given a captured series whose quoted touch size collapses", t, func() {
		series := make([]Observation, 0, 400)
		series = ramp(series, 100, 100, 300, 0.02, 100)
		series = ramp(series, 100, 100, 100, 0.02, 5)

		discovery := DiscoverEpisodes("TEST/USD", series, DefaultDiscoveryPolicy())

		Convey("The collapse is discovered as a liquidity collapse", func() {
			var collapse *Episode

			for index := range discovery.Episodes {
				if discovery.Episodes[index].Kind == EpisodeLiquidityCollapse {
					collapse = &discovery.Episodes[index]
					break
				}
			}

			So(collapse, ShouldNotBeNil)
			So(collapse.Ratio, ShouldBeLessThanOrEqualTo, discovery.Policy.DepthRatio)
		})
	})
}

func TestDiscoverEpisodesFutureSelectionTest(t *testing.T) {
	Convey("Given an episode discovered from a captured series", t, func() {
		series := excursionSeries()
		discovery := DiscoverEpisodes("TEST/USD", series, DefaultDiscoveryPolicy())
		So(discovery.Episodes, ShouldNotBeEmpty)

		anchor, found := discovery.Episodes[0].Reference(ReferenceAnchor)
		So(found, ShouldBeTrue)

		Convey("Its anchor names an exact capture identity, not a timestamp neighbourhood", func() {
			So(anchor.Capture.Run, ShouldEqual, RunID("run-test"))
			So(anchor.Capture.StreamEpoch, ShouldEqual, StreamEpoch(1))
			So(anchor.Capture.Sequence, ShouldNotEqual, CaptureSequence(0))
			So(anchor.HasValue, ShouldBeTrue)
		})
	})
}
