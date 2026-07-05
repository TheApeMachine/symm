package resonance

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/market"
)

func TestSignalMeasure(t *testing.T) {
	observedAt := startAt(0)

	Convey("Given laminar market rows", t, func() {
		signal := NewSignal(context.Background(), nil, 0.02, 8)
		defer func() { _ = signal.Close() }()

		scope := "FLOW/EUR"
		seedBook(t, signal, scope, 1, 0.001, observedAt)
		measurements := measureTicker(t, signal, scope, 1, 1, -2, observedAt)

		Convey("When resonance is measured", func() {
			Convey("Then it should classify laminar resonance", func() {
				So(measurements, ShouldHaveLength, 1)
				assertResonanceMeasurement(measurements[0], scope)
				So(measurements[0].DominantCategory(), ShouldEqual, logic.CategoryType(CategoryFlow))
			})
		})
	})

	Convey("Given equilibrium market rows", t, func() {
		signal := NewSignal(context.Background(), nil, 0.02, 8)
		defer func() { _ = signal.Close() }()

		scope := "COUPLE/EUR"
		seedBook(t, signal, scope, 1, 2.001, observedAt)
		measurements := measureTicker(t, signal, scope, 1, 1, -2, observedAt)

		Convey("When resonance is measured", func() {
			Convey("Then it should classify equilibrium coupling", func() {
				So(measurements, ShouldHaveLength, 1)
				assertResonanceMeasurement(measurements[0], scope)
				So(measurements[0].DominantCategory(), ShouldEqual, logic.CategoryType(CategoryCoupling))
			})
		})
	})

	Convey("Given sparse market rows", t, func() {
		signal := NewSignal(context.Background(), nil, 0.02, 8)
		defer func() { _ = signal.Close() }()

		results, err := signal.SettleScopes([]string{"NEW/EUR"})

		Convey("When resonance is settled", func() {
			Convey("Then it should abstain", func() {
				So(err, ShouldBeNil)
				So(results, ShouldBeEmpty)
			})
		})
	})
}

func TestSignalMeasureCategorySemantics(t *testing.T) {
	observedAt := startAt(0)

	Convey("Given laminar market rows", t, func() {
		signal := NewSignal(context.Background(), nil, 0.02, 8)
		defer func() { _ = signal.Close() }()

		scope := "FLOW/EUR"
		seedBook(t, signal, scope, 1, 0.001, observedAt)
		measurements := measureTicker(t, signal, scope, 1, 1, -2, observedAt)

		Convey("When resonance is measured", func() {
			Convey("Then laminar resonance should dominate", func() {
				So(measurements, ShouldHaveLength, 1)
				So(measurements[0].DominantCategory(), ShouldEqual, logic.CategoryType(CategoryFlow))
				So(measurements[0].CategoryMass(logic.CategoryType(CategoryFlow)), ShouldBeGreaterThan, 0)
			})
		})
	})

	Convey("Given invalid spread market rows", t, func() {
		signal := NewSignal(context.Background(), nil, 0.02, 8)
		defer func() { _ = signal.Close() }()

		scope := "COUPLE/EUR"
		seedBook(t, signal, scope, 1, 2.001, observedAt)
		measurements := measureTicker(t, signal, scope, 1, 1, -2, observedAt)

		Convey("When resonance is measured", func() {
			Convey("Then equilibrium coupling should dominate", func() {
				So(measurements, ShouldHaveLength, 1)
				So(measurements[0].DominantCategory(), ShouldEqual, logic.CategoryType(CategoryCoupling))
				So(measurements[0].CategoryMass(logic.CategoryType(CategoryCoupling)), ShouldEqual, 1)
			})
		})
	})
}

