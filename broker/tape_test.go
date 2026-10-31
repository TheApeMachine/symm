package broker

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/system"
)

func TestTapeAliveRequiresWindowDensity(t *testing.T) {
	Convey("tape stays quiet until MinTapeTrades land inside the window", t, func() {
		tape := NewTape()
		now := time.Now().UTC()
		min := system.Cfg.Risk.MinTapeTrades

		So(tape.Alive("PEP/USD"), ShouldBeFalse)

		for i := 0; i < min-1; i++ {
			tape.Note([]kraken.TradeData{{
				Symbol:    "PEP/USD",
				Timestamp: now.Add(-time.Duration(i) * time.Second),
			}})
		}

		So(tape.Count("PEP/USD", now), ShouldEqual, min-1)
		So(tape.Alive("PEP/USD"), ShouldBeFalse)

		tape.Note([]kraken.TradeData{{
			Symbol:    "PEP/USD",
			Timestamp: now,
		}})

		So(tape.Alive("PEP/USD"), ShouldBeTrue)
	})

	Convey("stamps older than the window do not count", t, func() {
		tape := NewTape()
		now := time.Now().UTC()
		min := system.Cfg.Risk.MinTapeTrades
		window := system.Cfg.Risk.TapeWindow

		for i := 0; i < min; i++ {
			tape.Note([]kraken.TradeData{{
				Symbol:    "DEAD/USD",
				Timestamp: now.Add(-window - time.Minute),
			}})
		}

		So(tape.Count("DEAD/USD", now), ShouldEqual, 0)
		So(tape.Alive("DEAD/USD"), ShouldBeFalse)
	})
}
