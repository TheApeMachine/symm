package strategy

import (
	"testing"
	"time"

	spotbook "github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/hindsight"
	"github.com/theapemachine/symm/kraken"
)

func TestEntryCandidateReprice(t *testing.T) {
	Convey("Given a frozen fee-inclusive claim on actual displayed spot depth", t, func() {
		at := time.Unix(100, 0)
		candidate := candidateFixture("TEST/USD", at)
		registry := NewCandidateBook(func(hindsight.LearningEvent) error { return nil })
		So(registry.Publish(candidate), ShouldBeNil)
		wallet, book := virtualFixture()
		books := &agentBooks{current: book}
		fee := &kraken.TradeVolumeFee{Fee: decimal.NewFromInt64(1)}
		cost, state := candidate.Reprice(books, wallet.pair, fee, at)
		So(state, ShouldEqual, "")
		So(cost.Cmp(candidate.cost), ShouldEqual, 0)
		Convey("A multi-leg ask withdrawal and replacement cannot preserve the old economics", func() {
			book.Update(&spotbook.UpdateOptions{Direction: spotbook.Ask, ID: "ask", Price: decimal.NewFromInt64(101), Quantity: decimal.NewFromInt64(0), Silent: true})
			book.Update(&spotbook.UpdateOptions{Direction: spotbook.Ask, ID: "new", Price: decimal.NewFromInt64(103), Quantity: decimal.NewFromInt64(4), Silent: true})
			_, state = candidate.Reprice(books, wallet.pair, fee, at.Add(time.Second))
			So(state, ShouldEqual, "repricing failed")
		})
		Convey("Changed fees and expired contexts are separate pre-venue refusals", func() {
			fee.Fee = decimal.NewFromInt64(2)
			_, state = candidate.Reprice(books, wallet.pair, fee, at)
			So(state, ShouldEqual, "no longer executable")
			_, state = candidate.Reprice(books, wallet.pair, fee, at.Add(time.Minute))
			So(state, ShouldEqual, "stale")
		})
	})
}
