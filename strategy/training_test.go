package strategy

import (
	"context"
	"errors"
	"iter"
	"testing"
	"time"

	"github.com/bytedance/sonic"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/kraken"
	wire "github.com/theapemachine/symm/telemetry/generated/telemetry"
)

/*
fillingDesk is a Desk whose transport fills every market order immediately,
echoing the client order ID back through Desk.Apply like Paper does. writes
receives one value per submitted order.
*/
type fillingDesk struct {
	desk   *broker.Desk
	writes chan string
}

func (filling *fillingDesk) Write(buf []byte) error {
	message := kraken.AddOrderMessage{}

	if err := sonic.Unmarshal(buf, &message); err != nil {
		return err
	}

	quantity, err := decimal.NewFromString(message.Params.Volume)

	if err != nil {
		return err
	}

	filling.desk.Apply(&kraken.Execution{Data: []kraken.ExecutionData{{
		ClientOrderID: message.Params.ClOrdId,
		Symbol:        message.Params.Pair,
		Side:          message.Params.Type,
		LastQty:       quantity,
		Cost:          decimal.NewFromFloat64(quantity.Float64() * 60000),
		FeeUsdEquiv:   decimal.NewFromFloat64(0.1),
	}}})

	filling.writes <- message.Params.Type
	return nil
}

func newFillingDesk() *fillingDesk {
	normalizer := spot.NewNormalizer()
	normalizer.Update(&spot.AssetsManagerUpdate{
		NewAssets: map[string]spot.AssetInfo{
			"BTC": {AltName: "XBT"},
			"USD": {AltName: "USD"},
		},
		NewPairs: map[string]spot.AssetPair{
			"BTC/USD": {WSName: "BTC/USD", Base: "BTC", Quote: "USD", LotDecimals: 8, LotMultiplier: 1},
		},
	})

	price := broker.NewPrice(context.Background(), nil, nil, nil, normalizer)
	price.SetFee("BTC/USD", kraken.TradeVolumeFee{Fee: decimal.NewFromFloat64(0.26)})
	price.SetReferenceCash(decimal.NewFromFloat64(200))
	price.SetQuote("BTC/USD", decimal.NewFromFloat64(59990), decimal.NewFromFloat64(60000))

	filling := &fillingDesk{writes: make(chan string, 4)}
	filling.desk = broker.NewDesk(context.Background(), filling, price)
	return filling
}

/*
listing is a fake S3 prefix-less listing of the given keys, in arbitrary order.
*/
func listing(keys ...string) iter.Seq2[string, error] {
	return func(yield func(string, error) bool) {
		for _, key := range keys {
			if !yield(key, nil) {
				return
			}
		}
	}
}

/*
matchingTraining builds a Training over a fake key listing, matching through
the given Desk with the given minimum path confidence.
*/
func matchingTraining(desk *broker.Desk, confidence int, keys ...string) *Training {
	sorted, err := sortedKeys(listing(keys...))

	So(err, ShouldBeNil)

	return &Training{
		desk:       desk,
		keys:       sorted,
		confidence: confidence,
		paths:      make(map[string]*livePath),
		decisions:  make(map[string]*wire.DecisionT),
	}
}

func latest(training *Training) *wire.DecisionT {
	frame := training.DecisionsWire()

	So(frame.Decisions, ShouldHaveLength, 1)
	return frame.Decisions[0]
}

func advanceAll(training *Training, tokens ...string) {
	for _, token := range tokens {
		training.advance("BTC/USD", token, time.Unix(1_700_000_000, 0))
	}
}

