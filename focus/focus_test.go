package focus

import (
	"sort"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
)

func TestNewSet(t *testing.T) {
	Convey("Given a new focus set", t, func() {
		set := NewSet()

		Convey("It should start empty", func() {
			So(set, ShouldNotBeNil)
			So(len(set.Snapshot()), ShouldEqual, 0)
			So(set.Has("BTC/EUR"), ShouldBeFalse)
		})
	})
}

func TestSetAdd(t *testing.T) {
	Convey("Given a focus set", t, func() {
		set := NewSet()

		Convey("It should report added symbols as present", func() {
			set.Add("BTC/EUR")
			set.Add("ETH/EUR")

			So(set.Has("BTC/EUR"), ShouldBeTrue)
			So(set.Has("ETH/EUR"), ShouldBeTrue)
			So(set.Has("SOL/EUR"), ShouldBeFalse)
		})

		Convey("It should be idempotent for repeated adds", func() {
			set.Add("BTC/EUR")
			set.Add("BTC/EUR")

			So(len(set.Snapshot()), ShouldEqual, 1)
		})
	})
}

func TestSetRemove(t *testing.T) {
	Convey("Given a focus set with two symbols", t, func() {
		set := NewSet()
		set.Add("BTC/EUR")
		set.Add("ETH/EUR")

		Convey("It should drop only the removed symbol", func() {
			set.Remove("BTC/EUR")

			So(set.Has("BTC/EUR"), ShouldBeFalse)
			So(set.Has("ETH/EUR"), ShouldBeTrue)
		})
	})
}

func TestAnchorSymbol(t *testing.T) {
	t.Cleanup(viper.Reset)

	Convey("Given market config", t, func() {
		Convey("It should prefer market.anchor_symbol", func() {
			viper.Set("market.anchor_symbol", "ETH/EUR")
			viper.Set("market.default_symbols", []string{"BTC/EUR"})

			So(AnchorSymbol(), ShouldEqual, "ETH/EUR")
		})

		Convey("It should fall back to the first default symbol", func() {
			viper.Set("market.anchor_symbol", "")
			viper.Set("market.default_symbols", []string{"BTC/EUR"})

			So(AnchorSymbol(), ShouldEqual, "BTC/EUR")
		})

		Convey("It should panic when anchor config is missing", func() {
			viper.Set("market.anchor_symbol", "")
			viper.Set("market.default_symbols", []string{})

			So(func() { AnchorSymbol() }, ShouldPanic)
		})
	})
}

func TestSetStreams(t *testing.T) {
	t.Cleanup(viper.Reset)

	Convey("Given a focus set with one open position", t, func() {
		viper.Set("market.anchor_symbol", "")
		viper.Set("market.default_symbols", []string{"BTC/EUR"})
		set := NewSet()
		set.Add("ALGO/EUR")

		Convey("It should always stream the anchor symbol", func() {
			So(set.Streams(AnchorSymbol()), ShouldBeTrue)
		})

		Convey("It should stream only the anchor and open positions", func() {
			So(set.Streams("ALGO/EUR"), ShouldBeTrue)
			So(set.Streams("ETH/EUR"), ShouldBeFalse)
		})
	})
}

func TestSetSnapshot(t *testing.T) {
	Convey("Given a focus set", t, func() {
		set := NewSet()
		set.Add("BTC/EUR")
		set.Add("ETH/EUR")

		Convey("It should return every focused symbol", func() {
			snapshot := set.Snapshot()
			sort.Strings(snapshot)

			So(snapshot, ShouldResemble, []string{"BTC/EUR", "ETH/EUR"})
		})
	})
}
