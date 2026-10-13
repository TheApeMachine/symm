package strategy

import (
	"context"
	"testing"
	"time"

	"github.com/bytedance/sonic"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique/data"
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

		Convey("Step emits region metrics and meta", func() {
			training := NewTraining(ctx, filling.price, filling.desk, catalog)
			now := time.Now().UTC()
			join := data.NewMeasurement(100, "BTC/USD", "runtime:join", now, now)

			// Add peers covering regions
			peers := make([]*data.Measurement, 0, 12)
			for idx := range 12 {
				peer := data.NewMeasurement(100, "BTC/USD", "hawkes", now, now)
				peer = peer.Write(data.NewMetric("buy_from_buy", float64(idx+2), data.UnitCount, data.TimescaleTick))
				peers = append(peers, peer)
			}
			join.Peers(peers...)
			join = join.Write()

			next := training.Step(join)
			So(next, ShouldNotBeNil)
			So(next.Source, ShouldEqual, "training")
		})
	})
}
