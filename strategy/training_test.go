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
	"github.com/theapemachine/symm/store"
	"github.com/theapemachine/symm/system"
)

type testFillingDesk struct {
	desk    *broker.Desk
	price   *broker.Price
	writes  chan string
	volumes chan *decimal.Decimal
}

func (filling *testFillingDesk) Read(keys ...string) ([]byte, error) {
	return nil, nil
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
		client := store.NewClient(ctx, "symm")

		Convey("NewTraining requires price, desk and client", func() {
			tr1 := NewTraining(ctx, nil, filling.desk, client)
			So(tr1.Error(), ShouldNotBeNil)

			tr2 := NewTraining(ctx, filling.price, nil, client)
			So(tr2.Error(), ShouldNotBeNil)

			tr3 := NewTraining(ctx, filling.price, filling.desk, nil)
			So(tr3.Error(), ShouldNotBeNil)
		})

		Convey("ReloadKeys without client policy stays idle", func() {
			training := NewTraining(ctx, filling.price, filling.desk, client)
			So(training.Error(), ShouldBeNil)
			current := training.policy.Load()
			So(len(current.actions), ShouldEqual, 0)
		})

		Convey("CognitionTree exports stored keys", func() {
			training := NewTraining(ctx, filling.price, filling.desk, client)
			tree := training.CognitionTree()
			So(tree.Keys, ShouldNotBeNil)
		})

		Convey("Step emits region metrics and meta", func() {
			training := NewTraining(ctx, filling.price, filling.desk, client)
			now := time.Now().UTC()
			join := data.NewMeasurement(100, "BTC/USD", "runtime:join", now, now)

			pins := [][2]string{
				{"hawkes", "arrival_rate:buy"},
				{"hawkes", "conditional_intensity_velocity"},
				{"hawkes", "arrival_rate:sell"},
				{"cvd", "midpoint_log_return"},
				{"cvd", "flow_aligned_midpoint_return"},
				{"toxicity", "retreat_fraction:bid"},
				{"liquidity", "best_ask_price"},
				{"depthflow", "book_imbalance"},
				{"depthflow", "net_displayed_flow:bid"},
				{"leadlag", "led_peer_share"},
				{"correlation", "historical_path_distance"},
				{"correlation", "cohort_covariance_score"},
			}

			peers := make([]*data.Measurement, 0, len(pins))
			for idx, pin := range pins {
				peer := data.NewMeasurement(100, "BTC/USD", pin[0], now, now)
				peer = peer.Write(data.NewMetric(pin[1], float64(idx+2), data.UnitCount, data.TimescaleTick))
				peers = append(peers, peer)
			}
			join.Peers(peers...)
			join = join.Write()

			next := training.Step(join)
			So(next, ShouldNotBeNil)
			So(next.Source, ShouldEqual, "training")
		})

		Convey("Step matches suffix and sets meta when policy is active", func() {
			training := NewTraining(ctx, filling.price, filling.desk, client)
			now := time.Now().UTC()
			join := data.NewMeasurement(100, "BTC/USD", "runtime:join", now, now)

			pins := [][2]string{
				{"hawkes", "arrival_rate:buy"},
				{"hawkes", "conditional_intensity_velocity"},
				{"hawkes", "arrival_rate:sell"},
				{"cvd", "midpoint_log_return"},
				{"cvd", "flow_aligned_midpoint_return"},
				{"toxicity", "retreat_fraction:bid"},
				{"liquidity", "best_ask_price"},
				{"depthflow", "book_imbalance"},
				{"depthflow", "net_displayed_flow:bid"},
				{"leadlag", "led_peer_share"},
				{"correlation", "historical_path_distance"},
				{"correlation", "cohort_covariance_score"},
			}

			peers := make([]*data.Measurement, 0, len(pins))
			for idx, pin := range pins {
				peer := data.NewMeasurement(100, "BTC/USD", pin[0], now, now)
				peer = peer.Write(data.NewMetric(pin[1], float64(idx+2), data.UnitCount, data.TimescaleTick))
				peers = append(peers, peer)
			}
			join.Peers(peers...)
			join = join.Write()

			training.policy.Store(&policy{
				actions: map[string]string{
					"R00/R01": "noop",
				},
			})

			next := training.Step(join)
			So(next, ShouldNotBeNil)
			So(next.Source, ShouldEqual, "training")
		})
	})
}
