package toxicity

import (
	"context"
	"fmt"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/market"
)

type toxicityFrame struct {
	role   string
	level3 kraken.Level3Data
	trade  kraken.TradeData
}

func TestSignalIngestRoles(t *testing.T) {
	Convey("Given a toxicity signal", t, func() {
		signal := NewSignal(context.Background())
		defer func() { _ = signal.Close() }()

		Convey("When ingest roles are requested", func() {
			roles := signal.IngestRoles()

			Convey("Then it should consume level3 and trade data", func() {
				So(roles, ShouldResemble, []string{"level3", "trade"})
			})
		})

		Convey("When L2 book data is measured", func() {
			measurements, err := signal.Measure(market.Input{Role: "book"}, nil)

			Convey("Then it should not fabricate toxicity", func() {
				So(err, ShouldBeNil)
				So(measurements, ShouldBeNil)
			})
		})
	})
}

func TestSignalMeasure(t *testing.T) {
	Convey("Given low-evidence level3 and trade rows", t, func() {
		signal := NewSignal(context.Background())
		defer func() { _ = signal.Close() }()

		frames := []toxicityFrame{
			level3Frame("BTC/USD", startAt(0),
				[]kraken.Level3Order{
					l3Order("", "B1", 100, 20),
					l3Order("", "B2", 99.9, 100),
				},
				[]kraken.Level3Order{l3Order("", "A1", 101, 100)},
			),
			tradeFrame("BTC/USD", "buy", 100, 20, startAt(1)),
		}

		result := replay(t, signal, frames)

		Convey("When toxicity is measured", func() {
			Convey("Then no category should be emitted yet", func() {
				So(result, ShouldBeNil)
			})
		})
	})
}

func TestSignalMeasureCategorySemantics(t *testing.T) {
	Convey("Given a near-touch L3 block that disappears without trade", t, func() {
		signal := NewSignal(context.Background())
		defer func() { _ = signal.Close() }()

		frames := []toxicityFrame{
			level3Frame("BTC/USD", startAt(0),
				[]kraken.Level3Order{
					l3Order("", "B1", 100, 100),
					l3Order("", "B2", 100, 20),
				},
				[]kraken.Level3Order{l3Order("", "A1", 101, 100)},
			),
			level3Frame("BTC/USD", startAt(1),
				[]kraken.Level3Order{l3Order("delete", "B1", 100, 100)},
				nil,
			),
		}

		result := replay(t, signal, frames)

		Convey("When toxicity is measured", func() {
			Convey("Then toxic bluff should dominate", func() {
				So(result, ShouldNotBeNil)
				So(result.DominantCategory(), ShouldEqual, logic.CategoryToxicBluff)
				So(result.Metric("bluffScore"), ShouldBeGreaterThan, 0)
				So(result.Metric("l3"), ShouldEqual, 1)
			})
		})
	})

	Convey("Given one side retreating much faster than it fills", t, func() {
		signal := NewSignal(context.Background())
		defer func() { _ = signal.Close() }()

		frames := make([]toxicityFrame, 0)
		for index := range 3 {
			frames = appendVacuumWarmup(frames, index)
		}

		frames = append(frames,
			level3Frame("BTC/USD", startAt(20),
				[]kraken.Level3Order{
					l3Order("add", "BF-final", 100, 10),
					l3Order("add", "BC-final", 99.9, 100),
				},
				[]kraken.Level3Order{l3Order("add", "A-final", 101, 100)},
			),
			tradeFrame("BTC/USD", "buy", 100, 10, startAt(21)),
			level3Frame("BTC/USD", startAt(22),
				[]kraken.Level3Order{
					l3Order("delete", "BF-final", 100, 10),
					l3Order("delete", "BC-final", 99.9, 100),
				},
				nil,
			),
		)

		result := replay(t, signal, frames)

		Convey("When toxicity is measured", func() {
			Convey("Then liquidity vacuum should dominate", func() {
				So(result, ShouldNotBeNil)
				So(result.DominantCategory(), ShouldEqual, logic.CategoryLiquidityVacuum)
				So(result.Metric("vacuumScore"), ShouldBeGreaterThan, 0)
				So(result.Metric("l3"), ShouldEqual, 1)
			})
		})
	})

	Convey("Given touch liquidity that fills without cancels", t, func() {
		signal := NewSignal(context.Background())
		defer func() { _ = signal.Close() }()

		frames := []toxicityFrame{
			level3Frame("BTC/USD", startAt(0),
				[]kraken.Level3Order{
					l3Order("", "B1", 100, 20),
					l3Order("", "B2", 99.9, 100),
				},
				[]kraken.Level3Order{l3Order("", "A1", 101, 100)},
			),
			tradeFrame("BTC/USD", "buy", 100, 20, startAt(1)),
			level3Frame("BTC/USD", startAt(2),
				[]kraken.Level3Order{l3Order("delete", "B1", 100, 20)},
				nil,
			),
		}

		result := replay(t, signal, frames)

		Convey("When toxicity is measured", func() {
			Convey("Then hard support should dominate", func() {
				So(result, ShouldNotBeNil)
				So(result.DominantCategory(), ShouldEqual, logic.CategoryHardSupport)
				So(result.Metric("supportScore"), ShouldBeGreaterThan, 0)
				So(result.Metric("l3"), ShouldEqual, 1)
			})
		})
	})
}

