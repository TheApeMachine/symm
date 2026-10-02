package strategy

import (
	"context"
	"testing"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique/cognition"
)

func TestEvaluator(t *testing.T) {
	Convey("Evaluator pricing and model reinforcement", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		price := broker.NewPrice(ctx, nil, nil, nil, nil)
		price.SetFee("XXBTZUSD", kraken.TradeVolumeFee{
			Fee: decimal.NewFromFloat64(0.26),
		})
		price.SetReferenceCash(decimal.NewFromFloat64(10000))

		engine := cognition.NewEngine(cognition.Config{})
		evaluator := NewEvaluator(price, engine)

		Convey("EvaluatePnL calculates net economic return after fees", func() {
			entryAsk := decimal.NewFromFloat64(60000.0)
			exitBid := decimal.NewFromFloat64(63000.0) // 5% gross move

			pnl, err := evaluator.EvaluatePnL("XXBTZUSD", entryAsk, exitBid)
			So(err, ShouldBeNil)
			// Net return after taker fees should be around 4.4% - 4.5%
			So(pnl, ShouldBeGreaterThan, 0.04)
			So(pnl, ShouldBeLessThan, 0.05)
		})

		Convey("Train reinforces cognition trie for context", func() {
			contextSeq := []byte{1, 2, 0, 3, 4, 0}
			evaluator.Train(contextSeq, cognition.ActionEnter, 0.045)

			res, err := engine.Evaluate([]byte{3, 4})
			So(err, ShouldBeNil)
			So(res.Evaluation.WinnerClass, ShouldEqual, "enter")
		})

		Convey("ObserveWait records intermediate prefix states", func() {
			prefix := []byte{1, 2}
			evaluator.ObserveWait(prefix)

			res, err := engine.Evaluate(prefix)
			So(err, ShouldBeNil)
			So(res.Evaluation.WinnerClass, ShouldEqual, "wait")
		})
	})
}
