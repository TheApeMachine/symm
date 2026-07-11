package trader

import (
	"testing"

	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/tests"
	"github.com/theapemachine/symm/types"

	. "github.com/smartystreets/goconvey/convey"
)

/*
level3BTCUSDSnapshot is a minimal two-sided BTC/USD level3 snapshot whose
checksum was computed against Kraken's documented algorithm: strip the
decimal point and leading zeros from each order's limit_price and
order_qty (formatted at price_precision 1 / qty_precision 8), append
asks low-to-high then bids high-to-low, and CRC32 the result.
*/
const level3BTCUSDSnapshot = `[{"symbol":"BTC/USD","type":"snapshot","timestamp":"2026-07-04T12:00:00Z","checksum":335758147,"bids":[{"order_id":"OQCLML-BW3P3-BUCMWZ","limit_price":43125.3,"order_qty":0.15,"timestamp":"2026-07-04T12:00:00Z"}],"asks":[{"order_id":"OQCLML-BW3P3-BUCMWA","limit_price":43125.4,"order_qty":0.20,"timestamp":"2026-07-04T12:00:00Z"}]}]`

func newLevel3TestInstrument(t testing.TB) *Instrument {
	return newTestInstrument(kraken.InstrumentPair{
		Symbol:         "BTC/USD",
		PriceIncrement: tests.Decimal(t, "0.1"),
		PricePrecision: 1,
		QtyPrecision:   8,
	})
}

func TestLevel3Measure(t *testing.T) {
	Convey("Given level3 with a typed signal", t, func() {
		recording := &recordingSignal{}
		pool := testPool()
		instrument := newLevel3TestInstrument(t)
		level3 := NewLevel3(pool, &Signal{Level3: []types.Signal[any]{recording}}, testUIHub(), instrument, NewLevel3Book(10))
		raw := []byte(level3BTCUSDSnapshot)

		Convey("When level3 data is measured", func() {
			pushRing(level3.ring, raw)
			measurements, err := level3.Measure()

			Convey("It should measure each row through the signal with the reconstructed top-of-book price", func() {
				So(err, ShouldBeNil)
				So(measurements, ShouldHaveLength, 1)
				So(recording.rows, ShouldHaveLength, 1)
				row := recording.rows[0].(kraken.Level3Data)
				So(row.Symbol, ShouldEqual, "BTC/USD")
				So(measurements[0].Metrics["price"], ShouldAlmostEqual, (43125.3+43125.4)/2)
			})
		})
	})
}

func TestLevel3Reconcile(t *testing.T) {
	Convey("Given a level3 feed with a BTC/USD instrument", t, func() {
		public := &writeConn{}
		level3Conn := &writeConn{}
		pool := testPool()
		instrument := NewInstrument(pool, public, &writeConn{}, level3Conn, nil)
		instrument.status = types.READY
		instrument.cache.Store("BTC/USD", kraken.InstrumentPair{
			Symbol:         "BTC/USD",
			PriceIncrement: tests.Decimal(t, "0.1"),
			PricePrecision: 1,
			QtyPrecision:   8,
		})
		level3 := NewLevel3(pool, &Signal{}, testUIHub(), instrument, NewLevel3Book(10))
		rows := kraken.NewLevel3DataSlice([]byte(level3BTCUSDSnapshot))

		Convey("When the row's checksum validates", func() {
			ok := level3.reconcile(rows[0])

			Convey("Then the book is trustworthy and no resubscription is sent", func() {
				So(ok, ShouldBeTrue)
				So(level3Conn.writes, ShouldEqual, 0)
			})
		})

		Convey("When a subsequent row corrupts the checksum", func() {
			level3.reconcile(rows[0])

			corrupted := kraken.NewLevel3DataSlice([]byte(`[{"symbol":"BTC/USD","type":"update","checksum":1,"bids":[{"event":"add","order_id":"BAD-ORDER","limit_price":43000,"order_qty":1,"timestamp":"2026-07-04T12:00:01Z"}],"asks":[]}]`))

			Convey("Then the book is invalidated and a resubscription is sent", func() {
				ok := level3.reconcile(corrupted[0])

				So(ok, ShouldBeFalse)
				So(level3Conn.writes, ShouldEqual, 2)
			})

			Convey("Then a second consecutive failure does not resubscribe again", func() {
				level3.reconcile(corrupted[0])
				ok := level3.reconcile(corrupted[0])

				So(ok, ShouldBeFalse)
				So(level3Conn.writes, ShouldEqual, 2)
			})
		})
	})
}

func BenchmarkLevel3Measure(b *testing.B) {
	pool := testPool()
	instrument := newLevel3TestInstrument(b)
	level3 := NewLevel3(pool, &Signal{Level3: []types.Signal[any]{
		&benchmarkSignal{},
	}}, benchUIHub(), instrument, NewLevel3Book(10))
	raw := []byte(level3BTCUSDSnapshot)

	b.ReportAllocs()
	for b.Loop() {
		pushRing(level3.ring, raw)
		if _, err := level3.Measure(); err != nil {
			b.Fatal(err)
		}
	}
}
