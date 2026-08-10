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
			So(symbol.Status, ShouldEqual, Status(""))
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
			So(symbol.Status, ShouldEqual, BUSY)
			So(symbol.Measurements, ShouldResemble, []*Measurement{updated, peer})
		})
	})
}

func TestSymbolStamp(t *testing.T) {
	Convey("Given both direct consumers of an active measurement cut", t, func() {
		symbol := NewSymbol("BTC/USD", nil)
		measurement := &Measurement{
			ID: "hawkes", Source: SourceHawkes, Symbol: "BTC/USD",
		}
		symbol.AddMeasurement(measurement)

		symbol.Stamp(SourceCategory)

		Convey("It should retain the cut until resonance also finishes", func() {
			So(symbol.Status, ShouldEqual, BUSY)
			So(symbol.Measurements, ShouldResemble, []*Measurement{measurement})
		})

		symbol.Stamp(SourceResonance)

		Convey("It should retain the cut until every named consumer finishes", func() {
			So(symbol.Status, ShouldEqual, BUSY)
			So(symbol.Measurements, ShouldResemble, []*Measurement{measurement})
		})

		Convey("It should retain the completed readiness stamps", func() {
			So(symbol.Stamped(SourceCategory), ShouldBeTrue)
			So(symbol.Stamped(SourceResonance), ShouldBeTrue)
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
