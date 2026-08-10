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
			So(symbol.Measurements, ShouldResemble, []*Measurement{updated, peer})
		})
	})
}

func TestSymbolMeasurementsSnapshot(t *testing.T) {
	Convey("Given a logic reader holding a symbol snapshot", t, func() {
		symbol := NewSymbol("BTC/USD", nil)
		first := &Measurement{ID: "first", Source: SourceHawkes, Symbol: "BTC/USD"}
		updated := &Measurement{ID: "updated", Source: SourceHawkes, Symbol: "BTC/USD"}
		symbol.AddMeasurement(first)
		snapshot := symbol.MeasurementsSnapshot()

		symbol.AddMeasurement(updated)

		Convey("It should keep the in-flight logic slice stable", func() {
			So(snapshot, ShouldResemble, []*Measurement{first})
			So(symbol.MeasurementsSnapshot(), ShouldResemble, []*Measurement{updated})
		})
	})
}

func BenchmarkSymbolAddMeasurement(b *testing.B) {
	symbol := NewSymbol("BTC/USD", nil)
	measurement := &Measurement{
		ID: "hawkes", Source: SourceHawkes, Symbol: "BTC/USD",
	}
	symbol.AddMeasurement(measurement)
	b.ReportAllocs()

	for b.Loop() {
		symbol.AddMeasurement(measurement)
	}
}

func BenchmarkSymbolMeasurementsSnapshot(b *testing.B) {
	symbol := NewSymbol("BTC/USD", nil)
	symbol.AddMeasurement(&Measurement{
		ID: "hawkes", Source: SourceHawkes, Symbol: "BTC/USD",
	})
	b.ReportAllocs()

	for b.Loop() {
		_ = symbol.MeasurementsSnapshot()
	}
}
