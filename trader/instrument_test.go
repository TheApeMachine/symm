package trader

import (
	"sync"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/types"
)

func TestInstrumentOn(t *testing.T) {
	Convey("Given an instrument snapshot frame", t, func() {
		instrument := &Instrument{
			status: types.INITIALIZING,
			cache:  &sync.Map{},
			quote:  "EUR",
		}

		raw := []byte(`{"channel":"instrument","type":"snapshot","data":{"pairs":[{"symbol":"BTC/USD","base":"BTC","quote":"USD","status":"online"}]}}`)

		Convey("When the frame is ingested", func() {
			instrument.On(raw)

			Convey("Then the pair is cached", func() {
				pair, err := instrument.Pair("BTC/USD")
				So(err, ShouldBeNil)
				So(pair.Symbol, ShouldEqual, "BTC/USD")
				So(pair.Status, ShouldEqual, "online")
			})
		})
	})
}

func BenchmarkInstrumentOn(b *testing.B) {
	instrument := &Instrument{
		status: types.INITIALIZING,
		cache:  &sync.Map{},
		quote:  "EUR",
	}
	raw := []byte(`{"channel":"instrument","type":"snapshot","data":{"pairs":[{"symbol":"BTC/USD","base":"BTC","quote":"USD","status":"online"}]}}`)

	b.ReportAllocs()

	for b.Loop() {
		instrument.On(raw)
	}
}
