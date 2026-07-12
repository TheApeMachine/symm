package trader

import (
	"context"
	"testing"
	"time"

	"github.com/spf13/viper"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/logic/manifold"
	"github.com/theapemachine/symm/types"

	. "github.com/smartystreets/goconvey/convey"
)

type level3RecoveryBook struct {
	reset chan string
}

func (book *level3RecoveryBook) Apply(kraken.Level3Data, int, int) bool { return true }

func (book *level3RecoveryBook) TopOfBook(string) (float64, float64, bool) {
	return 99, 101, true
}

func (book *level3RecoveryBook) InvalidReason(string) manifold.InvalidReason {
	return manifold.Valid
}

func (book *level3RecoveryBook) Reset(symbol string) {
	book.reset <- symbol
}

func receiveLevel3Row(t *testing.T, rows <-chan kraken.Level3Data) kraken.Level3Data {
	t.Helper()

	select {
	case row := <-rows:
		return row
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for level3 row")
		return kraken.Level3Data{}
	}
}

func TestLevel3Recover(t *testing.T) {
	Convey("Given a level3 observation whose authoritative state diverged", t, func() {
		viper.Set("market.l3_ring_capacity", 8)
		viper.Set("market.universe.trading_tier_size", 3)
		results := make(chan manifold.ProcessResult, 3)
		results <- manifold.ProcessResult{
			State: manifold.State{InvalidReason: manifold.ChecksumFailed},
		}
		results <- manifold.ProcessResult{
			State: manifold.State{InvalidReason: manifold.ChecksumFailed},
		}
		results <- manifold.ProcessResult{AdvanceReady: true}
		analyzer := &level3AnalyzerStub{
			observed: make(chan string, 3),
			advanced: make(chan string, 1),
			rows:     make(chan kraken.Level3Data, 3),
			results:  results,
		}
		instrument := &level3InstrumentStub{refreshed: make(chan string, 2)}
		book := &level3RecoveryBook{reset: make(chan string, 2)}
		level3 := NewLevel3(
			context.Background(),
			&Signal{Level3: []types.Signal[any]{&level3SignalStub{}}},
			nil,
			instrument,
			analyzer,
			book,
		)
		So(level3, ShouldNotBeNil)
		defer level3.Close()

		Convey("When deltas, a failed replacement, and a valid snapshot arrive", func() {
			level3.On([]byte(`{
				"channel":"level3",
				"type":"update",
				"data":[{"symbol":"BTC/USD","timestamp":"2026-07-12T00:00:01Z"}]
			}`))
			So(receiveLevel3Row(t, analyzer.rows).Type, ShouldEqual, "update")
			So(receiveSymbol(t, instrument.refreshed), ShouldEqual, "BTC/USD")
			So(receiveSymbol(t, book.reset), ShouldEqual, "BTC/USD")

			level3.On([]byte(`{
				"channel":"level3",
				"type":"update",
				"data":[
					{"symbol":"BTC/USD","type":"update","timestamp":"2026-07-12T00:00:02Z"},
					{"symbol":"BTC/USD","type":"snapshot","timestamp":"2026-07-12T00:00:03Z"}
				]
			}`))
			So(receiveLevel3Row(t, analyzer.rows).Type, ShouldEqual, "snapshot")

			level3.On([]byte(`{
				"channel":"level3",
				"type":"snapshot",
				"data":[{"symbol":"BTC/USD","timestamp":"2026-07-12T00:00:04Z"}]
			}`))
			So(receiveLevel3Row(t, analyzer.rows).Type, ShouldEqual, "snapshot")
			So(receiveSymbol(t, analyzer.advanced), ShouldEqual, "BTC/USD")

			Convey("Then recovery resets and refreshes once, ignores deltas, and clears on validation", func() {
				_, recovering := level3.recovering["BTC/USD"]
				So(recovering, ShouldBeFalse)
				So(level3.Status(), ShouldEqual, types.READY)

				So(len(instrument.refreshed), ShouldEqual, 0)
				So(len(book.reset), ShouldEqual, 0)
			})
		})
	})
}