func BenchmarkSignalMeasure(b *testing.B) {
	observedAt := startAt(0)

	b.ReportAllocs()

	for b.Loop() {
		signal := NewSignal(context.Background(), nil, 0.02, 8)
		seedBook(b, signal, "FLOW/EUR", 1, 0.001, observedAt)
		measurements := measureTicker(b, signal, "FLOW/EUR", 1, 1, -2, observedAt)

		if len(measurements) == 0 {
			b.Fatal("Measure returned no resonance measurement")
		}

		_ = signal.Close()
	}
}

func seedMarket(
	t testing.TB,
	signal *Signal,
	scope string,
	last float64,
	volume float64,
	changePct float64,
	spreadRatio float64,
	observedAt time.Time,
) {
	t.Helper()

	seedBook(t, signal, scope, last, spreadRatio, observedAt)
	seedTicker(t, signal, scope, last, volume, changePct, observedAt)
}

func seedTicker(
	t testing.TB,
	signal *Signal,
	scope string,
	last float64,
	volume float64,
	changePct float64,
	observedAt time.Time,
) {
	t.Helper()

	if err := signal.observeTickers(kraken.TickerDataSlice{
		tickerRow(scope, last, volume, changePct, observedAt),
	}); err != nil {
		t.Fatal(err)
	}
}

func seedBook(
	t testing.TB,
	signal *Signal,
	scope string,
	last float64,
	spreadRatio float64,
	observedAt time.Time,
) {
	t.Helper()

	if err := signal.observeBooks(kraken.BookDataSlice{
		bookRow(scope, last, spreadRatio, observedAt),
	}); err != nil {
		t.Fatal(err)
	}
}

func measureTicker(
	t testing.TB,
	signal *Signal,
	scope string,
	last float64,
	volume float64,
	changePct float64,
	observedAt time.Time,
) []*logic.Measurement {
	t.Helper()

	measurements, err := signal.Measure(market.Input{
		Role:   "ticker",
		At:     observedAt,
		Ticker: kraken.TickerDataSlice{tickerRow(scope, last, volume, changePct, observedAt)},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}

	return measurements
}

func settledMeasurement(
	t testing.TB,
	signal *Signal,
	scope string,
) *logic.Measurement {
	t.Helper()

	results, err := signal.SettleScopes([]string{scope})
	if err != nil {
		t.Fatal(err)
	}

	return results[scope]
}

func tickerRow(
	scope string,
	last float64,
	volume float64,
	changePct float64,
	observedAt time.Time,
) kraken.TickerData {
	return kraken.TickerData{
		Symbol:    scope,
		Last:      last,
		Volume:    volume,
		ChangePct: changePct,
		Timestamp: observedAt,
	}
}

func bookRow(
	scope string,
	last float64,
	spreadRatio float64,
	observedAt time.Time,
) kraken.BookData {
	bid := last * (1 - spreadRatio/2)
	ask := last * (1 + spreadRatio/2)

	return kraken.BookData{
		Symbol:    scope,
		Timestamp: observedAt,
		Bids: []kraken.BookLevel{
			{Price: bid, Qty: 1},
		},
		Asks: []kraken.BookLevel{
			{Price: ask, Qty: 1},
		},
	}
}

func assertResonanceMeasurement(measurement *logic.Measurement, scope string) {
	So(measurement.Source, ShouldEqual, logic.SourceResonance)
	So(measurement.Symbol, ShouldEqual, scope)
	So(measurement.At.IsZero(), ShouldBeFalse)
	So(measurement.Confidence, ShouldBeGreaterThan, 0)
	So(measurement.EntryBaseline, ShouldBeGreaterThan, 0)
	So(measurement.ExitBaseline, ShouldBeGreaterThan, 0)
	So(measurement.EntryBaseline, ShouldBeGreaterThanOrEqualTo, measurement.ExitBaseline)
	So(measurement.Metric("price"), ShouldBeGreaterThan, 0)
	So(measurement.HasDistribution(), ShouldBeTrue)
}

func startAt(seconds int) time.Time {
	return time.Date(2024, 1, 1, 0, 0, seconds, 0, time.UTC)
}
