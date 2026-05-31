package trader

import (
	"testing"

	"github.com/smartystreets/goconvey/convey"
)

func TestOrderSessionTrackExit(t *testing.T) {
	convey.Convey("Given an exit intent already pending for a symbol", t, func() {
		session := &orderSession{}
		first := session.trackExit(
			"exit-1",
			"ALGO/EUR",
			orderIntent{kind: "exit", symbol: "ALGO/EUR"},
		)
		second := session.trackExit(
			"exit-2",
			"ALGO/EUR",
			orderIntent{kind: "exit", symbol: "ALGO/EUR"},
		)

		convey.Convey("It should reject a second pending exit", func() {
			_, firstFound := session.intentFor("exit-1")
			_, secondFound := session.intentFor("exit-2")

			convey.So(first, convey.ShouldBeTrue)
			convey.So(second, convey.ShouldBeFalse)
			convey.So(firstFound, convey.ShouldBeTrue)
			convey.So(secondFound, convey.ShouldBeFalse)
			convey.So(session.HasPendingExit("ALGO/EUR"), convey.ShouldBeTrue)
		})

		convey.Convey("It should clear the pending exit only when that intent drops", func() {
			session.dropIntent("exit-2", "ALGO/EUR")
			convey.So(session.HasPendingExit("ALGO/EUR"), convey.ShouldBeTrue)

			session.dropIntent("exit-1", "ALGO/EUR")
			convey.So(session.HasPendingExit("ALGO/EUR"), convey.ShouldBeFalse)
		})
	})
}

func TestOrderSessionDropIntentPreservesNewerEntry(t *testing.T) {
	convey.Convey("Given a newer entry intent replaces an older symbol slot", t, func() {
		session := &orderSession{}
		session.trackEntry("entry-1", "BTC/EUR", orderIntent{kind: "entry"})
		session.trackEntry("entry-2", "BTC/EUR", orderIntent{kind: "entry"})

		convey.Convey("It should not clear the newer pending entry when the old intent drops", func() {
			session.dropIntent("entry-1", "BTC/EUR")
			convey.So(session.HasPendingEntry("BTC/EUR"), convey.ShouldBeTrue)

			session.dropIntent("entry-2", "BTC/EUR")
			convey.So(session.HasPendingEntry("BTC/EUR"), convey.ShouldBeFalse)
		})
	})
}

func BenchmarkOrderSessionHasPendingExit(b *testing.B) {
	session := &orderSession{}
	session.trackExit(
		"exit-1",
		"ALGO/EUR",
		orderIntent{kind: "exit", symbol: "ALGO/EUR"},
	)

	b.ResetTimer()

	for b.Loop() {
		_ = session.HasPendingExit("ALGO/EUR")
	}
}
