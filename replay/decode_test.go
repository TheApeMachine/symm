package replay

import (
	"encoding/json"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

type taggedRow struct {
	Type string `json:"type"`
}

func (row *taggedRow) SetEnvelopeType(kind string) {
	row.Type = kind
}

func TestDecodeWSRows(t *testing.T) {
	Convey("Given a websocket payload", t, func() {
		payload := []byte(`{"channel":"ticker","type":"update","data":[{"symbol":"BTC/EUR","last":1}]}`)

		rows, kind, err := DecodeWSRows[map[string]any](payload)

		Convey("It should decode rows and envelope type", func() {
			So(err, ShouldBeNil)
			So(kind, ShouldEqual, "update")
			So(len(rows), ShouldEqual, 1)
			So(rows[0]["symbol"], ShouldEqual, "BTC/EUR")
		})
	})

	Convey("Given tagged rows", t, func() {
		payload := []byte(`{"channel":"ticker","type":"snapshot","data":[{"symbol":"BTC/EUR"}]}`)

		rows, kind, err := DecodeWSRows[taggedRow](payload)

		Convey("It should propagate envelope type into rows", func() {
			So(err, ShouldBeNil)
			So(kind, ShouldEqual, "snapshot")
			So(rows[0].Type, ShouldEqual, "snapshot")
		})
	})
}

func TestDecodeWSSnapshot(t *testing.T) {
	Convey("Given a snapshot payload", t, func() {
		payload := []byte(`{"channel":"instrument","type":"snapshot","data":{"symbol":"BTC/EUR"}}`)

		row, err := DecodeWSSnapshot[map[string]any](payload)

		Convey("It should decode one object frame", func() {
			So(err, ShouldBeNil)
			So((*row)["symbol"], ShouldEqual, "BTC/EUR")
		})
	})
}

func BenchmarkDecodeWSRows(b *testing.B) {
	payload := json.RawMessage(`{"channel":"ticker","type":"update","data":[{"symbol":"BTC/EUR","last":1}]}`)

	for b.Loop() {
		_, _, _ = DecodeWSRows[map[string]any](payload)
	}
}
