package trader

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/market"
)

func TestDecisionChoose(t *testing.T) {
	Convey("Given a trader decision gate requiring measured signal agreement", t, func() {
		at := time.Now().UTC()
		story := market.NewStory(context.Background())
		viper.Set("trading.sizing.base_fraction", 0.05)
		viper.Set("trading.entry.min_active_origins", 2)
		viper.Set("market.story.measurement_max_age", time.Minute)
		decision, err := NewDecision()
		action := &logic.Action{
			Type:            logic.ActionMarket,
			Side:            logic.SideBuy,
			Symbol:          "BTC/USD",
			EntryScore:      0.6,
			EntryConfidence: 0.8,
		}

		So(err, ShouldBeNil)

		Convey("When only one active signal origin backs the candidate", func() {
			err = story.Update([]*logic.Measurement{{
				Source: logic.SourceCVD,
				Symbol: "BTC/USD",
				At:     at,
				Distribution: map[logic.CategoryType]float64{
					logic.CategoryAggressiveDrive: 1,
				},
				Confidence:    0.8,
				Strength:      0.8,
				EntryBaseline: 0.5,
				ExitBaseline:  0.5,
				Status:        "measured",
			}})
			selected, selectErr := decision.Choose(
				[]*logic.Action{action},
				story,
				at,
			)

			Convey("Then Decision.Choose blocks the trade before broker dispatch", func() {
				So(err, ShouldBeNil)
				So(selectErr, ShouldBeNil)
				So(selected, ShouldHaveLength, 0)
				So(action.Allowed, ShouldBeFalse)
				So(action.Verdict, ShouldEqual, "blocked")
				So(action.Reason, ShouldEqual, "insufficient active signals 1/2")
			})
		})

		Convey("When the configured number of measured origins backs the candidate", func() {
			err = story.Update([]*logic.Measurement{
				{
					Source: logic.SourceCVD,
					Symbol: "BTC/USD",
					At:     at,
					Distribution: map[logic.CategoryType]float64{
						logic.CategoryAggressiveDrive: 1,
					},
					Confidence:    0.8,
					Strength:      0.8,
					EntryBaseline: 0.5,
					ExitBaseline:  0.5,
					Status:        "measured",
				},
				{
					Source: logic.SourceResonance,
					Symbol: "BTC/USD",
					At:     at,
					Distribution: map[logic.CategoryType]float64{
						logic.CategoryLaminar: 1,
					},
					Confidence:    0.7,
					Strength:      0.7,
					EntryBaseline: 0.5,
					ExitBaseline:  0.5,
					Status:        "measured",
				},
			})
			selected, selectErr := decision.Choose(
				[]*logic.Action{action},
				story,
				at,
			)

			Convey("Then Decision.Choose admits the selected candidate", func() {
				So(err, ShouldBeNil)
				So(selectErr, ShouldBeNil)
				So(selected, ShouldHaveLength, 1)
				So(selected[0], ShouldEqual, action)
				So(action.Allowed, ShouldBeTrue)
				So(action.Verdict, ShouldEqual, "allow")
				So(action.Fraction, ShouldEqual, 0.05)
			})
		})

		Convey("When raw confidence and normalized score disagree across candidates", func() {
			err = story.Update([]*logic.Measurement{
				{
					Source: logic.SourceCVD,
					Symbol: "BTC/USD",
					At:     at,
					Distribution: map[logic.CategoryType]float64{
						logic.CategoryAggressiveDrive: 1,
					},
					Confidence:    0.9,
					Strength:      0.9,
					EntryBaseline: 0.8,
					ExitBaseline:  0.5,
					Status:        "measured",
				},
				{
					Source: logic.SourceResonance,
					Symbol: "BTC/USD",
					At:     at,
					Distribution: map[logic.CategoryType]float64{
						logic.CategoryLaminar: 1,
					},
					Confidence:    0.8,
					Strength:      0.8,
					EntryBaseline: 0.5,
					ExitBaseline:  0.5,
					Status:        "measured",
				},
				{
					Source: logic.SourceCVD,
					Symbol: "ETH/USD",
					At:     at,
					Distribution: map[logic.CategoryType]float64{
						logic.CategoryAggressiveDrive: 1,
					},
					Confidence:    0.7,
					Strength:      0.7,
					EntryBaseline: 0.4,
					ExitBaseline:  0.5,
					Status:        "measured",
				},
				{
					Source: logic.SourceResonance,
					Symbol: "ETH/USD",
					At:     at,
					Distribution: map[logic.CategoryType]float64{
						logic.CategoryLaminar: 1,
					},
					Confidence:    0.6,
					Strength:      0.6,
					EntryBaseline: 0.5,
					ExitBaseline:  0.5,
					Status:        "measured",
				},
			})
			highConfidence := &logic.Action{
				Type:            logic.ActionMarket,
				Side:            logic.SideBuy,
				Symbol:          "BTC/USD",
				EntryScore:      0.2,
				EntryConfidence: 0.9,
			}
			highScore := &logic.Action{
				Type:            logic.ActionMarket,
				Side:            logic.SideBuy,
				Symbol:          "ETH/USD",
				EntryScore:      0.5,
				EntryConfidence: 0.7,
			}
			selected, selectErr := decision.Choose(
				[]*logic.Action{highConfidence, highScore},
				story,
				at,
			)

			Convey("Then Decision.Choose ranks by normalized edge", func() {
				So(err, ShouldBeNil)
				So(selectErr, ShouldBeNil)
				So(selected, ShouldHaveLength, 1)
				So(selected[0], ShouldEqual, highScore)
				So(highScore.Allowed, ShouldBeTrue)
				So(highConfidence.Verdict, ShouldEqual, "blocked")
				So(highConfidence.Reason, ShouldEqual, "lower-ranked candidate")
			})
		})
	})
}

func BenchmarkDecisionChoose(benchmark *testing.B) {
	at := time.Now().UTC()
	story := market.NewStory(context.Background())
	viper.Set("trading.sizing.base_fraction", 0.05)
	viper.Set("trading.entry.min_active_origins", 2)
	viper.Set("market.story.measurement_max_age", time.Minute)
	decision, err := NewDecision()
	if err != nil {
		benchmark.Fatal(err)
	}

	if err := story.Update([]*logic.Measurement{
		{
			Source: logic.SourceCVD,
			Symbol: "BTC/USD",
			At:     at,
			Distribution: map[logic.CategoryType]float64{
				logic.CategoryAggressiveDrive: 1,
			},
			Confidence:    0.8,
			Strength:      0.8,
			EntryBaseline: 0.5,
			ExitBaseline:  0.5,
			Status:        "measured",
		},
		{
			Source: logic.SourceResonance,
			Symbol: "BTC/USD",
			At:     at,
			Distribution: map[logic.CategoryType]float64{
				logic.CategoryLaminar: 1,
			},
			Confidence:    0.7,
			Strength:      0.7,
			EntryBaseline: 0.5,
			ExitBaseline:  0.5,
			Status:        "measured",
		},
	}); err != nil {
		benchmark.Fatal(err)
	}

	benchmark.ReportAllocs()
	for benchmark.Loop() {
		action := &logic.Action{
			Type:            logic.ActionMarket,
			Side:            logic.SideBuy,
			Symbol:          "BTC/USD",
			EntryScore:      0.6,
			EntryConfidence: 0.8,
		}

		if _, err := decision.Choose([]*logic.Action{action}, story, at); err != nil {
			benchmark.Fatal(err)
		}
	}
}
