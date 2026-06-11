package market

import (
	"math"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	market "github.com/theapemachine/symm/kraken/market"
)

func validCrossSectionConfig() *CrossSectionConfig {
	return &CrossSectionConfig{
		MatchWindow: time.Minute,
		ReturnCap:   64,
		MinBars:     8,
		BreadthHist: 64,
	}
}

func TestNewCrossSection(t *testing.T) {
	Convey("Given cross section config", t, func() {
		crossSection, err := NewCrossSection(validCrossSectionConfig())

		Convey("It should construct from config", func() {
			So(err, ShouldBeNil)
			So(crossSection, ShouldNotBeNil)
			So(crossSection.MinBarsRequired(), ShouldEqual, 8)
		})
	})

	Convey("Given invalid return_capacity", t, func() {
		cfg := validCrossSectionConfig()
		cfg.ReturnCap = 0

		_, err := NewCrossSection(cfg)

		Convey("It should return an error", func() {
			So(err, ShouldNotBeNil)
		})
	})
}

func TestCrossSectionObserve(t *testing.T) {
	Convey("Given a cross section with return capacity 4", t, func() {
		crossSection := &CrossSection{returnCap: 4, matchWindow: time.Minute}
		eventAt := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

		firstRow, err := market.NewSymbolRow("BTC/EUR", 100, 1, 1000, 1, eventAt)
		So(err, ShouldBeNil)
		So(crossSection.Observe(firstRow), ShouldBeNil)

		secondRow, err := market.NewSymbolRow("BTC/EUR", 110, 1, 1100, 1, eventAt.Add(time.Second))
		So(err, ShouldBeNil)
		So(crossSection.Observe(secondRow), ShouldBeNil)

		Convey("It should append log returns before updating price", func() {
			returns := crossSection.SymbolReturns("BTC/EUR", 1)
			So(len(returns), ShouldEqual, 1)
			So(returns[0], ShouldAlmostEqual, math.Log(1.1), 1e-12)
		})
	})
}

func TestCrossSectionMacroMomentum(t *testing.T) {
	Convey("Given peer changes excluding self", t, func() {
		crossSection := &CrossSection{}
		crossSection.universe.Store("SELF/EUR", &market.Symbol{Name: "SELF/EUR", Value: 99})
		crossSection.universe.Store("A/EUR", &market.Symbol{Name: "A/EUR", Value: 1})
		crossSection.universe.Store("B/EUR", &market.Symbol{Name: "B/EUR", Value: 3})

		Convey("It should return the median peer change", func() {
			So(crossSection.MacroMomentum("SELF/EUR"), ShouldEqual, 2)
		})
	})
}

func TestCrossSectionBreadthStaleness(t *testing.T) {
	Convey("Given one fresh uptick and one stale uptick", t, func() {
		crossSection := &CrossSection{matchWindow: time.Minute}
		eventAt := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

		crossSection.universe.Store("FRESH/EUR", &market.Symbol{
			Name: "FRESH/EUR", Value: 2, Updated: eventAt,
		})
		crossSection.universe.Store("STALE/EUR", &market.Symbol{
			Name: "STALE/EUR", Value: 3, Updated: eventAt.Add(-5 * time.Minute),
		})

		Convey("It should ignore stale symbols when computing breadth", func() {
			So(crossSection.Breadth(eventAt), ShouldEqual, 1)
		})
	})

	Convey("Given a future-dated symbol row", t, func() {
		crossSection := &CrossSection{matchWindow: time.Minute}
		eventAt := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

		crossSection.universe.Store("FUTURE/EUR", &market.Symbol{
			Name: "FUTURE/EUR", Value: 2, Updated: eventAt.Add(time.Hour),
		})
		crossSection.universe.Store("FRESH/EUR", &market.Symbol{
			Name: "FRESH/EUR", Value: 2, Updated: eventAt,
		})

		Convey("It should ignore future-dated rows and stay finite", func() {
			breadth := crossSection.Breadth(eventAt)
			So(math.IsNaN(breadth), ShouldBeFalse)
			So(math.IsInf(breadth, 0), ShouldBeFalse)
			So(breadth, ShouldEqual, 1)
		})
	})
}

func TestCrossSectionVolumes(t *testing.T) {
	Convey("Given symbols with volume set", t, func() {
		crossSection := &CrossSection{}
		crossSection.universe.Store("A/EUR", &market.Symbol{Name: "A/EUR", Volume: 10})
		crossSection.universe.Store("B/EUR", &market.Symbol{Name: "B/EUR", Volume: 20})
		crossSection.universe.Store("C/EUR", &market.Symbol{Name: "C/EUR"})

		Convey("It should return populated volumes", func() {
			volumes := crossSection.Volumes()
			So(len(volumes), ShouldEqual, 2)
			So(volumes, ShouldContain, 10.0)
			So(volumes, ShouldContain, 20.0)
		})
	})
}

func TestCrossSectionPressure(t *testing.T) {
	Convey("Given a symbol with pressure set", t, func() {
		crossSection := &CrossSection{}
		crossSection.universe.Store("BTC/EUR", &market.Symbol{Name: "BTC/EUR", Pressure: 0.75})

		Convey("It should return stored pressure", func() {
			pressure, err := crossSection.Pressure("BTC/EUR")
			So(err, ShouldBeNil)
			So(pressure, ShouldEqual, 0.75)
		})

		Convey("It should error when the symbol is missing", func() {
			_, err := crossSection.Pressure("ETH/EUR")
			So(err, ShouldNotBeNil)
		})

		Convey("TradePressure should return zero when the symbol is missing", func() {
			So(crossSection.TradePressure("ETH/EUR"), ShouldEqual, 0)
		})
	})
}

func TestCrossSectionTrailingSymbolReturns(t *testing.T) {
	Convey("Given a symbol with fewer returns than the requested window", t, func() {
		crossSection := &CrossSection{returnCap: 64}
		crossSection.universe.Store("BTC/EUR", &market.Symbol{
			Name: "BTC/EUR",
			Returns: []float64{0.01, -0.008, 0.012, 0.005, 0.003, 0.002, 0.004, 0.001,
				0.006, 0.002, -0.001, 0.003, 0.002, 0.004, 0.001, 0.002},
		})

		Convey("It should return the available trailing window", func() {
			returns := crossSection.trailingSymbolReturns("BTC/EUR", 256)

			So(len(returns), ShouldEqual, 16)
			So(returns[0], ShouldAlmostEqual, 0.01, 1e-12)
			So(returns[len(returns)-1], ShouldAlmostEqual, 0.002, 1e-12)
		})
	})
}

func BenchmarkCrossSectionObserve(b *testing.B) {
	crossSection := &CrossSection{returnCap: 64, matchWindow: time.Minute}
	eventAt := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	price := 100.0

	for b.Loop() {
		price *= 1.0001
		crossSection.Observe(&market.Symbol{Name: "BTC/EUR", Price: price, Updated: eventAt})
	}
}
