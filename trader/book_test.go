package trader

import (
	"testing"
	"time"

	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"

	. "github.com/smartystreets/goconvey/convey"
)

func TestBookMeasure(testingTB *testing.T) {
	Convey("Given a book with a typed signal", testingTB, func() {
		recording := &recordingSignal{}
		book := NewBook([]types.Signal[any]{recording})
		book.ObserveInstruments(kraken.InstrumentData{
			Pairs: []kraken.InstrumentPair{{
				Symbol:         "MATIC/USD",
				PriceIncrement: testDecimal("0.0001"),
			}},
		})
		message := kraken.BookDataSlice{{
			Symbol: "MATIC/USD",
			Bids: []kraken.BookLevel{{
				Price: testDecimal("0.5666"),
				Qty:   4831.75496356,
			}},
			Asks: []kraken.BookLevel{{
				Price: testDecimal("0.5668"),
				Qty:   4410.79769741,
			}},
			Checksum:  2439117997,
			Timestamp: time.Date(2026, 7, 4, 12, 0, 0, 0, time.UTC),
		}}

		Convey("When book data is measured", func() {
			measurements, err := book.Measure(message)

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

func BenchmarkBookMeasure(benchmarkTB *testing.B) {
	book := NewBook([]types.Signal[any]{
		&benchmarkSignal{},
	})
	book.ObserveInstruments(kraken.InstrumentData{
		Pairs: []kraken.InstrumentPair{{
			Symbol:         "MATIC/USD",
			PriceIncrement: testDecimal("0.0001"),
		}},
	})
	message := kraken.BookDataSlice{{
		Symbol: "MATIC/USD",
		Bids: []kraken.BookLevel{{
			Price: testDecimal("0.5666"),
			Qty:   4831.75496356,
		}},
		Asks: []kraken.BookLevel{{
			Price: testDecimal("0.5668"),
			Qty:   4410.79769741,
		}},
		Checksum:  2439117997,
		Timestamp: time.Date(2026, 7, 4, 12, 0, 0, 0, time.UTC),
	}}

	benchmarkTB.ReportAllocs()
	for benchmarkTB.Loop() {
		if _, err := book.Measure(message); err != nil {
			benchmarkTB.Fatal(err)
		}
	}
}
