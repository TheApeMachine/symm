package strategy

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/bytedance/sonic"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/system"
	"github.com/theapemachine/symm/tests/tablestest"
)

type testFillingDesk struct {
	desk    *broker.Desk
	price   *broker.Price
	writes  chan string
	volumes chan *decimal.Decimal
}

func (filling *testFillingDesk) Write(buf []byte) error {
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

	filling.desk.Apply(&kraken.Execution{Data: []kraken.ExecutionData{{
		ClientOrderID: message.Params.ClOrdId,
		Symbol:        message.Params.Pair,
		Side:          message.Params.Type,
		LastQty:       quantity,
		Cost:          decimal.NewFromFloat64(quantity.Float64() * 60000),
		FeeUsdEquiv:   decimal.NewFromFloat64(0.1),
	}}})

	if filling.writes != nil {
		filling.writes <- message.Params.Type
	}

	return nil
}

func newTestFillingDesk(t *testing.T) *testFillingDesk {
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

	price := broker.NewPrice(t.Context(), nil, nil, nil, normalizer)
	price.SetFee("BTC/USD", kraken.TradeVolumeFee{Fee: decimal.NewFromFloat64(0.26)})
	price.SetReferenceCash(decimal.NewFromFloat64(200))

	filling := &testFillingDesk{
		price:   price,
		writes:  make(chan string, 16),
		volumes: make(chan *decimal.Decimal, 16),
	}
	filling.desk = broker.NewDesk(context.Background(), filling, price, nil)
	return filling
}

func TestTraining(t *testing.T) {
	system.NewConfig()
	ctx := context.Background()

	Convey("Training", t, func() {
		filling := newTestFillingDesk(t)
		catalog := tablestest.New(t)

		Convey("NewTraining requires price, desk and catalog", func() {
			tr1 := NewTraining(ctx, nil, filling.desk, catalog)
			So(tr1.Error(), ShouldNotBeNil)

			tr2 := NewTraining(ctx, filling.price, nil, catalog)
			So(tr2.Error(), ShouldNotBeNil)

			tr3 := NewTraining(ctx, filling.price, filling.desk, nil)
			So(tr3.Error(), ShouldNotBeNil)
		})

		Convey("ReloadKeys without tokenizer stays idle", func() {
			training := NewTraining(ctx, filling.price, filling.desk, catalog)
			So(training.Error(), ShouldBeNil)
			current := training.policy.Load()
			So(current.tokenizer, ShouldBeNil)
		})

		Convey("CognitionTree exports stored keys", func() {
			training := NewTraining(ctx, filling.price, filling.desk, catalog)
			tree := training.CognitionTree()
			So(tree.Keys, ShouldNotBeNil)
		})

		Convey("Step emits an actual member count for every existing region", func() {
			training := NewTraining(ctx, filling.price, filling.desk, catalog)
			next := training.Step(trainingRegionFrame())
			So(next, ShouldNotBeNil)
			So(training.Error(), ShouldBeNil)
			So(next.Source, ShouldEqual, "training")

			for idx := range store.RegionCount {
				members := data.Pull(next.Read(fmt.Sprintf("region_members:R%02d", idx)))
				brightness := data.Pull(next.Read(fmt.Sprintf("region_brightness:R%02d", idx)))
				So(members, ShouldNotBeNil)
				So(brightness, ShouldNotBeNil)
				So(members.Metric.Raw(), ShouldEqual, 1)
				So(brightness.Metric.Raw(), ShouldEqual, 0)
			}

			So(filling.writes, ShouldHaveLength, 0)
		})

		Convey("Step rejects twelve copies of a metric from just one region", func() {
			training := NewTraining(ctx, filling.price, filling.desk, catalog)
			complete := trainingRegionFrame()
			join := data.NewMeasurement(complete.Epoch, complete.Label, "runtime:join", 1, 1)
			join.At, join.From = complete.At, complete.From

			for idx := range store.RegionCount {
				peer := data.NewMeasurement(complete.Epoch, complete.Label, "hawkes", int64(idx+1), 1)
				peer.At, peer.From = complete.At, complete.From
				unit, timescale := data.GetDimension("hawkes", "buy_from_buy")
				join.Peers(peer.Write(data.NewMetric("buy_from_buy", float64(idx+2), unit, timescale)))
			}

			join.Write()
			previous := data.NewFromPrior(complete, "training", map[string]float64{})
			training.last[complete.Label] = previous
			training.paths[complete.Label] = []string{"existing"}
			tokenizer := &Tokenizer{Depth: 1, Regions: 1, Threshold: 0}
			training.policy.Store(&policy{
				tokenizer: tokenizer,
				actions:   map[string]string{"R00": "enter"},
			})
			training.seen = *tokenizer

			Convey("No output, path advancement or order is produced", func() {
				So(training.Step(join), ShouldBeNil)
				So(training.Error(), ShouldNotBeNil)
				So(training.Error().Error(), ShouldContainSubstring, "R01")
				So(training.last[complete.Label], ShouldEqual, previous)
				So(training.paths[complete.Label], ShouldResemble, []string{"existing"})
				So(filling.writes, ShouldHaveLength, 0)

				So(training.Step(complete), ShouldBeNil)
				So(training.last[complete.Label], ShouldEqual, previous)
				So(training.paths[complete.Label], ShouldResemble, []string{"existing"})
				So(filling.writes, ShouldHaveLength, 0)
			})
		})

		Convey("Step rejects a nil frame before accessing its label", func() {
			training := NewTraining(ctx, filling.price, filling.desk, catalog)
			So(training.Step(nil), ShouldBeNil)
			So(training.Error(), ShouldNotBeNil)
			So(training.last, ShouldHaveLength, 0)
			So(training.paths, ShouldHaveLength, 0)
			So(filling.writes, ShouldHaveLength, 0)
		})
	})
}

/*
trainingRegionFrame supplies one real pinned metric for each existing region.
It is a region-coverage fixture, not a claim that each source emitted its
entire metric schema. Every metric takes its declared dimension from data.
*/
func trainingRegionFrame() *data.Measurement {
	at := time.Unix(1_700_000_000, 0).UTC()
	join := data.NewMeasurement(100, "BTC/USD", "runtime:join", 1, 1)
	join.At, join.From = at, at
	producers := []struct {
		source  string
		metrics []string
	}{
		{"hawkes", []string{"arrival_rate:buy", "arrival_rate", "arrival_rate:sell"}},
		{"cvd", []string{"aggressive_notional:buy", "gross_notional", "aggressive_notional:sell"}},
		{"depthflow", []string{"net_displayed_flow:ask", "book_imbalance", "net_displayed_flow:bid"}},
		{"sentiment", []string{"breadth", "unchanged_count", "decline_count"}},
	}

	for idx, producer := range producers {
		peer := data.NewMeasurement(join.Epoch, join.Label, producer.source, int64(idx+1), join.Tick)
		peer.At, peer.From = at, at
		metrics := make([]*data.Metric, 0, len(producer.metrics))

		for _, label := range producer.metrics {
			unit, timescale := data.GetDimension(producer.source, label)
			metrics = append(metrics, data.NewMetric(label, 1, unit, timescale))
		}

		join.Peers(peer.Write(metrics...))
	}

	return join.Write()
}