func TestTraining_advance(t *testing.T) {
	Convey("Given stored paths that share a prefix and then diverge", t, func() {
		filling := newFillingDesk()
		training := matchingTraining(
			filling.desk, 3,
			"R01/R02/R03/R05/exit.json",
			"R01/R02/R03/R04/enter.json",
		)

		Convey("The shared prefix is ambiguous and takes no action", func() {
			advanceAll(training, "R01", "R02", "R03")

			decision := latest(training)
			So(decision.Action, ShouldEqual, "accumulate")
			So(decision.Cause, ShouldEqual, "R01/R02/R03")
			So(decision.Confidence, ShouldEqual, 3)
			So(decision.Alternatives, ShouldHaveLength, 2)
			So(filling.desk.State("BTC/USD"), ShouldEqual, broker.FLAT)
			So(training.paths["BTC/USD"].tokens, ShouldResemble, []string{"R01", "R02", "R03"})
		})

		Convey("The divergent token makes the match unique and enters through the Desk", func() {
			advanceAll(training, "R01", "R02", "R03", "R04")

			So(<-filling.writes, ShouldEqual, "buy")
			So(filling.desk.State("BTC/USD"), ShouldEqual, broker.HOLDING)

			decision := latest(training)
			So(decision.Action, ShouldEqual, "enter")
			So(decision.Cause, ShouldEqual, "R01/R02/R03/R04")
			So(decision.Confidence, ShouldEqual, 4)
			So(decision.Alternatives, ShouldResemble, []*wire.NamedNumberT{{Name: "enter", Value: 1}})
			So(decision.Reason, ShouldContainSubstring, "submitted")
			So(training.paths["BTC/USD"].tokens, ShouldBeEmpty)
		})

		Convey("Repeated tokens collapse and do not advance the path", func() {
			advanceAll(training, "R01", "R01", "R02", "R02", "R02")

			So(training.paths["BTC/USD"].tokens, ShouldResemble, []string{"R01", "R02"})
			So(training.DecisionsVersion(), ShouldEqual, 2)
		})

		Convey("A token no stored path continues resets the path", func() {
			advanceAll(training, "R01", "R02", "R09")

			decision := latest(training)
			So(decision.Action, ShouldEqual, "reset")
			So(decision.Cause, ShouldEqual, "R09")
			So(training.paths["BTC/USD"].tokens, ShouldBeEmpty)
			So(filling.desk.State("BTC/USD"), ShouldEqual, broker.FLAT)
		})

		Convey("A reset restarts from the current token when a stored path starts with it", func() {
			advanceAll(training, "R01", "R02", "R01")

			decision := latest(training)
			So(decision.Action, ShouldEqual, "accumulate")
			So(decision.Cause, ShouldEqual, "R01")
			So(training.paths["BTC/USD"].tokens, ShouldResemble, []string{"R01"})
		})
	})

	Convey("Given stored paths that agree on the action from the first token", t, func() {
		filling := newFillingDesk()
		training := matchingTraining(
			filling.desk, 3,
			"R06/R07/R08/enter.json",
			"R06/R07/R08/R10/enter.json",
		)

		Convey("A unanimous match shorter than the minimum confidence takes no action", func() {
			advanceAll(training, "R06", "R07")

			decision := latest(training)
			So(decision.Action, ShouldEqual, "accumulate")
			So(decision.Confidence, ShouldEqual, 2)
			So(decision.Reason, ShouldContainSubstring, "below minimum confidence 3")
			So(filling.desk.State("BTC/USD"), ShouldEqual, broker.FLAT)
		})

		Convey("A unanimous match at the minimum confidence takes the action", func() {
			advanceAll(training, "R06", "R07", "R08")

			So(<-filling.writes, ShouldEqual, "buy")
			So(filling.desk.State("BTC/USD"), ShouldEqual, broker.HOLDING)

			decision := latest(training)
			So(decision.Action, ShouldEqual, "enter")
			So(decision.Confidence, ShouldEqual, 3)
			So(decision.Alternatives, ShouldResemble, []*wire.NamedNumberT{{Name: "enter", Value: 2}})
		})
	})

	Convey("Given unique enter and exit paths", t, func() {
		filling := newFillingDesk()
		training := matchingTraining(
			filling.desk, 1,
			"R01/enter.json",
			"R02/exit.json",
		)

		Convey("An exit match on a flat symbol is ignored by the Desk gate", func() {
			advanceAll(training, "R02")

			decision := latest(training)
			So(decision.Action, ShouldEqual, "exit")
			So(decision.Reason, ShouldContainSubstring, "ignored: no filled position to exit")
			So(filling.desk.State("BTC/USD"), ShouldEqual, broker.FLAT)
		})

		Convey("An enter match while holding is ignored, and an exit match exits", func() {
			advanceAll(training, "R01")
			So(<-filling.writes, ShouldEqual, "buy")
			So(filling.desk.State("BTC/USD"), ShouldEqual, broker.HOLDING)

			advanceAll(training, "R03", "R01")
			So(latest(training).Reason, ShouldContainSubstring, "ignored: position is not flat")
			So(filling.writes, ShouldHaveLength, 0)

			advanceAll(training, "R02")
			So(<-filling.writes, ShouldEqual, "sell")
			So(filling.desk.State("BTC/USD"), ShouldEqual, broker.FLAT)
			So(latest(training).Action, ShouldEqual, "exit")
		})
	})
}

func TestTraining_match(t *testing.T) {
	Convey("Given keys whose tokens share leading characters", t, func() {
		training := matchingTraining(
			nil, 1,
			"R01/R02/enter.json",
			"R010/exit.json",
			"R01/exit.json",
			"R02/enter.json",
		)

		Convey("It returns exactly the keys an S3 prefix listing of the path returns", func() {
			So(training.match([]string{"R01"}), ShouldResemble, []string{"R01/R02/enter.json", "R01/exit.json"})
			So(training.match([]string{"R01", "R02"}), ShouldResemble, []string{"R01/R02/enter.json"})
			So(training.match([]string{"R03"}), ShouldBeEmpty)
		})
	})
}

func TestSortedKeys(t *testing.T) {
	Convey("Given a listing that fails part way", t, func() {
		failing := func(yield func(string, error) bool) {
			if !yield("R01/enter.json", nil) {
				return
			}

			yield("", errors.New("listing failed"))
		}

		Convey("The failure is returned, never an empty or partial key space", func() {
			keys, err := sortedKeys(failing)

			So(err, ShouldNotBeNil)
			So(keys, ShouldBeNil)
		})
	})
}
