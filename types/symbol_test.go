package types

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestSymbolNewSymbol(t *testing.T) {
	Convey("Given a symbol name", t, func() {
		symbol := NewSymbol("BTC/USD", nil)

		Convey("It should initialize empty symbol measurement state", func() {
			So(symbol.Symbol, ShouldEqual, "BTC/USD")
			So(symbol.Status, ShouldEqual, READY)
			So(symbol.Measurements, ShouldBeEmpty)
		})
	})
}

func TestSymbolAddMeasurement(t *testing.T) {
	Convey("Given measurements with one matching source identity", t, func() {
		symbol := NewSymbol("BTC/USD", nil)
		first := &Measurement{ID: "first", Source: SourceHawkes, Symbol: "BTC/USD"}
		updated := &Measurement{ID: "updated", Source: SourceHawkes, Symbol: "BTC/USD"}
		peer := &Measurement{
			ID: "peer", Source: SourceHawkes, Symbol: "BTC/USD", Peer: "ETH/USD",
		}

		symbol.AddMeasurement(first)
		symbol.AddMeasurement(peer)
		symbol.AddMeasurement(updated)

		Convey("It should replace only the matching source and peer", func() {
			So(symbol.Status, ShouldEqual, READY)
			So(symbol.Measurements, ShouldResemble, []*Measurement{updated, peer})
		})
	})
}

func BenchmarkSymbolAddMeasurement(b *testing.B) {
	measurement := &Measurement{
		ID: "hawkes", Source: SourceHawkes, Symbol: "BTC/USD",
	}
	b.ReportAllocs()

	for b.Loop() {
		symbol := NewSymbol("BTC/USD", nil)
		symbol.AddMeasurement(measurement)
	}
}
