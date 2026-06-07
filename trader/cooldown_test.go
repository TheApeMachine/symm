package trader

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestExitCooldownShorterThanEntry(t *testing.T) {
	Convey("Given a symbol rejected 2 seconds ago", t, func() {
		crypto := &Crypto{}
		crypto.cooldownStart.Store("BTC/EUR", time.Now().Add(-2*time.Second))

		Convey("Entries are still cooling (10s horizon)", func() {
			So(crypto.symbolCooling("BTC/EUR", false), ShouldBeTrue)
		})

		Convey("Exits already cleared their 1s horizon — the brakes work", func() {
			So(crypto.symbolCooling("BTC/EUR", true), ShouldBeFalse)
		})

		Convey("An exit probe does not erase the entry cooldown still in force", func() {
			crypto.symbolCooling("BTC/EUR", true)
			So(crypto.symbolCooling("BTC/EUR", false), ShouldBeTrue)
		})
	})

	Convey("Given a symbol rejected just now", t, func() {
		crypto := &Crypto{}
		crypto.coolSymbol("ETH/EUR")

		Convey("Both entries and exits are briefly cooling", func() {
			So(crypto.symbolCooling("ETH/EUR", false), ShouldBeTrue)
			So(crypto.symbolCooling("ETH/EUR", true), ShouldBeTrue)
		})
	})

	Convey("Given a symbol past every horizon", t, func() {
		crypto := &Crypto{}
		crypto.cooldownStart.Store("XRP/EUR", time.Now().Add(-11*time.Second))

		Convey("Nothing cools and the record is cleared", func() {
			So(crypto.symbolCooling("XRP/EUR", false), ShouldBeFalse)

			_, still := crypto.cooldownStart.Load("XRP/EUR")
			So(still, ShouldBeFalse)
		})
	})
}
