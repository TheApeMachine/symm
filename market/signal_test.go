package market

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/datura"
)

func TestIngestFrameMatches(t *testing.T) {
	Convey("Given websocket-shaped ingest rows", t, func() {
		ticker := datura.Acquire("kraken:public", datura.APPJSON)
		ticker.WithRole("ticker")
		ticker.WithScope("update")
		ticker.WithPayload([]byte(`{"channel":"ticker","type":"update"}`))

		Convey("It should match ticker role", func() {
			So(IngestFrameMatches(ticker, "ticker"), ShouldBeTrue)
			So(IngestFrameMatches(ticker, "book"), ShouldBeFalse)
		})

		snapshot := datura.Acquire("kraken:public", datura.APPJSON)
		snapshot.WithRole("book")
		snapshot.WithScope("snapshot")
		snapshot.WithPayload([]byte(`{"channel":"book","type":"snapshot"}`))

		Convey("It should match book snapshots", func() {
			So(IngestFrameMatches(snapshot, "book"), ShouldBeTrue)
		})
	})
}
