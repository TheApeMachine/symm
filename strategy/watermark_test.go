package strategy

import (
	"context"
	"net/http/httptest"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/data"
	"github.com/theapemachine/symm/store"
	"github.com/theapemachine/symm/store/storetest"
)

func TestTrainerWatermark(t *testing.T) {
	Convey("Given an epoch whose ticks 1..5 detect labelled, and collect still writing", t, func() {
		backend := storetest.NewBackend()
		server := httptest.NewServer(backend)
		defer server.Close()
		backend.Buckets["symm"] = true
		client := store.NewClient(context.Background(), "symm", storetest.Options(server.URL))

		const epoch = uint64(42)
		storeTick := func(tick uint64) {
			value := data.NewValue(data.TickOrigin, data.TickRole, data.TickScope)
			value.Epoch, value.Tick = epoch, tick
			value.FormatPrefix()
			backend.Objects[value.Prefix] = value.Bytes()
		}

		for tick := uint64(1); tick <= 8; tick++ {
			storeTick(tick)
		}

		watermark := data.DetectorWatermark{Epoch: epoch, FirstTick: 1, LastTick: 5, Ticks: 5, Symbols: 1}

		replay := func() ([]uint64, uint64, error) {
			seen := make([]uint64, 0)
			grown, err := replayThrough(client, watermark, func(value *data.Value) error {
				seen = append(seen, value.Tick)
				return nil
			})
			return seen, grown, err
		}

		Convey("Without a watermark train refuses the epoch", func() {
			_, err := readWatermark(client, epoch)
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "run detect")
		})

		Convey("With one it reads it back exactly", func() {
			stored := watermark.Value()
			backend.Objects[stored.Prefix] = stored.Bytes()

			read, err := readWatermark(client, epoch)
			So(err, ShouldBeNil)
			So(read, ShouldResemble, watermark)

			Convey("and a second watermark for the epoch is a conflict", func() {
				other := watermark
				other.LastTick, other.Ticks = 6, 6
				value := other.Value()
				backend.Objects[value.Prefix] = value.Bytes()

				_, err := readWatermark(client, epoch)
				So(err, ShouldNotBeNil)
			})
		})

		Convey("Replay stops at the watermark and reports the first tick past it", func() {
			seen, grown, err := replay()
			So(err, ShouldBeNil)
			So(seen, ShouldResemble, []uint64{1, 2, 3, 4, 5})
			So(grown, ShouldEqual, 6)
		})

		Convey("An epoch that has not grown reports nothing past the watermark", func() {
			watermark.LastTick, watermark.Ticks = 8, 8
			seen, grown, err := replay()
			So(err, ShouldBeNil)
			So(len(seen), ShouldEqual, 8)
			So(grown, ShouldEqual, 0)
		})

		Convey("Ticks up to the watermark that differ from what detect read are refused", func() {
			delete(backend.Objects, data.TickPrefix(epoch)+"0000000000000000003.bin")
			_, _, err := replay()
			So(err, ShouldNotBeNil)

			watermark.FirstTick, watermark.Ticks = 2, 3
			_, _, err = replay()
			So(err, ShouldNotBeNil)
		})
	})
}