func BenchmarkSignalMeasure(b *testing.B) {
	frames := []toxicityFrame{
		level3Frame("BTC/USD", startAt(0),
			[]kraken.Level3Order{
				l3Order("", "B1", 100, 100),
				l3Order("", "B2", 100, 20),
			},
			[]kraken.Level3Order{l3Order("", "A1", 101, 100)},
		),
		level3Frame("BTC/USD", startAt(1),
			[]kraken.Level3Order{l3Order("delete", "B1", 100, 100)},
			nil,
		),
	}

	b.ReportAllocs()

	for b.Loop() {
		signal := NewSignal(context.Background())
		result := replay(b, signal, frames)

		if result == nil {
			b.Fatal("Measure returned nil")
		}

		_ = signal.Close()
	}
}

func replay(
	t testing.TB,
	signal *Signal,
	frames []toxicityFrame,
) *logic.Measurement {
	t.Helper()

	var result *logic.Measurement
	for _, frame := range frames {
		measurements, err := signal.Measure(frame.input(), nil)
		if err != nil {
			t.Fatal(err)
		}

		for _, measurement := range measurements {
			if measurement.Confidence <= 0 || measurement.Strength <= 0 {
				continue
			}

			result = measurement
		}
	}

	return result
}

func appendVacuumWarmup(frames []toxicityFrame, index int) []toxicityFrame {
	symbol := "BTC/USD"
	offset := index * 3

	return append(frames,
		level3Frame(symbol, startAt(offset),
			[]kraken.Level3Order{
				l3Order("add", fmt.Sprintf("BF-%d", index), 100, 100),
				l3Order("add", fmt.Sprintf("BC-%d", index), 99.9, 10),
			},
			[]kraken.Level3Order{l3Order("add", fmt.Sprintf("A-%d", index), 101, 100)},
		),
		tradeFrame(symbol, "buy", 100, 100, startAt(offset+1)),
		level3Frame(symbol, startAt(offset+2),
			[]kraken.Level3Order{
				l3Order("delete", fmt.Sprintf("BF-%d", index), 100, 100),
				l3Order("delete", fmt.Sprintf("BC-%d", index), 99.9, 10),
			},
			nil,
		),
	)
}

func (frame toxicityFrame) input() market.Input {
	if frame.role == "level3" {
		return market.Input{
			Role:   "level3",
			At:     frame.level3.Timestamp,
			Level3: kraken.Level3DataSlice{frame.level3},
		}
	}

	return market.Input{
		Role:  "trade",
		At:    frame.trade.Timestamp,
		Trade: kraken.TradeDataSlice{frame.trade},
	}
}

func level3Frame(
	symbol string,
	timestamp time.Time,
	bids []kraken.Level3Order,
	asks []kraken.Level3Order,
) toxicityFrame {
	return toxicityFrame{
		role: "level3",
		level3: kraken.Level3Data{
			Symbol:    symbol,
			Timestamp: timestamp,
			Bids:      bids,
			Asks:      asks,
		},
	}
}

func tradeFrame(
	symbol string,
	side string,
	price float64,
	quantity float64,
	timestamp time.Time,
) toxicityFrame {
	return toxicityFrame{
		role: "trade",
		trade: kraken.TradeData{
			Symbol:    symbol,
			Side:      side,
			Price:     price,
			Qty:       quantity,
			Timestamp: timestamp,
		},
	}
}

func l3Order(
	event string,
	orderID string,
	price float64,
	quantity float64,
) kraken.Level3Order {
	return kraken.Level3Order{
		Event:      event,
		OrderID:    orderID,
		LimitPrice: price,
		OrderQty:   quantity,
		Timestamp:  startAt(0),
	}
}

func startAt(seconds int) time.Time {
	return time.Date(2026, 5, 30, 12, 0, seconds, 0, time.UTC)
}
