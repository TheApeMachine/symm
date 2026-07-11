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
	"github.com/theapemachine/symm/types"
	"github.com/theapemachine/symm/ui"

	. "github.com/smartystreets/goconvey/convey"
)

func init() {
	// NewBook/NewTicker/NewTrade/NewOHLC/NewLevel3 size their SPSC feed rings
	// from viper; tests never load cmd/cfg/config.yml, so without this the
	// ring constructor sees capacity 0, fails its positive-power-of-two
	// check, and returns nil.
	viper.Set("signals.feed_ring_capacity", 128)
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

func (signal *nilSignal) Categories() []types.CategoryType {
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

func (signal *recordingSignal) Categories() []types.CategoryType {
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

func (signal *blockingSignal) Categories() []types.CategoryType {
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

func (signal *benchmarkSignal) Categories() []types.CategoryType {
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
