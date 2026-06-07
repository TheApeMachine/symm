package market

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

// REST AssetPairs still speaks Kraken's legacy asset codes (XBT, XDG) while
// the v2 websocket — and therefore every tape row — speaks BTC and DOGE. A
// rules cache keyed by raw wsnames rejected BTC/EUR entries with "missing
// instrument rules" during every tune (2026-06-07).
func TestNormalizeWsname(t *testing.T) {
	Convey("Legacy codes map to v2 names on either side", t, func() {
		So(normalizeWsname("XBT/EUR"), ShouldEqual, "BTC/EUR")
		So(normalizeWsname("XDG/EUR"), ShouldEqual, "DOGE/EUR")
		So(normalizeWsname("EUR/XBT"), ShouldEqual, "EUR/BTC")
	})

	Convey("Modern names and odd shapes pass through untouched", t, func() {
		So(normalizeWsname("ETH/EUR"), ShouldEqual, "ETH/EUR")
		So(normalizeWsname("XBTEUR"), ShouldEqual, "XBTEUR") // no slash: not a wsname
		So(normalizeWsname(""), ShouldEqual, "")
	})
}
