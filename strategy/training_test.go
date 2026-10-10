package strategy

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"slices"
	"strings"
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
	// holdSells leaves sells unfilled, so an exit stays in progress.
	holdSells bool
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

	if filling.holdSells && message.Params.Type == "sell" {
		filling.writes <- message.Params.Type
		return nil
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

/*
bookDepth is a book history holding one version, stamped with the venue time
of the last advanced token.
*/
type bookDepth struct {
	view *broker.BookView
}

func (depth *bookDepth) BookAt(_ string, at time.Time, read func(*broker.BookView)) {
	if !depth.view.At.After(at) {
		read(depth.view)
	}
}

func (depth *bookDepth) BookWindow(_ string, from, to time.Time, read func(*broker.BookView)) bool {
	if !depth.view.At.After(to) {
		read(depth.view)
	}

	return !depth.view.At.After(from)
}

func (depth *bookDepth) Latest(_ string, read func(*broker.BookView)) {
	read(depth.view)
}

/*
statBlobs serves every token path's blob with a 5% gain over a one-minute
hold.
*/
type statBlobs struct{}

func (statBlobs) GetBlob(context.Context, string) ([]byte, error) {
	return []byte(`{"gain":0.05,"hold_seconds":60}`), nil
}

/*
tokenStart is the venue time of the first advanced token; each further token
is one minute later.
*/
var tokenStart = time.Unix(1_700_000_000, 0)

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

	// Signed flow alternating +-1000 per minute for an hour before the
	// tokens: participation never binds.
	price.Flow = broker.NewFlow()

	for minute := 60; minute > 0; minute-- {
		side := []string{"buy", "sell"}[minute%2]
		price.Flow.Record("BTC/USD", tokenStart.Add(-time.Duration(minute)*time.Minute), side, 1000)
	}

	filling := &fillingDesk{writes: make(chan string, 4)}
	filling.desk = broker.NewDesk(context.Background(), filling, price)
	filling.desk.UseDepth(&bookDepth{view: &broker.BookView{
		At:       tokenStart,
		Bids:     []broker.BookLevel{{Price: 59990, Quantity: 10}},
		Asks:     []broker.BookLevel{{Price: 60000, Quantity: 10}},
		Complete: true,
	}})

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
		blobs:      statBlobs{},
		stats:      make(map[string]*pathStats),
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
		training.advance("BTC/USD", token, tokenStart)
	}
}

/*
eventually polls condition until it holds or a deadline passes: an entry's
sizing outcome replaces its decision reason off the market path.
*/
func eventually(condition func() bool) bool {
	deadline := time.Now().Add(3 * time.Second)

	for time.Now().Before(deadline) {
		if condition() {
			return true
		}

		time.Sleep(5 * time.Millisecond)
	}

	return condition()
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
			So(eventually(func() bool {
				return strings.Contains(latest(training).Reason, "submitted (sized from 1 matched gains, 1 durations")
			}), ShouldBeTrue)
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
			So(latest(training).Reason, ShouldContainSubstring, "learned_exit submitted")
		})

		Convey("A learned exit while the capacity monitor is fully exiting is recorded beside it", func() {
			filling.holdSells = true
			advanceAll(training, "R01")
			So(<-filling.writes, ShouldEqual, "buy")

			So(filling.desk.ExitBy("BTC/USD", broker.TriggerCapacityExit), ShouldBeNil)
			So(<-filling.writes, ShouldEqual, "sell")
			So(filling.desk.State("BTC/USD"), ShouldEqual, broker.EXITING)

			advanceAll(training, "R02")
			So(latest(training).Reason, ShouldContainSubstring, "learned_exit recorded beside the exit already in progress")
			So(filling.writes, ShouldHaveLength, 0)

			sells := filling.desk.PositionsWire().Rows[0].Holding.Sells
			So(sells, ShouldHaveLength, 2)
			So(sells[0].Trigger, ShouldEqual, broker.TriggerCapacityExit)
			So(sells[1].Trigger, ShouldEqual, broker.TriggerLearnedExit)
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

/*
cycleTape is n region tokens cycling R01..R12, so no token repeats its
neighbour and Train would not collapse any of them.
*/
func cycleTape(n int) []string {
	tape := make([]string, n)

	for idx := range tape {
		tape[idx] = fmt.Sprintf("R%02d", idx%12+1)
	}

	return tape
}

func TestFitFrom(t *testing.T) {
	Convey("Given a fragment of 300 three-byte tokens", t, func() {
		tape := cycleTape(300)

		for _, action := range []string{actionEnter, actionExit} {
			from := fitFrom(tape, len(action))
			key := strings.Join(append(slices.Clone(tape[from:]), action), "/")

			Convey("the kept "+action+" key fits the S3 limit and one more token would not", func() {
				So(from, ShouldBeGreaterThan, 0)
				So(len(key), ShouldBeLessThanOrEqualTo, maxKeyBytes)
				So(len(tape[from-1])+1+len(key), ShouldBeGreaterThan, maxKeyBytes)
			})
		}

		Convey("bytes and strings keep the same tokens", func() {
			raw := make([][]byte, len(tape))

			for idx, token := range tape {
				raw[idx] = []byte(token)
			}

			So(fitFrom(raw, len(actionEnter)), ShouldEqual, fitFrom(tape, len(actionEnter)))
		})

		Convey("a fragment that already fits is kept whole", func() {
			So(fitFrom(tape[:10], len(actionEnter)), ShouldEqual, 0)
			So(fitFrom([]string{}, len(actionEnter)), ShouldEqual, 0)
		})
	})

	Convey("Given an endpoint tick, the fragment reads through the tick before it", t, func() {
		low, end := fragmentTicks(26607, 26608)
		So(low, ShouldEqual, 26607)
		So(end, ShouldEqual, 26607)

		low, end = fragmentTicks(3209, 3209)
		So(end, ShouldBeLessThan, low)
	})
}

func TestTraining_advanceLongPath(t *testing.T) {
	Convey("Given a stored enter key Train cut from a 300-token precursor", t, func() {
		tape := cycleTape(300)
		kept := tape[fitFrom(tape, len(actionEnter)):]
		key := strings.Join(kept, "/") + "/" + actionEnter
		filling := newFillingDesk()
		training := matchingTraining(filling.desk, len(kept), key)

		Convey("Step replaying the whole precursor matches it at full length, never holding more than a key can", func() {
			var entered *wire.DecisionT

			for _, token := range tape {
				training.advance("BTC/USD", token, tokenStart)

				if live := training.paths["BTC/USD"]; live != nil {
					So(len(strings.Join(live.tokens, "/"))+1+len(actionExit), ShouldBeLessThanOrEqualTo, maxKeyBytes)
				}

				if decision := latest(training); decision.Action == "enter" {
					entered = decision
					break
				}
			}

			So(entered, ShouldNotBeNil)
			So(entered.Confidence, ShouldEqual, len(kept))
		})
	})
}
