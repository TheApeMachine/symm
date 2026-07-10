package trader

import (
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/spot"
	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/types"

	. "github.com/smartystreets/goconvey/convey"
)

type writeConn struct {
	writes int
}

func (conn *writeConn) Client() *spot.WebSocket {
	return nil
}

func (conn *writeConn) On(string, func([]byte)) {}

func (conn *writeConn) Write(_ json.Marshaler) error {
	conn.writes++
	return nil
}

func (conn *writeConn) Close() {}

func (conn *writeConn) Get(string, json.Marshaler) ([]byte, error) {
	return nil, nil
}

func (conn *writeConn) Post(string, json.Marshaler) ([]byte, error) {
	return nil, nil
}

var _ websocket.Conn = (*writeConn)(nil)

func ingestInstrumentSnapshot(
	instrument *Instrument,
	pool *qpool.Q[any],
	raw []byte,
) {
	done := make(chan struct{})
	pool.ScheduleFast(func() {
		frame := make([]byte, len(raw))
		copy(frame, raw)

		message := kraken.NewInstrumentData(frame)

		for _, pair := range message.Pairs {
			instrument.cache.LoadOrStore(pair.Symbol, pair)
		}

		close(done)
	})
	<-done
}

func TestInstrumentOn(t *testing.T) {
	Convey("Given instrument data for quote pairs", t, func() {
		viper.Set("market.quote_currency", "USD")
		viper.Set("market.subscribe_batch", 50)
		viper.Set("market.subscribe_pace", 20*time.Millisecond)

		pool := testPool()
		public := &writeConn{}
		private := &writeConn{}
		level3 := &writeConn{}
		instrument := NewInstrument(pool, public, private, level3, nil)

		raw, readErr := os.ReadFile("../tests/fixtures/instrument/fixtures/snapshot.json")
		So(readErr, ShouldBeNil)

		Convey("When the snapshot is ingested", func() {
			ingestInstrumentSnapshot(instrument, pool, raw)
			errnie.Error(instrument.Subscribe())

			Convey("Then pairs are cached and subscriptions are sent", func() {
				So(instrument.Status(), ShouldEqual, types.READY)
				So(public.writes, ShouldBeGreaterThan, 1)
				So(level3.writes, ShouldBeGreaterThan, 0)
			})
		})

		Convey("When the snapshot arrives after Subscribe starts", func() {
			public := &writeConn{}
			private := &writeConn{}
			level3 := &writeConn{}
			instrument := NewInstrument(pool, public, private, level3, nil)
			raw, readErr := os.ReadFile("../tests/fixtures/instrument/fixtures/snapshot.json")
			So(readErr, ShouldBeNil)

			done := make(chan error, 1)

			go func() {
				time.Sleep(20 * time.Millisecond)
				ingestInstrumentSnapshot(instrument, pool, raw)
			}()

			go func() {
				time.Sleep(40 * time.Millisecond)
				done <- instrument.Subscribe()
			}()

			var subscribeErr error

			select {
			case subscribeErr = <-done:
			case <-time.After(2 * time.Second):
				t.Fatal("instrument subscribe timed out")
			}

			Convey("Then subscriptions are still sent once the catalog arrives", func() {
				So(subscribeErr, ShouldBeNil)
				So(instrument.Status(), ShouldEqual, types.READY)
				So(public.writes, ShouldBeGreaterThan, 0)
			})
		})
	})
}
