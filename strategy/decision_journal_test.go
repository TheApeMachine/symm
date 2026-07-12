package strategy

import (
	"fmt"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/types"
)

func TestDecisionJournalRecord(t *testing.T) {
	Convey("Given an empty DecisionJournal", t, func() {
		journal := NewDecisionJournal()
		first := testDecision(1, ActionBuy, 0.05)
		second := testDecision(2, ActionHold, 0.0)

		Convey("When decisions from successive forecast epochs are recorded", func() {
			firstRecorded, firstErr := journal.Record(first)
			secondRecorded, secondErr := journal.Record(second)
			duplicateRecorded, duplicateErr := journal.Record(first)
			first.Alternatives[0].Utility = 99.0

			Convey("Then history is chronological, copied, and unique per epoch", func() {
				So(firstErr, ShouldBeNil)
				So(secondErr, ShouldBeNil)
				So(duplicateErr, ShouldBeNil)
				So(firstRecorded, ShouldBeTrue)
				So(secondRecorded, ShouldBeTrue)
				So(duplicateRecorded, ShouldBeFalse)

				decisions := journal.Decisions("BTC/USD")
				So(decisions, ShouldHaveLength, 2)
				So(decisions[0].Forecast.SourceEpoch, ShouldEqual, 1)
				So(decisions[1].Forecast.SourceEpoch, ShouldEqual, 2)
				So(decisions[0].Alternatives[0].Utility, ShouldEqual, 0.05)
			})
		})
	})
}

func TestDecisionJournalEvaluated(t *testing.T) {
	Convey("Given a DecisionJournal containing one forecast evaluation", t, func() {
		journal := NewDecisionJournal()
		decision := testDecision(1, ActionBuy, 0.05)
		_, err := journal.Record(decision)
		So(err, ShouldBeNil)

		Convey("When the same and a newer epoch are queried", func() {
			Convey("Then only the recorded epoch reports as evaluated", func() {
				So(
					journal.Evaluated("BTC/USD", decision.Forecast),
					ShouldBeTrue,
				)

				newer := decision.Forecast
				newer.SourceEpoch++
				So(journal.Evaluated("BTC/USD", newer), ShouldBeFalse)
			})
		})
	})
}

func TestDecisionJournalDecisions(t *testing.T) {
	Convey("Given a DecisionJournal containing a decision", t, func() {
		journal := NewDecisionJournal()
		_, err := journal.Record(testDecision(1, ActionBuy, 0.05))
		So(err, ShouldBeNil)

		Convey("When a reader mutates the returned decision", func() {
			decisions := journal.Decisions("BTC/USD")
			decisions[0].Alternatives[0].Utility = 99.0

			Convey("Then the append-only journal remains unchanged", func() {
				loaded := journal.Decisions("BTC/USD")
				So(loaded[0].Alternatives[0].Utility, ShouldEqual, 0.05)
			})
		})
	})
}

func TestDecisionJournalRecordChronology(t *testing.T) {
	Convey("Given a decision journal whose latest evaluation is newer", t, func() {
		journal := NewDecisionJournal()
		_, err := journal.Record(testDecision(2, ActionBuy, 0.05))
		So(err, ShouldBeNil)
		older := testDecision(1, ActionBuy, 0.04)

		Convey("When an older logical evaluation is recorded", func() {
			recorded, err := journal.Record(older)

			Convey("Then strategy fails explicitly without rewriting its history", func() {
				So(recorded, ShouldBeFalse)
				So(err, ShouldNotBeNil)
				So(journal.Decisions("BTC/USD"), ShouldHaveLength, 1)
			})
		})
	})
}

func BenchmarkDecisionJournalRecord(b *testing.B) {
	const symbols = 1455
	decisions := make([]Decision, symbols)

	for index := range symbols {
		decisions[index] = testDecision(1, ActionBuy, float64(index))
		decisions[index].Symbol = fmt.Sprintf("ASSET-%04d/USD", index)
		decisions[index].Forecast.Symbol = decisions[index].Symbol
	}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		journal := NewDecisionJournal()

		for _, decision := range decisions {
			recorded, err := journal.Record(decision)

			if err != nil {
				b.Fatal(err)
			}

			if !recorded {
				b.Fatal("decision epoch was not recorded")
			}
		}
	}
}

func testDecision(
	sourceEpoch uint64,
	action Action,
	utility float64,
) Decision {
	return Decision{
		At:      time.Unix(int64(sourceEpoch), 0),
		Symbol:  "BTC/USD",
		Action:  action,
		Utility: utility,
		Alternatives: []Alternative{
			{Action: ActionBuy, Utility: utility},
			{Action: ActionHold, Utility: 0.0},
		},
		Forecast: types.Forecasts{
			Source:        "manifold_forecast",
			Symbol:        "BTC/USD",
			SourceEpoch:   sourceEpoch,
			Target:        "next_l3_epoch_mid_log_return",
			ModelVersion:  "test",
			At:            time.Unix(int64(sourceEpoch), 0),
			HorizonEvents: 1,
			ExpiresEpoch:  sourceEpoch + 1,
			Ready:         true,
			Confidence:    0.8,
		},
		Reason: "test evaluation",
	}
}
