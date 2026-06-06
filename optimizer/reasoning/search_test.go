package reasoning

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/market/perspectives/reasoning"
	"github.com/theapemachine/symm/market/perspectives/types"
	"github.com/theapemachine/symm/optimizer/replay"
)

// upLeg is one rally: an ignition signal at the start, a run up to +10%, then a
// pullback past a 2% trail off the peak — so entering on the signal and riding a
// trailing stop locks in the run.
func upLeg(start float64, at time.Time, step time.Duration) []types.Measurement {
	return []types.Measurement{
		{Symbol: "BTC/EUR", Category: types.CategoryVerticalIgnition, SNR: 1.5, Last: start, At: at},
		{Symbol: "BTC/EUR", Last: start * 1.05, At: at.Add(step)},
		{Symbol: "BTC/EUR", Last: start * 1.10, At: at.Add(2 * step)},
		{Symbol: "BTC/EUR", Last: start * 1.07, At: at.Add(3 * step)}, // pulls back through the trail
	}
}

func rallyTape() []types.Measurement {
	base := time.Unix(1_700_000_000, 0)
	step := time.Second
	rows := make([]types.Measurement, 0, 16)

	start := 100.0
	at := base

	for leg := 0; leg < 3; leg++ {
		rows = append(rows, upLeg(start, at, step)...)
		start *= 1.07         // next leg opens near the last exit
		at = at.Add(5 * step) // a spacer tick of slack between legs
	}

	return rows
}

func frictionlessCosts() replay.ReplayCosts {
	return replay.ReplayCosts{
		StartingCapital:  100,
		PositionFraction: 1,
		WalletCurrency:   "EUR",
	}
}

func TestSearchFindsAProfitableStrategy(t *testing.T) {
	Convey("Given a tape of repeated rallies and a frictionless €100 wallet", t, func() {
		rows := rallyTape()
		vocab := DeriveVocabulary(rows)

		result := Search(context.Background(), rows, frictionlessCosts(), SearchConfig{
			BeamWidth: 6,
			MaxRounds: 8,
			Patience:  3,
		})

		Convey("It finds a strategy that makes money", func() {
			So(result.Best.Return, ShouldBeGreaterThan, 0)
			So(result.Best.Trades, ShouldBeGreaterThan, 0)
		})

		Convey("It actually explored beyond the seeds", func() {
			So(result.Evaluated, ShouldBeGreaterThan, len(Seeds(vocab)))
		})

		Convey("The winner is a real, serializable playbook with an entry and management", func() {
			_, hasEntry := entryNode(result.Best.Forest)
			_, hasManagement := managementNode(result.Best.Forest)
			So(hasEntry, ShouldBeTrue)
			So(hasManagement, ShouldBeTrue)

			encoded, err := reasoning.MarshalThoughts(result.Best.Forest, 2)
			So(err, ShouldBeNil)
			So(len(encoded), ShouldBeGreaterThan, 0)
		})

		Convey("The search never regresses below the best seed", func() {
			seedBest := 0.0
			for _, seed := range Seeds(vocab) {
				sim := replay.NewThoughtSimulation(context.Background(), seed, replay.PrecompileTape(rows), frictionlessCosts())
				if score := sim.Result().Score; score > seedBest {
					seedBest = score
				}
			}
			So(result.Best.Return, ShouldBeGreaterThanOrEqualTo, seedBest)
		})
	})
}

func BenchmarkSearch(b *testing.B) {
	rows := rallyTape()
	config := SearchConfig{
		BeamWidth: 4,
		MaxRounds: 3,
		Patience:  2,
	}

	for b.Loop() {
		_ = Search(context.Background(), rows, frictionlessCosts(), config)
	}
}
