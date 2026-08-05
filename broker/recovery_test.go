package broker_test

import (
	"bytes"
	"os"
	"slices"
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	"github.com/phuslu/log"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/symm/tests"
	testtypes "github.com/theapemachine/symm/tests/types"
	"github.com/theapemachine/symm/types"
)

func TestRecover(t *testing.T) {
	Convey("Given wallet inventory and its acquisition history at boot", t, func() {
		tradingModel := viper.GetString("trading.model")
		apiKey, hadAPIKey := os.LookupEnv("KRAKEN_API_KEY")
		apiSecret, hadAPISecret := os.LookupEnv("KRAKEN_API_SECRET")

		viper.Set("trading.model", "real")
		_ = os.Setenv("KRAKEN_API_KEY", "fixture-key")
		_ = os.Setenv("KRAKEN_API_SECRET", "Zml4dHVyZS1zZWNyZXQ=")

		defer func() {
			viper.Set("trading.model", tradingModel)

			if hadAPIKey {
				_ = os.Setenv("KRAKEN_API_KEY", apiKey)
			} else {
				_ = os.Unsetenv("KRAKEN_API_KEY")
			}

			if hadAPISecret {
				_ = os.Setenv("KRAKEN_API_SECRET", apiSecret)
			} else {
				_ = os.Unsetenv("KRAKEN_API_SECRET")
			}
		}()

		symbols := []*testtypes.Symbol{
			testtypes.NewSymbol("SIM1/USD", 100.0, 42),
			testtypes.NewSymbol("SIM2/USD", 100.0, 1337),
		}
		entryAt := time.Now().UTC().Add(-time.Hour)
		market := tests.NewMarketWithAccount(
			t.Context(),
			symbols,
			map[string]string{"USD": "150", "SIM2": "2", "USDT": "5000"},
			map[string]spot.Trade{
				"entry": {
					Pair:   "sim2usd",
					Time:   decimal.NewFromFloat64(float64(entryAt.UnixNano()) / 1e9),
					Type:   "buy",
					Cost:   decimal.NewFromInt64(300),
					Fee:    decimal.NewFromFloat64(0.78),
					Volume: decimal.NewFromInt64(3),
				},
				"partial-exit": {
					Pair:   "sim2usd",
					Time:   decimal.NewFromFloat64(float64(entryAt.Add(time.Minute).UnixNano()) / 1e9),
					Type:   "sell",
					Cost:   decimal.NewFromInt64(100),
					Fee:    decimal.NewFromFloat64(0.26),
					Volume: decimal.NewFromInt64(1),
				},
			},
		)
		logs := &bytes.Buffer{}
		originalWriter := log.DefaultLogger.Writer
		log.DefaultLogger.Writer = log.IOWriter{Writer: logs}

		defer func() {
			market.Close()
			log.DefaultLogger.Writer = originalWriter
		}()

		Convey("Recovery should adopt the known lot and skip unmanaged wallet inventory", func() {
			positions := slices.Collect(market.Desk.Positions())

			So(positions, ShouldHaveLength, 1)
			So(market.Desk.OpenPositions(), ShouldEqual, 1)
			So(positions[0].ID, ShouldEqual, "recovered:SIM2/USD")
			So(positions[0].Status, ShouldEqual, types.OPEN)
			So(positions[0].Holding.Symbol, ShouldEqual, "SIM2/USD")
			So(positions[0].Holding.Qty.Cmp(decimal.NewFromInt64(2)), ShouldEqual, 0)
			So(positions[0].Holding.SellableQty.Cmp(decimal.NewFromInt64(2)), ShouldEqual, 0)
			So(positions[0].Holding.EntryPrice.Float64(), ShouldEqual, 100.0)
			So(positions[0].Holding.EntryFee.Float64(), ShouldAlmostEqual, 0.52, 1e-8)
			So(positions[0].Holding.EntryAt, ShouldNotBeNil)
			So(positions[0].Holding.Mark, ShouldNotBeNil)
			So(positions[0].Holding.Mark.Cmp(positions[0].Holding.EntryPrice), ShouldEqual, 0)
			So(positions[0].Holding.Stoploss, ShouldNotBeNil)
			So(positions[0].Holding.Stoploss.Entry.Cmp(positions[0].Holding.EntryPrice), ShouldEqual, 0)
			So(positions[0].Holding.Stoploss.Mark.Cmp(positions[0].Holding.EntryPrice), ShouldEqual, 0)
			So(positions[0].Holding.Stoploss.Floor, ShouldBeNil)

			market.Tick()
			positions = slices.Collect(market.Desk.Positions())

			So(market.Desk.Price().Tick("sim2usd"), ShouldNotBeNil)
			So(positions[0].Holding.EntryPrice.Float64(), ShouldEqual, 100.0)
			So(positions[0].Holding.Mark.Cmp(positions[0].Holding.EntryPrice), ShouldNotEqual, 0)
			So(positions[0].Holding.Stoploss.Entry.Cmp(positions[0].Holding.EntryPrice), ShouldEqual, 0)
			So(positions[0].Holding.Stoploss.Mark.Cmp(positions[0].Holding.Mark), ShouldEqual, 0)
			So(positions[0].Holding.Stoploss.Plan.Present, ShouldBeTrue)
			So(positions[0].Holding.Stoploss.Floor, ShouldNotBeNil)
			So(positions[0].Holding.Stoploss.HardFloor.Cmp(positions[0].Holding.EntryPrice), ShouldEqual, -1)
			So(bytes.Contains(logs.Bytes(), []byte("ticker not found")), ShouldBeFalse)
		})
	})
}

func BenchmarkRecover(b *testing.B) {
	tradingModel := viper.GetString("trading.model")
	viper.Set("trading.model", "real")
	defer viper.Set("trading.model", tradingModel)

	symbols := []*testtypes.Symbol{
		testtypes.NewSymbol("SIM1/USD", 100.0, 42),
		testtypes.NewSymbol("SIM2/USD", 100.0, 1337),
	}
	entryAt := time.Now().UTC().Add(-time.Hour)

	for b.Loop() {
		market := tests.NewMarketWithAccount(
			b.Context(),
			symbols,
			map[string]string{"USD": "150", "SIM2": "2", "USDT": "5000"},
			map[string]spot.Trade{
				"entry": {
					Pair:   "sim2usd",
					Time:   decimal.NewFromFloat64(float64(entryAt.UnixNano()) / 1e9),
					Type:   "buy",
					Cost:   decimal.NewFromInt64(300),
					Fee:    decimal.NewFromFloat64(0.78),
					Volume: decimal.NewFromInt64(3),
				},
			},
		)
		market.Close()
	}
}
