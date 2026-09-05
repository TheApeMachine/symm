package strategy

import (
	"math/big"
	"testing"
	"time"

	"github.com/google/uuid"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/hindsight"
	"github.com/theapemachine/symm/types"
)

func candidateFixture(symbol string, at time.Time) *EntryCandidate {
	wallet, book := virtualFixture()
	quantity := big.NewRat(1, 1)
	_, gross := wallet.sweep(book, quantity, true, nil, nil)
	cost := new(big.Rat).Mul(gross, &wallet.factor)
	candidate := &EntryCandidate{Record: hindsight.CandidateRecord{ID: uuid.NewString(), Decision: 1, Symbol: symbol, Action: "enter", At: at, Horizon: time.Minute, GridVersion: 1, Authority: 1,
		Quantity: quantity.RatString(), Notional: cost.RatString(), QtyMinimum: wallet.minimum.RatString(), QtyIncrement: wallet.lot.RatString(), CostMinimum: wallet.costMinimum.RatString(), FeeRate: wallet.fee.RatString()},
		action: LearningAction{Kind: types.ActionEnter}, quantity: quantity, cost: cost, bid: book.Bids.High.Price.Rat()}
	wallet.sweep(book, quantity, true, &candidate.ladder, nil)
	candidate.Intent = ExecutionIntent{Candidate: candidate, Symbol: symbol, Quantity: quantity, MaximumCost: cost, Kind: types.ActionEnter, CorrelationID: candidate.Record.ID}
	return candidate
}

func TestCandidateBookPublish(t *testing.T) {
	Convey("Given prospectively priced immutable candidates", t, func() {
		events := []hindsight.LearningEvent{}
		book := NewCandidateBook(func(event hindsight.LearningEvent) error { events = append(events, event); return nil })
		at := time.Unix(100, 0)
		first := candidateFixture("TEST/USD", at)
		So(book.Publish(first), ShouldBeNil)
		So(first.Current(at), ShouldBeTrue)
		Convey("Replacing the originating context revokes even a queued candidate", func() {
			first.selected = true
			second := candidateFixture("TEST/USD", at.Add(time.Second))
			second.Record.GridVersion++
			So(book.Publish(second), ShouldBeNil)
			So(first.Current(at.Add(time.Second)), ShouldBeFalse)
			So(events[0].Candidate.GridVersion, ShouldEqual, 1)
			So(events[1].CandidateResult.State, ShouldEqual, "stale")
			So(events[1].CandidateResult.ID, ShouldEqual, events[0].Candidate.ID)
			So(events[0].Candidate.At, ShouldEqual, at)
		})

		Convey("Changed authority or funding cannot leave the old claim waiting for later cash", func() {
			So(book.Outcome(first, "insufficient capital", at, "", "account cash changed"), ShouldBeNil)
			So(first.Current(at), ShouldBeFalse)
			candidates, err := book.Viable(at)
			So(err, ShouldBeNil)
			So(candidates, ShouldBeEmpty)
		})
		Convey("The measured horizon retires a claim without fabricating a local reward", func() {
			candidates, err := book.Viable(at.Add(time.Minute))
			So(err, ShouldBeNil)
			So(candidates, ShouldBeEmpty)
			So(events[1].Kind, ShouldEqual, "candidate_status")
			So(events[1].Target, ShouldEqual, 0)
		})
	})
}

func BenchmarkCandidateBookViable(b *testing.B) {
	book := NewCandidateBook(func(hindsight.LearningEvent) error { return nil })
	at := time.Unix(100, 0)
	for _, symbol := range []string{"A/USD", "B/USD", "C/USD"} {
		if err := book.Publish(candidateFixture(symbol, at)); err != nil {
			b.Fatal(err)
		}
	}
	b.ReportAllocs()
	for b.Loop() {
		if _, err := book.Viable(at); err != nil {
			b.Fatal(err)
		}
	}
}
