package pumpdump

import (
	"context"
	"fmt"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/qpool"
)

func newTestPool(t testing.TB) *qpool.Q[any] {
	if t != nil {
		t.Helper()
	}

	pool := qpool.NewQ[any](context.Background(), 2, 4, nil)

	if pool == nil && t != nil {
		t.Fatal("qpool.NewQ returned nil")
	}

	return pool
}

func tickerQuery(scope string) *datura.Artifact {
	acquired := datura.Acquire("kraken:public", datura.APPJSON)
	acquired.WithRole("ticker")
	acquired.WithScope(scope)

	return acquired
}

func krakenTickerFrame(
	volume, vwap, last, bid, ask, changePct float64,
	scope string,
) []byte {
	return fmt.Appendf(nil, 
		`{"channel":"ticker","type":"update","data":[{"symbol":%q,"bid":%g,"bid_qty":740.0,"ask":%g,"ask_qty":740.0,"last":%g,"volume":%g,"vwap":%g,"change_pct":%g}]}`,
		scope, bid, ask, last, volume, vwap, changePct,
	)
}

func insertTickerReplay(
	signal *Signal,
	query *datura.Artifact,
	volume, vwap, last, bid, ask, changePct float64,
) {
	scope, scopeErr := query.Scope()
	role, roleErr := query.Role()

	if scopeErr != nil || roleErr != nil || scope == "" || role == "" {
		return
	}

	replay := datura.Acquire("kraken:public", datura.APPJSON)
	replay.WithRole(role)
	replay.WithScope(scope)
	replay.WithPayload(krakenTickerFrame(volume, vwap, last, bid, ask, changePct, scope))

	wire, err := replay.Message().Marshal()

	if err != nil || len(wire) == 0 {
		replay.Release()
		return
	}

	signal.tree.Insert(query.Prefix(), wire)
	replay.Release()
}

func verticalIgnitionTicker() (float64, float64, float64, float64, float64, float64) {
	return 11000, 10000, 41000, 40990, 41010, 3.1
}

func coiledCompressionTicker() (float64, float64, float64, float64, float64, float64) {
	return 15000, 10000, 10050, 10049.999, 10050.001, 0.5
}

func organicTrendTicker() (float64, float64, float64, float64, float64, float64) {
	return 10100, 10000, 15000, 10000, 30000, 1.0
}

func fadedExhaustionTicker() (float64, float64, float64, float64, float64, float64) {
	return 5000, 10000, 10100, 10095, 10105, 0.1
}

func TestSignalMeasure(t *testing.T) {
	Convey("Given a vertical ignition ticker update", t, func() {
		signal := NewSignal(context.Background(), newTestPool(t), dmt.NewTree(""))
		So(signal, ShouldNotBeNil)

		defer func() {
			_ = signal.Close()
		}()

		query := tickerQuery("ETH/EUR")

		defer query.Release()

		volume, vwap, last, bid, ask, changePct := verticalIgnitionTicker()
		insertTickerReplay(signal, query, volume, vwap, last, bid, ask, changePct)

		result := signal.Measure(query)

		Convey("It should classify vertical ignition from the ticker replay", func() {
			So(result, ShouldNotBeNil)
			So(datura.Peek[int](result, "classifier", "category"), ShouldEqual, 1)
			So(datura.Peek[float64](result, "classifier", "confidence"), ShouldBeGreaterThan, 0)
		})
	})

	Convey("Given spread compression with low precursor", t, func() {
		signal := NewSignal(context.Background(), newTestPool(t), dmt.NewTree(""))
		So(signal, ShouldNotBeNil)

		defer func() {
			_ = signal.Close()
		}()

		query := tickerQuery("BTC/EUR")

		defer query.Release()

		volume, vwap, last, bid, ask, changePct := coiledCompressionTicker()
		insertTickerReplay(signal, query, volume, vwap, last, bid, ask, changePct)

		result := signal.Measure(query)

		Convey("It should classify coiled compression from the ticker replay", func() {
			So(result, ShouldNotBeNil)
			So(datura.Peek[int](result, "classifier", "category"), ShouldEqual, 2)
			So(datura.Peek[float64](result, "classifier", "confidence"), ShouldBeGreaterThan, 0)
		})
	})

	Convey("Given steady momentum without vertical lift", t, func() {
		signal := NewSignal(context.Background(), newTestPool(t), dmt.NewTree(""))
		So(signal, ShouldNotBeNil)

		defer func() {
			_ = signal.Close()
		}()

		query := tickerQuery("TREND/EUR")

		defer query.Release()

		volume, vwap, last, bid, ask, changePct := organicTrendTicker()
		insertTickerReplay(signal, query, volume, vwap, last, bid, ask, changePct)

		result := signal.Measure(query)

		Convey("It should classify organic trend from the ticker replay", func() {
			So(result, ShouldNotBeNil)
			So(datura.Peek[int](result, "classifier", "category"), ShouldEqual, 3)
			So(datura.Peek[float64](result, "classifier", "confidence"), ShouldBeGreaterThan, 0)
		})
	})

	Convey("Given fading volume lift with flat precursor", t, func() {
		signal := NewSignal(context.Background(), newTestPool(t), dmt.NewTree(""))
		So(signal, ShouldNotBeNil)

		defer func() {
			_ = signal.Close()
		}()

		query := tickerQuery("FADE/EUR")

		defer query.Release()

		volume, vwap, last, bid, ask, changePct := fadedExhaustionTicker()
		insertTickerReplay(signal, query, volume, vwap, last, bid, ask, changePct)

		result := signal.Measure(query)

		Convey("It should classify faded exhaustion from the ticker replay", func() {
			So(result, ShouldNotBeNil)
			So(datura.Peek[int](result, "classifier", "category"), ShouldEqual, 4)
			So(datura.Peek[float64](result, "classifier", "confidence"), ShouldBeGreaterThan, 0)
		})
	})

	Convey("Given a sparse tree at startup", t, func() {
		signal := NewSignal(context.Background(), newTestPool(t), dmt.NewTree(""))
		So(signal, ShouldNotBeNil)

		defer func() {
			_ = signal.Close()
		}()

		query := tickerQuery("NEW/EUR")

		defer query.Release()

		result := signal.Measure(query)

		Convey("It should leave the query unclassified without ticker rows", func() {
			So(result, ShouldNotBeNil)
			So(datura.Peek[int](result, "classifier", "category"), ShouldEqual, 0)
			So(datura.Peek[float64](result, "classifier", "confidence"), ShouldEqual, 0)
		})
	})
}

func BenchmarkSignalMeasure(b *testing.B) {
	query := tickerQuery("BTC/EUR")
	volume, vwap, last, bid, ask, changePct := coiledCompressionTicker()

	b.ReportAllocs()

	for b.Loop() {
		signal := NewSignal(context.Background(), newTestPool(b), dmt.NewTree(""))

		if signal == nil {
			b.Fatal("NewSignal returned nil")
		}

		insertTickerReplay(signal, query, volume, vwap, last, bid, ask, changePct)

		result := signal.Measure(query)

		if result == nil {
			b.Fatal("Measure returned nil")
		}

		if datura.Peek[int](result, "classifier", "category") <= 0 {
			b.Fatal("Measure did not classify coiled compression")
		}

		_ = signal.Close()
	}

	query.Release()
}
