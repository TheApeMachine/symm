package trader

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/spot"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/datura/structure"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/logic/manifold"
	"github.com/theapemachine/symm/system"
	"github.com/theapemachine/symm/types"
)

func initManifoldViper() {
	viper.Set("market.l3_depth", 10)
	viper.Set("market.manifold.lifetime_capacity", 256)
	viper.Set("market.manifold_max_symbols", 64)
	viper.Set("market.forecast.rls.initial_variance", 1.0)
	viper.Set("market.forecast.rls.forgetting_factor", 1.0)
}

func TestLevel3IngressDrainIngestsOntoThesis(testingTB *testing.T) {
	Convey("Given buffered authoritative level3 rows", testingTB, func() {
		initManifoldViper()
		ctx := context.Background()
		ring, err := structure.NewMPMCRing[kraken.Level3Data](ctx, 8)
		So(err, ShouldBeNil)

		manager := seedBookManager("BTC/USD", 99, 101)

		ingress := &Level3Ingress{
			ring:    ring,
			enabled: true,
			books: func(yield func(*spot.BookManager) bool) {
				yield(manager)
			},
		}

		snapshot := kraken.Level3Data{
			Symbol:    "BTC/USD",
			Type:      "snapshot",
			Timestamp: time.Unix(5, 0),
			Bids: []kraken.Level3Order{{
				OrderID: "bid-1", LimitPrice: 99, OrderQty: 2,
				Timestamp: time.Unix(5, 0),
			}},
			Asks: []kraken.Level3Order{{
				OrderID: "ask-1", LimitPrice: 101, OrderQty: 3,
				Timestamp: time.Unix(5, 0),
			}},
		}
		update := kraken.Level3Data{
			Symbol: "BTC/USD", Type: "update", Timestamp: time.Unix(6, 0),
			Bids: []kraken.Level3Order{{
				Event: "modify", OrderID: "bid-1", LimitPrice: 99,
				OrderQty: 4, Timestamp: time.Unix(6, 0),
			}},
		}

		So(ingress.ring.Push(snapshot), ShouldBeTrue)
		So(ingress.ring.Push(update), ShouldBeTrue)

		instrument := &Instrument{cache: &sync.Map{}}
		instrument.cache.Store("BTC/USD", kraken.InstrumentPair{
			Symbol:         "BTC/USD",
			PricePrecision: 1,
			QtyPrecision:   8,
		})

		booter := system.NewBooter(ctx, nil)
		booter.AddStages(system.NewStage(system.StagePreflight))
		So(booter.Start(), ShouldBeNil)

		analyzer := logic.NewAnalyzer(booter, nil)
		thesis := types.NewThesis(nil)

		ingress.Drain(thesis, analyzer, instrument)

		Convey("Then drain appends manifold evidence onto the tick thesis", func() {
			So(thesis, ShouldNotBeNil)
			So(len(thesis.Forecasts)+len(thesis.Measurements)+len(thesis.Hypotheses), ShouldBeGreaterThan, 0)
		})
	})
}

func TestSDKLevel3BookApplyAcceptsSynchronizedBook(testingTB *testing.T) {
	Convey("Given an SDK book synchronized to a level3 checksum", testingTB, func() {
		manager := seedBookManager("ETH/USD", 10, 11)

		sdkBook := NewSDKLevel3Book(func(yield func(*spot.BookManager) bool) {
			yield(manager)
		})
		symbolBook := sdkBook.ForSymbol("ETH/USD")
		row := kraken.Level3Data{Symbol: "ETH/USD", Checksum: 0}

		Convey("When checksum is absent but touch exists", func() {
			ok := symbolBook.Apply(row, 1, 8)

			Convey("Then manifold observation can proceed", func() {
				So(ok, ShouldBeTrue)
				bid, ask, topOK := symbolBook.TopOfBook("ETH/USD")
				So(topOK, ShouldBeTrue)
				So(bid, ShouldEqual, 10)
				So(ask, ShouldEqual, 11)
				So(symbolBook.InvalidReason("ETH/USD"), ShouldEqual, manifold.Valid)
			})
		})
	})
}

func BenchmarkLevel3IngressDrain(benchmark *testing.B) {
	initManifoldViper()

	ctx := context.Background()
	ring, _ := structure.NewMPMCRing[kraken.Level3Data](ctx, 1024)
	manager := seedBookManager("BTC/USD", 99, 101)

	ingress := &Level3Ingress{
		ring:    ring,
		enabled: true,
		books: func(yield func(*spot.BookManager) bool) {
			yield(manager)
		},
	}

	row := kraken.Level3Data{
		Symbol: "BTC/USD", Type: "snapshot", Timestamp: time.Unix(1, 0),
		Bids: []kraken.Level3Order{{
			OrderID: "bid-1", LimitPrice: 99, OrderQty: 2, Timestamp: time.Unix(1, 0),
		}},
		Asks: []kraken.Level3Order{{
			OrderID: "ask-1", LimitPrice: 101, OrderQty: 3, Timestamp: time.Unix(1, 0),
		}},
	}

	instrument := &Instrument{cache: &sync.Map{}}
	instrument.cache.Store("BTC/USD", kraken.InstrumentPair{
		Symbol: "BTC/USD", PricePrecision: 1, QtyPrecision: 8,
	})

	booter := system.NewBooter(ctx, nil)
	booter.AddStages(system.NewStage(system.StagePreflight))
	booter.Start()
	analyzer := logic.NewAnalyzer(booter, nil)

	for benchmark.Loop() {
		ingress.ring.Push(row)
		thesis := types.NewThesis(nil)
		ingress.Drain(thesis, analyzer, instrument)
	}
}
