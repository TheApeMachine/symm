package trader

import (
	"context"
	"runtime"
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/spf13/viper"
	"github.com/theapemachine/datura/structure"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/signal/pumpdump"
	"github.com/theapemachine/symm/types"
	"github.com/theapemachine/symm/ui"

	. "github.com/smartystreets/goconvey/convey"
)

func init() {
	// Feed constructors read capacities without loading cmd/cfg/config.yml in
	// package tests, so install the same valid ring and trading-tier shape here.
	viper.Set("signals.feed_ring_capacity", 128)
	viper.Set("market.l3_ring_capacity", 128)
	viper.Set("market.universe.trading_tier_size", 40)
}

func testUIHub() *ui.Hub {
	return &ui.Hub{Messages: make(chan []byte, 128)}
}

func benchUIHub() *ui.Hub {
	hub := &ui.Hub{Messages: make(chan []byte, 1024)}

	go func() {
		for range hub.Messages {
		}
	}()

	return hub
}

func testPool() *qpool.Q[any] {
	return qpool.NewQ[any](context.Background(), 2, runtime.NumCPU(), nil)
}

func pushRing(
	ring *structure.SPSCRing[[]byte],
	raw []byte,
) {
	frame := make([]byte, len(raw))
	copy(frame, raw)
	ring.Push(frame)
}

type nilSignal struct{}

func (signal *nilSignal) IngestRoles() []string {
	return nil
}

func (signal *nilSignal) Measure(
	input any,
	_ *types.CrossSection,
) ([]*types.Measurement, error) {
	return nil, nil
}

type recordingSignal struct {
	rows         []any
	crossSection *types.CrossSection
}

func (signal *recordingSignal) IngestRoles() []string {
	return nil
}

func (signal *recordingSignal) Measure(
	input any,
	crossSection *types.CrossSection,
) ([]*types.Measurement, error) {
	signal.rows = append(signal.rows, input)
	signal.crossSection = crossSection

	return []*types.Measurement{{}}, nil
}

type blockingSignal struct {
	started chan struct{}
	release chan struct{}
}

func (signal *blockingSignal) IngestRoles() []string {
	return nil
}

func (signal *blockingSignal) Measure(
	_ any,
	_ *types.CrossSection,
) ([]*types.Measurement, error) {
	signal.started <- struct{}{}
	<-signal.release

	return []*types.Measurement{{}}, nil
}

type benchmarkSignal struct{}

func (signal *benchmarkSignal) IngestRoles() []string {
	return nil
}

func (signal *benchmarkSignal) Measure(
	_ any,
	_ *types.CrossSection,
) ([]*types.Measurement, error) {
	return []*types.Measurement{{}}, nil
}

func TestSignalFluidRegistryShared(t *testing.T) {
	Convey("Given one fluid signal wired across ticker and book feeds", t, func() {
		signal := NewSignal(context.Background())
		fluidSignal := signal.Ticker[1]

		So(signal.Trade[3], ShouldEqual, fluidSignal)
		So(signal.Book[2], ShouldEqual, fluidSignal)
		eventAt := time.Date(2026, 7, 10, 4, 0, 0, 0, time.UTC)

		_, err := fluidSignal.Measure(kraken.TickerData{
			Symbol:    "BTC/USD",
			Bid:       decimal.NewFromFloat64(99),
			Ask:       decimal.NewFromFloat64(101),
			Last:      decimal.NewFromFloat64(100),
			Volume:    12.5,
			Timestamp: eventAt,
		}, nil)

		So(err, ShouldBeNil)

		Convey("When the book feed measures the same symbol", func() {
			measurements, err := signal.Book[2].Measure(kraken.BookData{
				Symbol: "BTC/USD",
				Type:   "snapshot",
				Bids: []kraken.BookLevel{
					{Price: *decimal.NewFromFloat64(99), Qty: 5},
				},
				Asks: []kraken.BookLevel{
					{Price: *decimal.NewFromFloat64(101), Qty: 5},
				},
				Timestamp: eventAt,
			}, nil)

			Convey("Then it should reuse the ticker-fed market state", func() {
				So(err, ShouldBeNil)
				So(measurements, ShouldBeNil)
			})
		})
	})
}

func TestSignalToxicityOwnedByLevel3(t *testing.T) {
	Convey("Given the composed runtime signals", t, func() {
		signal := NewSignal(context.Background())
		So(signal.Level3, ShouldHaveLength, 1)
		toxicitySignal := signal.Level3[0]

		Convey("It should not expose the same stateful toxicity signal to ordinary Trade measurement", func() {
			for _, tradeSignal := range signal.Trade {
				So(tradeSignal, ShouldNotEqual, toxicitySignal)
			}
		})
	})
}

func TestNewSignalPumpDumpOwnedByTicker(t *testing.T) {
	Convey("Given the composed runtime signals", t, func() {
		signal := NewSignal(context.Background())
		pumpdumpTickerCount := 0

		for _, tickerSignal := range signal.Ticker {
			if _, isPumpDump := tickerSignal.(*pumpdump.Signal[any]); isPumpDump {
				pumpdumpTickerCount++
			}
		}

		Convey("It should register pumpdump only on the ticker feed", func() {
			So(pumpdumpTickerCount, ShouldEqual, 1)

			for _, bookSignal := range signal.Book {
				_, isPumpDump := bookSignal.(*pumpdump.Signal[any])
				So(isPumpDump, ShouldBeFalse)
			}

			for _, tradeSignal := range signal.Trade {
				_, isPumpDump := tradeSignal.(*pumpdump.Signal[any])
				So(isPumpDump, ShouldBeFalse)
			}
		})
	})
}
