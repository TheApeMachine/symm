package strategy

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/learning"
	markettest "github.com/theapemachine/symm/tests/market"
	"github.com/theapemachine/symm/types"
)

func TestAccountRewardMeasure(t *testing.T) {
	Convey("Given a continuing account with known net liquidation marks", t, func() {
		ledger := &AccountReward{}
		at := time.Unix(1, 0)
		initial := EquityMark{HasFunding: true, At: at, Version: 1, Equity: 100}
		first, err := ledger.Measure(initial)
		So(err, ShouldBeNil)
		So(first.HasRate, ShouldBeFalse)
		So(first.Transitions, ShouldEqual, 0)

		Convey("profit, open losses, and recovery remain economic changes", func() {
			var outcome learning.RewardOutcome

			for index, equity := range []float64{110, 80, 100} {
				outcome, err = ledger.Measure(EquityMark{HasFunding: true,
					At:      at.Add(time.Duration(index+1) * time.Second),
					Version: uint64(index + 2), Equity: equity,
				})
				So(err, ShouldBeNil)
			}
			So(outcome.TotalReward, ShouldEqual, 0)
			So(outcome.TotalElapsed, ShouldEqual, 3*time.Second)
		})

		Convey("waiting falls behind only the rate already observed", func() {
			gain, err := ledger.Measure(EquityMark{HasFunding: true, At: at.Add(time.Second), Version: 2, Equity: 110})
			So(err, ShouldBeNil)
			So(gain.HasPriorRate, ShouldBeFalse)
			So(gain.Rate, ShouldEqual, 10)
			wait, err := ledger.Measure(EquityMark{HasFunding: true, At: at.Add(3 * time.Second), Version: 3, Equity: 110})
			So(err, ShouldBeNil)
			So(wait.Reward, ShouldEqual, 0)
			So(wait.PriorRate, ShouldEqual, 10)
			So(wait.Differential, ShouldEqual, -20)
			So(wait.Rate, ShouldAlmostEqual, 10.0/3)
		})

		Convey("flat from the outset is zero reward and not forced churn", func() {
			flat, err := ledger.Measure(EquityMark{HasFunding: true, At: at.Add(time.Hour), Version: 2, Equity: 100})
			So(err, ShouldBeNil)
			So(flat.Reward, ShouldEqual, 0)
			So(flat.HasRate, ShouldBeTrue)
			So(flat.Rate, ShouldEqual, 0)
		})

		Convey("deposits and withdrawals cannot become trading profit", func() {
			deposit := EquityMark{HasFunding: true, At: at.Add(time.Second), Version: 2, Equity: 150, NetFunding: 50}
			funded, err := ledger.Measure(deposit)
			So(err, ShouldBeNil)
			So(funded.Reward, ShouldEqual, 0)
			repeated, err := ledger.Measure(deposit)
			So(err, ShouldBeNil)
			So(repeated, ShouldResemble, funded)
			withdrawn, err := ledger.Measure(EquityMark{HasFunding: true,
				At: at.Add(2 * time.Second), Version: 3, Equity: 115, NetFunding: 10,
			})
			So(err, ShouldBeNil)
			So(withdrawn.Reward, ShouldEqual, 5)
			So(withdrawn.TotalReward, ShouldEqual, 5)
		})

		Convey("missed intermediate funding marks still cannot manufacture profit", func() {
			// A deposit of 50 and withdrawal of 40 occurred since version 1.
			mark := EquityMark{At: at.Add(time.Second), Version: 4, Equity: 115,
				NetFunding: 10, HasFunding: true}
			outcome, err := ledger.Measure(mark)
			So(err, ShouldBeNil)
			So(outcome.Reward, ShouldEqual, 5)
		})

		Convey("insolvency and a further loss remain losses despite relative feedback", func() {
			ruined, err := ledger.Measure(EquityMark{
				At: at.Add(time.Second), Version: 2, Equity: 0, HasFunding: true,
			})
			So(err, ShouldBeNil)
			So(ruined.Reward, ShouldEqual, -100)
			loss, err := ledger.Measure(EquityMark{
				At: at.Add(2 * time.Second), Version: 3, Equity: -5, HasFunding: true,
			})
			So(err, ShouldBeNil)
			So(loss.Reward, ShouldEqual, -5)
			So(loss.TotalReward, ShouldEqual, -105)
			So(loss.Differential, ShouldEqual, 95)
		})

		Convey("same-instant fills affect profit without inventing a rate", func() {
			fee, err := ledger.Measure(EquityMark{HasFunding: true, At: at, Version: 2, Equity: 99})
			So(err, ShouldBeNil)
			So(fee.Reward, ShouldEqual, -1)
			So(fee.HasRate, ShouldBeFalse)
			settled, err := ledger.Measure(EquityMark{HasFunding: true, At: at.Add(time.Second), Version: 3, Equity: 99})
			So(err, ShouldBeNil)
			So(settled.Rate, ShouldEqual, -1)
		})

		Convey("invalid or rewritten marks cannot mutate prior evidence", func() {
			before := *ledger

			for _, invalid := range []EquityMark{
				{}, {At: at, HasFunding: true},
				{At: at, Version: 1, Equity: 101, HasFunding: true},
				// This rewrite projects to the same reward but changes the account evidence.
				{At: at, Version: 1, Equity: 150, NetFunding: 50, HasFunding: true},
				{At: at.Add(-time.Second), Version: 2, Equity: 100, HasFunding: true},
				{At: at.Add(time.Second), Version: 2, Equity: 100},
			} {
				_, err := ledger.Measure(invalid)
				So(err, ShouldNotBeNil)
				So(ledger.last, ShouldResemble, initial)
				So(*ledger, ShouldResemble, before)
			}
		})

		Convey("serialization of an identical timestamp does not create a new valuation", func() {
			redelivered := initial
			redelivered.At = initial.At.In(time.FixedZone("other", 3600))
			outcome, err := ledger.Measure(redelivered)
			So(err, ShouldBeNil)
			So(outcome, ShouldResemble, first)
		})
	})

	Convey("Given an account without a valid initial valuation", t, func() {
		ledger := &AccountReward{}
		at := time.Unix(1, 0)

		for _, mark := range []EquityMark{
			{At: at, Version: 1, Equity: 100},
			{Version: 1, Equity: 100, HasFunding: true},
			{At: at, Equity: 100, HasFunding: true},
		} {
			_, err := ledger.Measure(mark)
			So(err, ShouldNotBeNil)
			So(*ledger, ShouldResemble, AccountReward{})
		}

		first, err := ledger.Measure(EquityMark{
			At: at, Version: 1, Equity: 100, NetFunding: 50, HasFunding: true,
		})
		So(err, ShouldBeNil)
		So(first.TotalReward, ShouldEqual, 0)
		next, err := ledger.Measure(EquityMark{
			At: at.Add(time.Second), Version: 2, Equity: 115, NetFunding: 60, HasFunding: true,
		})
		So(err, ShouldBeNil)
		So(next.Reward, ShouldEqual, 5)
	})

	Convey("Given separate virtual wallets initialized at the same boundary", t, func() {
		at := time.Unix(1, 0)
		firstLane, secondLane := &AccountReward{}, &AccountReward{}
		initial := EquityMark{
			At: at, Version: 10, Equity: 150, NetFunding: 20, HasFunding: true,
		}

		for _, lane := range []*AccountReward{firstLane, secondLane} {
			outcome, err := lane.Measure(initial)
			So(err, ShouldBeNil)
			So(outcome.TotalReward, ShouldEqual, 0)
		}

		untouched := *secondLane
		gain, err := firstLane.Measure(EquityMark{
			At: at.Add(time.Second), Version: 11, Equity: 165, NetFunding: 20, HasFunding: true,
		})
		So(err, ShouldBeNil)
		So(gain.Reward, ShouldEqual, 15)
		So(*secondLane, ShouldResemble, untouched)
		loss, err := secondLane.Measure(EquityMark{
			At: at.Add(time.Second), Version: 11, Equity: 140, NetFunding: 20, HasFunding: true,
		})
		So(err, ShouldBeNil)
		So(loss.Reward, ShouldEqual, -10)
		So(firstLane.last.Equity, ShouldEqual, 165)
	})

	Convey("Given different reporting frequencies over the same equity path", t, func() {
		at := time.Unix(1, 0)
		coarse, fine := &AccountReward{}, &AccountReward{}

		for _, ledger := range []*AccountReward{coarse, fine} {
			_, err := ledger.Measure(EquityMark{HasFunding: true, At: at, Version: 1, Equity: 100})
			So(err, ShouldBeNil)
		}

		var fineOutcome learning.RewardOutcome
		var err error

		for index, equity := range []float64{110, 90, 130} {
			fineOutcome, err = fine.Measure(EquityMark{HasFunding: true,
				At: at.Add(time.Duration(index+1) * time.Second), Version: uint64(index + 2), Equity: equity,
			})
			So(err, ShouldBeNil)
		}
		coarseOutcome, err := coarse.Measure(EquityMark{HasFunding: true, At: at.Add(3 * time.Second), Version: 2, Equity: 130})
		So(err, ShouldBeNil)
		So(coarseOutcome.TotalReward, ShouldEqual, fineOutcome.TotalReward)
		So(coarseOutcome.Rate, ShouldEqual, fineOutcome.Rate)
	})

	Convey("Given a held unit across a multi-leg Level3 tape", t, func() {
		// The fixture has one unit of inventory, zero external funding, and
		// zero fees. This tests marks, not a claim about virtual fill realism.
		tape := markettest.NewLevel3Tape("TEST/USD", time.Unix(1, 0))
		ledger := &AccountReward{}
		model := learning.NewModel[string, types.Action]()
		// A supplied ordered context and equal producer quality isolate the
		// account-to-prior boundary from discovery and action selection.
		regions := []uint64{3, 1, 2}
		authority := 9.0 / 16
		var pending uint64
		var lastBid float64
		var last learning.RewardOutcome

		for index, message := range tape.Messages {
			bid := tape.TrueBid[index]

			if bid == 0 {
				// The whole bid side is withdrawn: liquidation is unobservable.
				continue
			}

			outcome, err := ledger.Measure(EquityMark{HasFunding: true,
				At: message.Timestamp, Version: uint64(index + 1), Equity: bid,
			})
			So(err, ShouldBeNil)

			if pending != 0 {
				So(outcome.Reward, ShouldAlmostEqual, bid-lastBid)
				reading, err := model.Resolve(pending, outcome.Reward)
				So(err, ShouldBeNil)
				So(reading.Samples, ShouldEqual, outcome.Transitions)
				So(reading.Mean, ShouldAlmostEqual, outcome.TotalReward/float64(outcome.Transitions))
			}

			if index < len(tape.Messages)-1 {
				pending, err = model.Issue(tape.Symbol, regions, types.ActionHold, authority)
				So(err, ShouldBeNil)
			}

			lastBid, last = bid, outcome
		}
		So(last.TotalReward, ShouldAlmostEqual, tape.TrueBid[len(tape.TrueBid)-1]-tape.TrueBid[0])
		So(last.TotalReward, ShouldBeLessThan, 0)
		reading := model.Recall(tape.Symbol, regions, types.ActionHold)
		So(reading.Mean, ShouldBeLessThan, 0)
		So(reading.VarianceDefined, ShouldBeTrue)
		So(reading.Support, ShouldAlmostEqual, last.Transitions)
		So(model.Recall(tape.Symbol, regions, types.ActionExit).Defined, ShouldBeFalse)
	})
}

func BenchmarkAccountRewardMeasure(b *testing.B) {
	ledger := &AccountReward{}
	at := time.Unix(1, 0)
	version := uint64(0)
	// Repeated gains, funding changes, drawdown and recovery exercise the projection.
	marks := [...]EquityMark{
		{Equity: 100, NetFunding: 0},
		{Equity: 110, NetFunding: 0},
		{Equity: 160, NetFunding: 50},
		{Equity: 130, NetFunding: 50},
		{Equity: 100, NetFunding: 0},
	}
	b.ReportAllocs()

	for b.Loop() {
		version++
		at = at.Add(time.Second)
		mark := marks[(version-1)%uint64(len(marks))]
		mark.At, mark.Version, mark.HasFunding = at, version, true

		if _, err := ledger.Measure(mark); err != nil {
			b.Fatal(err)
		}
	}
}
