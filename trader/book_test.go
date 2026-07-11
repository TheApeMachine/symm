package trader

import (
	"context"
	"os"
	"testing"

	"github.com/spf13/viper"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/tests"
	"github.com/theapemachine/symm/types"

	. "github.com/smartystreets/goconvey/convey"
)

func newTestInstrument(pairs ...kraken.InstrumentPair) *Instrument {
	pool := testPool()
	instrument := NewInstrument(pool, &writeConn{}, &writeConn{}, &writeConn{}, nil)
	instrument.status = types.READY

	for _, pair := range pairs {
		instrument.cache.Store(pair.Symbol, pair)
	}

	return instrument
}

func TestBookMeasure(t *testing.T) {
	Convey("Given a book with a typed signal", t, func() {
		pool := testPool()
		recording := &recordingSignal{}
		instrument := newTestInstrument(kraken.InstrumentPair{
			Symbol:         "MATIC/USD",
			PriceIncrement: tests.Decimal(t, "0.0001"),
			QtyPrecision:   8,
		})
		book := NewBook(pool, &Signal{Book: []types.Signal[any]{recording}}, testUIHub(), instrument)
		raw, readErr := os.ReadFile("../tests/fixtures/book/fixtures/snapshot.json")
		So(readErr, ShouldBeNil)

		Convey("When book data is measured", func() {
			pushRing(book.ring, raw)
			measurements, err := book.Measure()

			Convey("It should measure each row through the signal", func() {
				So(err, ShouldBeNil)
				So(measurements, ShouldHaveLength, 1)
				So(recording.rows, ShouldHaveLength, 1)
				row := recording.rows[0].(kraken.BookData)
				So(row.Symbol, ShouldEqual, "MATIC/USD")
			})
		})
	})
}

func TestBookMeasureWithFluidSignal(t *testing.T) {
	Convey("Given a book feed wired with the fluid signal", t, func() {
		pool := testPool()
		signal := NewSignal(context.Background())
		instrument := newTestInstrument(kraken.InstrumentPair{
			Symbol:         "MATIC/USD",
			PriceIncrement: tests.Decimal(t, "0.0001"),
			QtyPrecision:   8,
		})
		book := NewBook(pool, signal, testUIHub(), instrument)
		raw, readErr := os.ReadFile("../tests/fixtures/book/fixtures/snapshot.json")
		So(readErr, ShouldBeNil)

		Convey("When a live snapshot is measured", func() {
			pushRing(book.ring, raw)
			_, err := book.Measure()

			Convey("Then it should not panic while configuring the fluid grid", func() {
				So(err, ShouldBeNil)
			})
		})
	})
}

func TestBookReconcile(t *testing.T) {
	Convey("Given a book fed Kraken's own documented checksum worked example", t, func() {
		pool := testPool()
		public := &writeConn{}
		instrument := NewInstrument(pool, public, &writeConn{}, &writeConn{}, nil)
		instrument.status = types.READY
		instrument.cache.Store("BTC/USD", kraken.InstrumentPair{
			Symbol:         "BTC/USD",
			PriceIncrement: tests.Decimal(t, "0.1"),
			QtyPrecision:   8,
		})
		book := NewBook(pool, &Signal{Book: []types.Signal[any]{&recordingSignal{}}}, testUIHub(), instrument)
		rows := kraken.NewBookDataSlice([]byte(btcUSDChecksumSnapshot))

		Convey("When the row's checksum validates", func() {
			ok := book.reconcile(rows[0])

			Convey("Then the book is trustworthy and no resubscription is sent", func() {
				So(ok, ShouldBeTrue)
				So(public.writes, ShouldEqual, 0)
			})
		})

		Convey("When a subsequent row corrupts the checksum", func() {
			book.reconcile(rows[0])

			corrupted := kraken.NewBookDataSlice([]byte(`{"channel":"book","type":"update","data":[{
				"symbol": "BTC/USD",
				"bids": [{"price": 45283.5, "qty": 999}],
				"asks": [],
				"checksum": 1,
				"timestamp": "2023-10-06T17:35:55.440295Z"
			}]}`))
			ok := book.reconcile(corrupted[0])

			Convey("Then the book is untrustworthy and the symbol is resubscribed exactly once", func() {
				So(ok, ShouldBeFalse)
				So(public.writes, ShouldEqual, 2)
			})

			Convey("Then a further corrupt row does not resubscribe again", func() {
				again := book.reconcile(corrupted[0])
				So(again, ShouldBeFalse)
				So(public.writes, ShouldEqual, 2)
			})

			Convey("Then a fresh valid snapshot recovers the book", func() {
				recovered := book.reconcile(rows[0])
				So(recovered, ShouldBeTrue)
			})
		})
	})
}

func TestBookOn(t *testing.T) {
	Convey("Given a book ring at capacity", t, func() {
		capacity := 8 * 1024
		previousCapacity := viper.GetInt("signals.feed_ring_capacity")
		viper.Set("signals.feed_ring_capacity", capacity)
		defer viper.Set("signals.feed_ring_capacity", previousCapacity)

		pool := testPool()
		book := NewBook(pool, &Signal{Book: []types.Signal[any]{}}, testUIHub(), newTestInstrument())

		for index := range capacity {
			pushRing(book.ring, []byte{byte(index)})
		}

		So(book.ring.Len(), ShouldEqual, capacity)

		Convey("When another frame arrives", func() {
			book.On([]byte("latest"))

			Convey("Then it should keep the latest frame without erroring", func() {
				So(book.ring.Len(), ShouldEqual, capacity)
			})
		})
	})
}

func BenchmarkBookMeasure(b *testing.B) {
	pool := testPool()
	instrument := newTestInstrument(kraken.InstrumentPair{
		Symbol:         "MATIC/USD",
		PriceIncrement: tests.Decimal(b, "0.0001"),
		QtyPrecision:   8,
	})
	book := NewBook(pool, &Signal{Book: []types.Signal[any]{
		&benchmarkSignal{},
	}}, benchUIHub(), instrument)
	raw, err := os.ReadFile("../tests/fixtures/book/fixtures/snapshot.json")
	if err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	for b.Loop() {
		pushRing(book.ring, raw)
		if _, err := book.Measure(); err != nil {
			b.Fatal(err)
		}
	}
}
