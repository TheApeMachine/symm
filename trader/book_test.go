package trader

import (
	"sync"
	"testing"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
	bookfixture "github.com/theapemachine/symm/tests/fixtures/book"
	"github.com/theapemachine/symm/types"
)

func TestBookMeasure(t *testing.T) {
	Convey("Given a book producer that replenishes the ring during measurement", t, func() {
		frame := bookFrame()
		instrument := readyBookInstrument()
		signal := &replenishingSignal{}
		book := NewBook(&Signal{Book: []types.Signal[any]{signal}}, nil, instrument)
		signal.replenish = func() { book.On(frame) }
		book.On(frame)

		_, err := book.Measure()

		Convey("It should consume only the prefix present when the cycle began", func() {
			So(err, ShouldBeNil)
			So(book.ring.Len(), ShouldEqual, 1)
		})
	})
}

type replenishingSignal struct {
	replenish func()
}

func (signal *replenishingSignal) IngestRoles() []string {
	return nil
}

func (signal *replenishingSignal) Measure(
	_ any,
	_ *types.CrossSection,
) ([]*types.Measurement, error) {
	if signal.replenish == nil {
		return nil, nil
	}

	replenish := signal.replenish
	signal.replenish = nil
	replenish()

	return nil, nil
}

func bookFrame() []byte {
	for frame := range bookfixture.NewFixture(bookfixture.SNAPSHOT, 1).Generate() {
		return frame
	}

	return nil
}

func readyBookInstrument() *Instrument {
	cache := &sync.Map{}
	cache.Store("MATIC/USD", kraken.InstrumentPair{
		Symbol:         "MATIC/USD",
		PriceIncrement: *decimal.NewFromFloat64(0.0001),
	})

	return &Instrument{status: types.READY, cache: cache}
}
