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
	volumes   chan *decimal.Decimal
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

	if filling.volumes != nil {
		filling.volumes <- quantity
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

	filling := &fillingDesk{
		price:   price,
		writes:  make(chan string, 4),
		volumes: make(chan *decimal.Decimal, 4),
	}
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
		System:   runtime.NewSystem(context.Background(), "training", nil),
		blobs:    statBlobs{},
		keys:     sorted,
		maxDepth: maxDepthOfKeys(sorted),
		paths:    &sync.Map{},
		grid:     store.NewGrid(),
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

		Convey("A precursor history exceeding maxDepth is dynamically trimmed to learned horizon", func() {
			deepTraining := matchingTraining(
				filling,
				"R01/R02/R03/enter.json",
			)
			So(deepTraining.maxDepth, ShouldEqual, 3)

			// Pre-populate path with tokens exceeding maxDepth (depth 3)
			deepTraining.paths.Store("BTC/USD", []string{"R97", "R98", "R01", "R02"})

			action, _, path, depth := deepTraining.advance("BTC/USD", "R03")
			So(action, ShouldEqual, "enter")
			So(path, ShouldEqual, "R01/R02/R03")
			So(depth, ShouldEqual, 3)
		})

		Convey("Repeated tokens collapse and do not advance the path", func() {
			action, _, path, depth := advanceAll(training, "R01", "R01", "R02", "R02", "R02")

			So(pathTokens(training, "BTC/USD"), ShouldResemble, []string{"R01", "R02"})
			So(path, ShouldEqual, "R01/R02")
			So(depth, ShouldEqual, 2)
			So(action, ShouldEqual, "")
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
		So(grid.RegionScores(prior).HasEvidence, ShouldBeFalse)

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

		pins := [12]struct {
			source string
			metric string
		}{
			0:  {"hawkes", "buy_from_buy"},
			1:  {"hawkes", "branching_ratio"},
			2:  {"hawkes", "sell_from_sell"},
			3:  {"cvd", "quantity:buy"},
			4:  {"cvd", "churn"},
			5:  {"cvd", "quantity:sell"},
			6:  {"depthflow", "removed:ask"},
			7:  {"depthflow", "spread"},
			8:  {"depthflow", "removed:bid"},
			9:  {"sentiment", "advance_fraction"},
			10: {"morphology", "wasserstein"},
			11: {"sentiment", "decline_fraction"},
		}

		var warmFrame *data.Measurement
		peers := make([]*data.Measurement, 12)

		for stepIdx := range 20 {
			frame := data.NewMeasurement(2, "BTC/USD", "spot:trade", int64(stepIdx+1), int64(stepIdx+1))
			frame.At, frame.From = tokenStart, tokenStart

			for idx, pin := range pins {
				// Region 0 rises every step, so each step lies beyond all of
				// its priors; the other regions alternate inside theirs.
				raw := float64(2 * (stepIdx % 2))
				if idx == 0 {
					raw = float64(stepIdx)
				}

				if peers[idx] == nil {
					peers[idx] = data.NewMeasurement(2, "BTC/USD", pin.source, int64(stepIdx+1), int64(stepIdx+1))
					peers[idx].At, peers[idx].From = tokenStart, tokenStart
					peers[idx].Write(data.NewMetric(pin.metric, raw, data.UnitDimensionless, data.TimescaleTick))
				} else {
					peers[idx] = peers[idx].Next(tokenStart, map[string]float64{pin.metric: raw})
				}

				frame.Peers(peers[idx])
			}

			frame.Write()
			warmFrame = frame
		}

		scores := grid.RegionScores(warmFrame)
		token := string(scores.Token())
		So(scores.HasEvidence, ShouldBeTrue)
		So(scores.Winner, ShouldEqual, uint8(0))
		So(token, ShouldEqual, "R00")

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
