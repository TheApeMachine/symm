package strategy

import (
	"context"
	"errors"
	"iter"
	"sync"
	"testing"
	"time"

	"github.com/bytedance/sonic"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/nomagique/store"
)

/*
fillingDesk is a Desk whose transport fills every market order immediately,
echoing the client order ID back through Desk.Apply like Paper does. writes
receives one value per submitted order.
*/
type fillingDesk struct {
	desk      *broker.Desk
	price     *broker.Price
	writes    chan string
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

	filling := &fillingDesk{price: price, writes: make(chan string, 4)}
	filling.desk = broker.NewDesk(context.Background(), filling, price, nil)

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
the given Desk.
*/
func matchingTraining(filling *fillingDesk, keys ...string) *Training {
	sorted, err := sortedKeys(listing(keys...))

	So(err, ShouldBeNil)

	training := &Training{
		System: runtime.NewSystem(context.Background(), "training", nil),
		blobs:  statBlobs{},
		keys:   sorted,
		paths:  &sync.Map{},
		grid:   store.NewGrid(),
	}

	if filling != nil {
		training.desk = filling.desk
		training.price = filling.price
	}

	return training
}

func pathTokens(training *Training, symbol string) []string {
	if val, ok := training.paths.Load(symbol); ok && val != nil {
		return val.([]string)
	}

	return nil
}

func advanceAll(training *Training, tokens ...string) (action, reason, path string, depth int) {
	for _, token := range tokens {
		action, reason, path, depth = training.advance("BTC/USD", token)
	}

	return action, reason, path, depth
}

func TestTraining_advance(t *testing.T) {
	Convey("Given stored paths that share a prefix and then diverge", t, func() {
		filling := newFillingDesk()
		training := matchingTraining(
			filling,
			"R01/R02/R03/R05/exit.json",
			"R01/R02/R03/R04/enter.json",
		)

		Convey("The shared prefix is ambiguous and takes no action", func() {
			action, _, path, depth := advanceAll(training, "R01", "R02", "R03")

			So(action, ShouldEqual, "accumulate")
			So(path, ShouldEqual, "R01/R02/R03")
			So(depth, ShouldEqual, 3)
			So(filling.desk.Has("BTC/USD"), ShouldBeFalse)
			So(pathTokens(training, "BTC/USD"), ShouldResemble, []string{"R01", "R02", "R03"})
		})

		Convey("The divergent token makes the match unique and enters through the Desk", func() {
			action, _, path, depth := advanceAll(training, "R01", "R02", "R03", "R04")

			So(<-filling.writes, ShouldEqual, "buy")
			So(filling.desk.Has("BTC/USD"), ShouldBeTrue)

			So(action, ShouldEqual, "enter")
			So(path, ShouldEqual, "R01/R02/R03/R04")
			So(depth, ShouldEqual, 4)
			So(pathTokens(training, "BTC/USD"), ShouldBeEmpty)
		})

		Convey("Repeated tokens collapse and do not advance the path", func() {
			action, _, path, depth := advanceAll(training, "R01", "R01", "R02", "R02", "R02")

			So(pathTokens(training, "BTC/USD"), ShouldResemble, []string{"R01", "R02"})
			So(path, ShouldEqual, "R01/R02")
			So(depth, ShouldEqual, 2)
			So(action, ShouldEqual, "")
		})

		Convey("A token no stored path continues resets the path", func() {
			action, _, path, depth := advanceAll(training, "R01", "R02", "R09")

			So(action, ShouldEqual, "")
			So(path, ShouldEqual, "")
			So(depth, ShouldEqual, 0)
			So(pathTokens(training, "BTC/USD"), ShouldBeEmpty)
			So(filling.desk.Has("BTC/USD"), ShouldBeFalse)
		})
	})

	Convey("Given stored paths with enter and noop precursors", t, func() {
		filling := newFillingDesk()
		training := matchingTraining(
			filling,
			"R01/R02/R03/R04/enter.json",
			"R01/R02/R03/R05/noop.json",
		)

		Convey("The shared prefix between enter and noop accumulates without entering", func() {
			action, _, _, _ := advanceAll(training, "R01", "R02", "R03")

			So(action, ShouldEqual, "accumulate")
			So(filling.desk.Has("BTC/USD"), ShouldBeFalse)
		})

		Convey("A path matching ONLY noop.json does not enter", func() {
			action, _, _, _ := advanceAll(training, "R01", "R02", "R03", "R05")

			So(action, ShouldEqual, "accumulate")
			So(filling.desk.Has("BTC/USD"), ShouldBeFalse)
			So(filling.writes, ShouldHaveLength, 0)
		})

		Convey("A path matching ONLY enter.json signals entry and enters", func() {
			action, _, _, _ := advanceAll(training, "R01", "R02", "R03", "R04")

			So(<-filling.writes, ShouldEqual, "buy")
			So(filling.desk.Has("BTC/USD"), ShouldBeTrue)

			So(action, ShouldEqual, "enter")
		})
	})

	Convey("Given stored paths that agree on the action from the first token", t, func() {
		filling := newFillingDesk()
		training := matchingTraining(
			filling,
			"R06/R07/R08/enter.json",
			"R06/R07/R08/R10/enter.json",
		)

		Convey("A unanimous match takes the action immediately", func() {
			action, _, _, depth := advanceAll(training, "R06")

			So(<-filling.writes, ShouldEqual, "buy")
			So(filling.desk.Has("BTC/USD"), ShouldBeTrue)

			So(action, ShouldEqual, "enter")
			So(depth, ShouldEqual, 1)
		})
	})

	Convey("Given unique enter and exit paths", t, func() {
		filling := newFillingDesk()
		training := matchingTraining(
			filling,
			"R01/enter.json",
			"R02/exit.json",
		)

		Convey("An exit match on a flat symbol is ignored by the Desk gate", func() {
			action, reason, _, _ := advanceAll(training, "R02")

			So(action, ShouldEqual, "exit")
			So(reason, ShouldContainSubstring, "ignored: no filled position to exit")
			So(filling.desk.Has("BTC/USD"), ShouldBeFalse)
		})

		Convey("An enter match while holding is ignored, and an exit match exits", func() {
			advanceAll(training, "R01")
			So(<-filling.writes, ShouldEqual, "buy")
			So(filling.desk.Has("BTC/USD"), ShouldBeTrue)

			_, reason, _, _ := advanceAll(training, "R03", "R01")
			So(reason, ShouldContainSubstring, "ignored: position already open")
			So(filling.writes, ShouldHaveLength, 0)

			action, reason, _, _ := advanceAll(training, "R02")
			So(<-filling.writes, ShouldEqual, "sell")
			So(filling.desk.Has("BTC/USD"), ShouldBeFalse)
			So(action, ShouldEqual, "exit")
			So(reason, ShouldContainSubstring, "learned_exit submitted")
		})
	})
}

func TestTraining_match(t *testing.T) {
	Convey("Given keys whose tokens share leading characters", t, func() {
		training := matchingTraining(
			nil,
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

func TestFragmentTicks(t *testing.T) {
	Convey("Given an endpoint tick, the fragment reads through the tick before it", t, func() {
		low, end := fragmentTicks(26607, 26608)
		So(low, ShouldEqual, 26607)
		So(end, ShouldEqual, 26607)

		low, end = fragmentTicks(3209, 3209)
		So(end, ShouldBeLessThan, low)
	})
}

func TestTraining_Step(t *testing.T) {
	Convey("Given training with a cold measurement (no evidence / warming up)", t, func() {
		filling := newFillingDesk()
		prior := data.NewMeasurement(9999, "COLD/USD", "unpinned", 1, 1)
		prior.At = tokenStart
		prior.From = tokenStart
		prior = prior.Write(data.NewMetric("raw_metric", 100.0, data.UnitPrice, data.TimescaleInstantaneous))

		grid := store.NewGrid()
		So(grid.RegionScores(prior).Winner, ShouldEqual, store.NoEvidence)

		training := matchingTraining(
			filling,
			"R01/enter.json",
		)
		training.grid = grid

		result := training.Step(prior)

		So(result, ShouldNotBeNil)
		So(result.Source, ShouldEqual, "training")
		So(result.Label, ShouldEqual, "COLD/USD")
		So(result.Meta("token_path"), ShouldEqual, "")
		So(result.Meta("token_depth"), ShouldEqual, "")
	})

	Convey("Given training with a warmed measurement possessing evidenced confluence", t, func() {
		filling := newFillingDesk()
		grid := store.NewGrid()

		var warmFrame *data.Measurement
		for stepIdx := range 10 {
			raw := float64(2 * (stepIdx % 2))
			frame := data.NewMeasurement(2, "BTC/USD", "spot:trade", 1, 1)
			frame.At, frame.From = tokenStart, tokenStart

			peer := data.NewMeasurement(2, "BTC/USD", "hawkes", 1, 1)
			peer.At, peer.From = tokenStart, tokenStart
			peer.Write(data.NewMetric("buy_from_buy", raw, data.UnitDimensionless, data.TimescaleTick))

			frame.Peers(peer)
			frame.Write()
			warmFrame = frame
		}

		scores := grid.RegionScores(warmFrame)
		token := string(scores.Token())
		So(scores.Winner, ShouldEqual, uint8(1))
		So(token, ShouldEqual, "R01")

		training := matchingTraining(
			filling,
			token+"/enter.json",
		)
		training.grid = grid

		result := training.Step(warmFrame)

		So(result, ShouldNotBeNil)
		So(result.Source, ShouldEqual, "training")
		So(result.Label, ShouldEqual, "BTC/USD")
		So(result.Meta("token_path"), ShouldEqual, token)
		So(result.Meta("token_depth"), ShouldEqual, "1")
	})
}
