package broker

import (
	"strings"
	"sync"
	"testing"

	"github.com/bytedance/sonic"
	. "github.com/smartystreets/goconvey/convey"
)

type recordingStopStore struct {
	mu   sync.Mutex
	keys []string
	vals [][]byte
}

func (store *recordingStopStore) WriteAtPrefix(key string, value []byte) (int, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	store.keys = append(store.keys, key)
	store.vals = append(store.vals, append([]byte(nil), value...))
	return len(value), nil
}

func TestNewStopHistory(t *testing.T) {
	Convey("Setup", t, func() {
		Convey("When NewStopHistory is called without a store", func() {
			history := NewStopHistory("paper", nil)

			Convey("Note is a silent no-op", func() {
				So(history, ShouldNotBeNil)
				So(history.tierKey, ShouldEqual, "paper")
				So(func() {
					history.Note("BTC/USD", "id", "beat", []byte(`{}`))
				}, ShouldNotPanic)
			})
		})
	})
}

func TestStopHistoryNote(t *testing.T) {
	Convey("Setup", t, func() {
		store := &recordingStopStore{}
		history := &StopHistory{store: store, tierKey: "paper"}
		sl := NewStoploss(1.5)
		sl.State = StoplossArmed
		body, err := sonic.Marshal(sl)
		So(err, ShouldBeNil)
		beat := sl.beat()

		Convey("When Note parks a stoploss snapshot", func() {
			history.Note("BTC/USD", "pos-1", beat, body)

			Convey("Key is tier/stops/symbol/id/nanos and payload round-trips", func() {
				So(len(store.keys), ShouldEqual, 1)
				So(strings.HasPrefix(store.keys[0], "paper/stops/BTC/USD/pos-1/"), ShouldBeTrue)
				parts := strings.Split(store.keys[0], "/")
				So(len(parts[len(parts)-1]), ShouldEqual, 19)

				var got Stoploss
				So(sonic.Unmarshal(store.vals[0], &got), ShouldBeNil)
				So(got.State, ShouldEqual, StoplossArmed)
				So(got.PeakPnLPct, ShouldEqual, 1.5)
			})
		})

		Convey("When Note is called twice with the same beat", func() {
			history.Note("BTC/USD", "pos-1", beat, body)
			history.Note("BTC/USD", "pos-1", beat, body)

			Convey("Only one object is written", func() {
				So(len(store.keys), ShouldEqual, 1)
			})
		})

		Convey("When beat changes", func() {
			history.Note("BTC/USD", "pos-1", beat, body)
			sl.State = StoplossTriggered
			sl.TriggerReason = "trail_confirm"
			next, err := sonic.Marshal(sl)
			So(err, ShouldBeNil)
			history.Note("BTC/USD", "pos-1", sl.beat(), next)

			Convey("A second temporal key is written", func() {
				So(len(store.keys), ShouldEqual, 2)
				So(store.keys[0], ShouldNotEqual, store.keys[1])
			})
		})

		Convey("When body, id, or beat is empty", func() {
			history.Note("BTC/USD", "pos-1", beat, nil)
			history.Note("BTC/USD", "", beat, body)
			history.Note("BTC/USD", "pos-1", "", body)

			Convey("Nothing is written", func() {
				So(len(store.keys), ShouldEqual, 0)
			})
		})
	})
}
