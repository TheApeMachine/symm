package utils

import (
	"testing"

	"github.com/theapemachine/symm/kraken"

	. "github.com/smartystreets/goconvey/convey"
)

func TestUnmarshal(t *testing.T) {
	Convey("Given a Kraken ticker channel frame", t, func() {
		raw := []byte(`{"channel":"ticker","type":"snapshot","data":[{"symbol":"BTC/USD","bid":99,"ask":101,"last":100,"volume":12.5,"timestamp":"2026-07-04T12:00:00Z"}]}`)

		Convey("When it is unmarshaled into the typed envelope", func() {
			frame := Unmarshal[kraken.Ticker](raw)

			Convey("Then the data rows are decoded", func() {
				So(frame.Data, ShouldHaveLength, 1)
				So(frame.Data[0].Symbol, ShouldEqual, "BTC/USD")
				So(frame.Data[0].Last.Float64(), ShouldEqual, 100)
			})
		})
	})
}

func TestGetBytes(t *testing.T) {
	Convey("Given a Kraken ticker channel frame", t, func() {
		raw := []byte(`{"channel":"ticker","type":"snapshot","data":[{"symbol":"BTC/USD","bid":99,"ask":101,"last":100,"volume":12.5,"timestamp":"2026-07-04T12:00:00Z"}]}`)

		Convey("When the data path is read", func() {
			_, err := GetBytes(raw, "data")

			Convey("Then it should return the raw data array", func() {
				So(err, ShouldBeNil)

				rows := kraken.NewTicker(raw).Data
				So(rows, ShouldHaveLength, 1)
				So(rows[0].Symbol, ShouldEqual, "BTC/USD")
				So(rows[0].Last.Float64(), ShouldEqual, 100)
			})
		})
	})

	Convey("Given a Kraken instrument channel frame", t, func() {
		raw := []byte(`{"channel":"instrument","type":"snapshot","data":{"pairs":[{"symbol":"BTC/USD","base":"BTC","quote":"USD","status":"online"}]}}`)

		Convey("When the data path is read", func() {
			payload, err := GetBytes(raw, "data")

			Convey("Then it should return the raw data object", func() {
				So(err, ShouldBeNil)

				instrument := kraken.NewInstrumentData(payload)
				So(instrument.Pairs, ShouldHaveLength, 1)
				So(instrument.Pairs[0].Symbol, ShouldEqual, "BTC/USD")
			})
		})
	})
}

func TestFrameData(t *testing.T) {
	Convey("Given a Kraken book channel frame", t, func() {
		raw := []byte(`{"channel":"book","type":"snapshot","data":[{"symbol":"MATIC/USD","bids":[{"price":0.5666,"qty":4831.75496356}],"asks":[{"price":0.5668,"qty":4410.79769741}],"checksum":2439117997,"timestamp":"2023-10-06T17:35:55.440295Z"}]}`)

		Convey("When the frame type and data are preserved", func() {
			_, err := FrameData(raw)

			Convey("Then book rows should carry the envelope type", func() {
				So(err, ShouldBeNil)

				books := kraken.NewBook(raw).Data
				So(books, ShouldHaveLength, 1)
				So(books[0].Type, ShouldEqual, "snapshot")
				So(books[0].Symbol, ShouldEqual, "MATIC/USD")
				So(books[0].Bids[0].Price.Float64(), ShouldAlmostEqual, 0.5666)
			})
		})
	})
}

func TestGetString(t *testing.T) {
	Convey("Given a Kraken channel frame", t, func() {
		raw := []byte(`{"channel":"ticker","type":"snapshot","data":[]}`)

		Convey("When the channel path is read", func() {
			channel := GetString(raw, "channel")

			Convey("Then it should return the channel name", func() {
				So(channel, ShouldEqual, "ticker")
			})
		})
	})
}

func BenchmarkUnmarshal(b *testing.B) {
	raw := []byte(`{"channel":"ticker","type":"snapshot","data":[{"symbol":"BTC/USD","bid":99,"ask":101,"last":100,"volume":12.5,"timestamp":"2026-07-04T12:00:00Z"}]}`)

	for b.Loop() {
		_ = Unmarshal[kraken.Ticker](raw)
	}
}
