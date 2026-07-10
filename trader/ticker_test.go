package trader

import (
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"

	. "github.com/smartystreets/goconvey/convey"
)

func TestTickerMeasure(t *testing.T) {
	Convey("Given a ticker with a typed signal", t, func() {
		recording := &recordingSignal{}
		crossSection, crossSectionErr := types.NewCrossSection(
			types.DefaultCrossSectionConfig(),
		)
		pool := testPool()
		ticker := NewTicker(pool, &Signal{
			Ticker:       []types.Signal[any]{recording},
			CrossSection: crossSection,
		}, testUIHub())
		raw := []byte(`[{"symbol":"BTC/USD","bid":99,"ask":101,"last":100,"volume":12.5,"timestamp":"2026-07-04T12:00:00Z"}]`)

		Convey("When ticker data is measured", func() {
			pushRing(ticker.ring, raw)
			measurements, err := ticker.Measure()

			Convey("It should measure each row through the signal", func() {
				So(crossSectionErr, ShouldBeNil)
				So(err, ShouldBeNil)
				So(measurements, ShouldHaveLength, 1)
				So(recording.rows, ShouldHaveLength, 1)
				row := recording.rows[0].(kraken.TickerData)
				So(row.Symbol, ShouldEqual, "BTC/USD")
				So(recording.crossSection.Symbols(), ShouldResemble, []string{"BTC/USD"})
			})
		})
	})

	Convey("Given a ticker with independent signals", t, func() {
		started := make(chan struct{}, 2)
		release := make(chan struct{})
		crossSection, crossSectionErr := types.NewCrossSection(
			types.DefaultCrossSectionConfig(),
		)
		pool := testPool()
		ticker := NewTicker(pool, &Signal{
			Ticker: []types.Signal[any]{
				&blockingSignal{started: started, release: release},
				&blockingSignal{started: started, release: release},
			},
			CrossSection: crossSection,
		}, testUIHub())
		raw := []byte(`[{"symbol":"BTC/USD","bid":99,"ask":101,"last":100,"volume":12.5,"timestamp":"2026-07-04T12:00:00Z"}]`)
		result := make(chan error, 1)

		pushRing(ticker.ring, raw)

		go func() {
			_, err := ticker.Measure()
			result <- err
		}()

		Convey("When ticker data is measured", func() {
			for index := 0; index < 2; index++ {
				select {
				case <-started:
				case <-time.After(time.Second):
					t.Fatal("ticker signals did not start concurrently")
				}
			}

			close(release)

			Convey("Then signal measurement should not serialize independent work", func() {
				So(crossSectionErr, ShouldBeNil)
				select {
				case err := <-result:
					So(err, ShouldBeNil)
				case <-time.After(time.Second):
					t.Fatal("ticker measurement did not complete")
				}
			})
		})
	})

	Convey("Given a ticker with a signal that emits no measurements", t, func() {
		crossSection, crossSectionErr := types.NewCrossSection(
			types.DefaultCrossSectionConfig(),
		)
		pool := testPool()
		hub := testUIHub()
		ticker := NewTicker(pool, &Signal{
			Ticker:       []types.Signal[any]{&nilSignal{}},
			CrossSection: crossSection,
		}, hub)
		raw := []byte(`[{"symbol":"BTC/USD","bid":99,"ask":101,"last":100,"volume":12.5,"timestamp":"2026-07-04T12:00:00Z"}]`)

		Convey("When ticker data is measured", func() {
			pushRing(ticker.ring, raw)
			measurements, err := ticker.Measure()

			Convey("Then it should not publish null measurements", func() {
				So(crossSectionErr, ShouldBeNil)
				So(err, ShouldBeNil)
				So(measurements, ShouldHaveLength, 0)

				select {
				case msg := <-hub.Messages:
					So(string(msg), ShouldNotContainSubstring, `"measurements":null`)
				default:
				}
			})
		})
	})
}

func TestTickerApply(t *testing.T) {
	Convey("Given a ticker feed with a stored snapshot", t, func() {
		pool := testPool()
		ticker := NewTicker(pool, &Signal{}, testUIHub())
		snapshot := kraken.TickerData{
			Symbol:    "BTC/USD",
			Bid:       *decimal.NewFromFloat64(99),
			Ask:       *decimal.NewFromFloat64(101),
			Last:      *decimal.NewFromFloat64(100),
			Volume:    12.5,
			Timestamp: time.Date(2026, 7, 4, 12, 0, 0, 0, time.UTC),
		}

		_, _ = ticker.apply(snapshot)

		Convey("When a partial update arrives", func() {
			update := kraken.TickerData{
				Symbol:    "BTC/USD",
				Last:      *decimal.NewFromFloat64(101),
				Timestamp: time.Date(2026, 7, 4, 12, 0, 1, 0, time.UTC),
			}

			merged, ready := ticker.apply(update)

			Convey("Then it should merge onto the snapshot before measuring", func() {
				So(ready, ShouldBeTrue)
				So(merged.Last.Float64(), ShouldEqual, 101)
				So(merged.Bid.Float64(), ShouldEqual, 99)
				So(merged.Ask.Float64(), ShouldEqual, 101)
				So(merged.Volume, ShouldEqual, 12.5)
			})
		})

		Convey("When only a symbol is present", func() {
			_, ready := ticker.apply(kraken.TickerData{Symbol: "ETH/USD"})

			Convey("Then it should wait for required fields", func() {
				So(ready, ShouldBeFalse)
			})
		})
	})
}

func BenchmarkTickerMeasure(b *testing.B) {
	crossSection, err := types.NewCrossSection(types.DefaultCrossSectionConfig())
	if err != nil {
		b.Fatal(err)
	}

	pool := testPool()
	ticker := NewTicker(pool, &Signal{
		Ticker:       []types.Signal[any]{&benchmarkSignal{}},
		CrossSection: crossSection,
	}, nil)
	raw := []byte(`[{"symbol":"BTC/USD","bid":99,"ask":101,"last":100,"volume":12.5,"timestamp":"2026-07-04T12:00:00Z"}]`)

	b.ReportAllocs()
	for b.Loop() {
		pushRing(ticker.ring, raw)
		if _, err := ticker.Measure(); err != nil {
			b.Fatal(err)
		}
	}
}
