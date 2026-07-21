package manifold

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/nomagique/geometry"
	pfluid "github.com/theapemachine/nomagique/physics/fluid"
	"github.com/theapemachine/symm/types"
)

/*
TestPhaseCorpus_CommitPhase proves that only exact ready cognitive outcomes
enter a symbol-scoped phase history.
*/
func TestPhaseCorpus_CommitPhase(t *testing.T) {
	Convey("Given a pending focused resident fingerprint", t, func() {
		corpus, err := NewPhaseCorpus(8)
		So(err, ShouldBeNil)
		at := time.Unix(3, 0)
		dial := geometry.PhaseDial{1 + 1i, 2 - 1i}
		corpus.Stage("BTC/USD", at, dial)

		Convey("It should retain only a ready outcome from the exact symbol epoch", func() {
			So(corpus.CommitPhase(types.Cognition{
				Symbol: "BTC/USD",
				At:     at,
			}), ShouldBeNil)
			So(corpus.spectra, ShouldBeEmpty)
			So(corpus.CommitPhase(types.Cognition{
				Symbol: "BTC/USD",
				At:     at.Add(time.Second),
				Winner: "sell",
				Ready:  true,
			}), ShouldBeNil)
			So(corpus.spectra, ShouldBeEmpty)

			outcome := types.Cognition{
				Symbol:     "BTC/USD",
				At:         at,
				Winner:     "buy",
				Ready:      true,
				Confidence: 0.75,
				Ambiguous:  true,
				Cohort:     12,
			}
			So(corpus.CommitPhase(outcome), ShouldBeNil)
			So(corpus.spectra["BTC/USD"].Size(), ShouldEqual, 1)
			So(corpus.pending, ShouldBeEmpty)

			responses, err := corpus.Responses("BTC/USD", dial, at.Add(time.Second))
			So(err, ShouldBeNil)
			So(responses, ShouldHaveLength, len(dial))
			So(responses[0].Outcome, ShouldResemble, PhaseOutcome{
				Symbol:     outcome.Symbol,
				Class:      outcome.Winner,
				Confidence: outcome.Confidence,
				Ambiguous:  outcome.Ambiguous,
				Cohort:     outcome.Cohort,
			})
			other, err := corpus.Responses("ETH/USD", dial, at.Add(time.Second))
			So(err, ShouldBeNil)
			So(other, ShouldBeEmpty)
		})
	})
}

/*
BenchmarkPhaseCorpus_CommitPhase measures the causal stage-and-label insertion
path at the configured live wave dimensionality.
*/
func BenchmarkPhaseCorpus_CommitPhase(b *testing.B) {
	corpus, err := NewPhaseCorpus(256)

	if err != nil {
		b.Fatal(err)
	}

	dial := benchmarkPhaseDial()
	sequence := int64(0)
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		sequence++
		at := time.Unix(0, sequence)
		corpus.Stage("BTC/USD", at, dial)

		if err := corpus.CommitPhase(types.Cognition{
			Symbol: "BTC/USD",
			At:     at,
			Winner: "buy",
			Ready:  true,
		}); err != nil {
			b.Fatal(err)
		}
	}
}

/*
BenchmarkPhaseCorpus_Responses measures a complete categorical phase turn over
a full retained event history at the live wave dimensionality.
*/
func BenchmarkPhaseCorpus_Responses(b *testing.B) {
	corpus, err := NewPhaseCorpus(256)

	if err != nil {
		b.Fatal(err)
	}

	dial := benchmarkPhaseDial()

	for sequence := int64(1); sequence <= int64(corpus.capacity); sequence++ {
		at := time.Unix(0, sequence)
		corpus.Stage("BTC/USD", at, dial)

		if err := corpus.CommitPhase(types.Cognition{
			Symbol: "BTC/USD",
			At:     at,
			Winner: "buy",
			Ready:  true,
		}); err != nil {
			b.Fatal(err)
		}
	}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if _, err := corpus.Responses("BTC/USD", dial, time.Time{}); err != nil {
			b.Fatal(err)
		}
	}
}

/*
benchmarkPhaseDial creates a finite nonzero fingerprint at the production
domain's configured omega resolution.
*/
func benchmarkPhaseDial() geometry.PhaseDial {
	dial := make(geometry.PhaseDial, pfluid.DefaultConfig().Grid.X)

	for index := range dial {
		dial[index] = complex(float64(index+1), float64(len(dial)-index))
	}

	return dial
}
