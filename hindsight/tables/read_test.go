package tables_test

import (
	"testing"
	"time"

	"github.com/apache/arrow-go/v18/arrow/compute"
	"github.com/apache/arrow-go/v18/arrow/memory"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/hindsight/tables"
	"github.com/theapemachine/symm/hindsight/tables/tablestest"
)

func TestCatalogMeasurements(t *testing.T) {
	Convey("Given measurements appended out of tick order", t, func() {
		catalog := tablestest.New(t)
		writer := tables.NewWriter(catalog)
		observed := time.Date(2026, 9, 8, 12, 0, 0, 0, time.UTC)

		for _, tick := range []int64{3, 1, 2} {
			writer.AddMeasurement(tables.MeasurementRow{
				Epoch:      100,
				Tick:       tick,
				Source:     "strategy",
				Symbol:     "BTC/USD",
				VenueAt:    observed.Add(time.Duration(tick) * time.Second),
				ObservedAt: observed.Add(time.Duration(tick) * time.Second),
				Maturity:   0.85,
				SNR:        2.5,
				SNRDefined: true,
				Metrics:    map[string]float64{"cvd": 12.5, "imbalance": -0.3},
				Metadata:   map[string]float64{"regime": 1.0},
				Payload:    []byte(`{"tick":` + string(rune('0'+tick)) + `}`),
			})
		}

		So(writer.Commit(t.Context()), ShouldBeNil)

		Convey("They read back in tick sequence order", func() {
			rows, err := catalog.Measurements(t.Context(), 100, 0)

			So(err, ShouldBeNil)
			So(len(rows), ShouldEqual, 3)
			So(rows[0].Tick, ShouldEqual, 1)
			So(rows[1].Tick, ShouldEqual, 2)
			So(rows[2].Tick, ShouldEqual, 3)
		})

		Convey("Every column survives the round trip", func() {
			rows, err := catalog.Measurements(t.Context(), 100, 0)

			So(err, ShouldBeNil)
			So(rows[0].Epoch, ShouldEqual, 100)
			So(rows[0].Source, ShouldEqual, "strategy")
			So(rows[0].Symbol, ShouldEqual, "BTC/USD")
			So(rows[0].Maturity, ShouldEqual, 0.85)
			So(rows[0].SNR, ShouldEqual, 2.5)
			So(rows[0].SNRDefined, ShouldBeTrue)
			So(rows[0].Metrics["cvd"], ShouldEqual, 12.5)
			So(rows[0].Metrics["imbalance"], ShouldEqual, -0.3)
			So(rows[0].Metadata["regime"], ShouldEqual, 1.0)
			So(string(rows[0].Payload), ShouldEqual, `{"tick":1}`)
			So(rows[0].ObservedAt.Equal(observed.Add(time.Second)), ShouldBeTrue)
		})

		Convey("A different epoch is pruned out", func() {
			rows, err := catalog.Measurements(t.Context(), 200, 0)

			So(err, ShouldBeNil)
			So(len(rows), ShouldEqual, 0)
		})

		Convey("Successive reads release Arrow storage and retain owned payloads", func() {
			allocator := memory.NewCheckedAllocator(memory.DefaultAllocator)
			ctx := compute.WithAllocator(t.Context(), allocator)
			rows, err := catalog.Measurements(ctx, 100, 0)

			So(err, ShouldBeNil)
			allocator.AssertSize(t, 0)

			writer.AddMeasurement(tables.MeasurementRow{
				Epoch: 200, Tick: 4, Source: "strategy", Symbol: "BTC/USD", Payload: []byte("other epoch"),
			})
			writer.AddMeasurement(tables.MeasurementRow{
				Epoch: 100, Tick: 4, Source: "strategy", Symbol: "BTC/USD", Payload: []byte("new bytes"),
			})
			So(writer.Commit(t.Context()), ShouldBeNil)
			tail, err := catalog.Measurements(ctx, 100, 3)

			So(err, ShouldBeNil)
			So(len(tail), ShouldEqual, 1)
			So(tail[0].Epoch, ShouldEqual, 100)
			So(string(tail[0].Payload), ShouldEqual, "new bytes")
			So(string(rows[0].Payload), ShouldEqual, `{"tick":1}`)
			allocator.AssertSize(t, 0)

			empty, err := catalog.Measurements(ctx, 100, 4)

			So(err, ShouldBeNil)
			So(len(empty), ShouldEqual, 0)
			allocator.AssertSize(t, 0)
		})

		Convey("An incremental read returns only what is newer than afterTick", func() {
			rows, err := catalog.Measurements(t.Context(), 100, 1)

			So(err, ShouldBeNil)
			So(len(rows), ShouldEqual, 2)
			So(rows[0].Tick, ShouldEqual, 2)
		})
	})
}

func TestCatalogPositions(t *testing.T) {
	Convey("Given a position carrying venue decimals", t, func() {
		catalog := tablestest.New(t)
		writer := tables.NewWriter(catalog)

		qty, err := decimal.NewFromString("1.50000000")
		So(err, ShouldBeNil)
		basis, err := decimal.NewFromString("75000.25000000")
		So(err, ShouldBeNil)
		pnl, err := decimal.NewFromString("123.45678901234567")
		So(err, ShouldBeNil)
		entryAt := time.Date(2026, 9, 8, 12, 0, 0, 0, time.UTC)

		zero, err := decimal.NewFromString("0")
		So(err, ShouldBeNil)

		writer.AddPosition(tables.PositionRow{
			Epoch:       1,
			Tick:        10,
			Symbol:      "BTC/USD",
			Status:      "open",
			Qty:         qty,
			Basis:       basis,
			EntryPrice:  basis,
			PnL:         pnl,
			RealizedPnL: zero,
			EntryAt:     &entryAt,
		})

		So(writer.Commit(t.Context()), ShouldBeNil)

		Convey("The decimals survive without float round trip", func() {
			rows, err := catalog.Positions(t.Context(), 1, 0)

			So(err, ShouldBeNil)
			So(len(rows), ShouldEqual, 1)
			So(rows[0].Tick, ShouldEqual, 10)
			So(rows[0].Symbol, ShouldEqual, "BTC/USD")
			So(rows[0].Status, ShouldEqual, "open")
			So(rows[0].Qty.Cmp(qty), ShouldEqual, 0)
			So(rows[0].Basis.Cmp(basis), ShouldEqual, 0)
			So(rows[0].PnL.Cmp(pnl), ShouldEqual, 0)
			So(rows[0].PnL.String(), ShouldStartWith, "123.45678901234567")
			So(rows[0].EntryAt.Equal(entryAt), ShouldBeTrue)
		})
	})
}

func TestCatalogOutcomes(t *testing.T) {
	Convey("Given an outcome row", t, func() {
		catalog := tablestest.New(t)
		writer := tables.NewWriter(catalog)
		outcome := 0.75
		at := time.Date(2026, 9, 8, 12, 0, 0, 0, time.UTC)

		writer.AddOutcome(tables.OutcomeRow{
			Epoch:        1,
			Tick:         5,
			DecisionID:   42,
			Symbol:       "BTC/USD",
			At:           at,
			ActionKind:   "buy",
			ActionPower:  2,
			ActionReduce: false,
			Authority:    0.9,
			Outcome:      &outcome,
		})

		So(writer.Commit(t.Context()), ShouldBeNil)

		Convey("It reads back accurately", func() {
			rows, err := catalog.Outcomes(t.Context(), 1, 0)

			So(err, ShouldBeNil)
			So(len(rows), ShouldEqual, 1)
			So(rows[0].Tick, ShouldEqual, 5)
			So(rows[0].DecisionID, ShouldEqual, 42)
			So(rows[0].Symbol, ShouldEqual, "BTC/USD")
			So(rows[0].ActionKind, ShouldEqual, "buy")
			So(rows[0].ActionPower, ShouldEqual, 2)
			So(rows[0].ActionReduce, ShouldBeFalse)
			So(rows[0].Authority, ShouldEqual, 0.9)
			So(*rows[0].Outcome, ShouldEqual, 0.75)
		})
	})
}

func BenchmarkCatalogMeasurements(b *testing.B) {
	catalog := tablestest.New(b)
	writer := tables.NewWriter(catalog)
	now := time.Now()

	for tick := int64(1); tick <= 4096; tick++ {
		writer.AddMeasurement(tables.MeasurementRow{
			Epoch:      1,
			Tick:       tick,
			Source:     "benchmark",
			Symbol:     "BTC/USD",
			VenueAt:    now,
			ObservedAt: now,
			Maturity:   1.0,
			Payload:    []byte(`{"sample":123}`),
		})
	}

	if err := writer.Commit(b.Context()); err != nil {
		b.Fatal(err)
	}

	for tick := int64(4097); tick <= 4112; tick++ {
		writer.AddMeasurement(tables.MeasurementRow{
			Epoch:      1,
			Tick:       tick,
			Source:     "benchmark",
			Symbol:     "BTC/USD",
			VenueAt:    now,
			ObservedAt: now,
			Maturity:   1.0,
		})
	}

	if err := writer.Commit(b.Context()); err != nil {
		b.Fatal(err)
	}

	for _, fixture := range []struct {
		name  string
		after int64
		rows  int
	}{
		{"whole_epoch", 0, 4112},
		{"new_suffix", 4096, 16},
		{"caught_up", 4112, 0},
	} {
		b.Run(fixture.name, func(b *testing.B) {
			b.ReportAllocs()

			for b.Loop() {
				rows, err := catalog.Measurements(b.Context(), 1, fixture.after)

				if err != nil {
					b.Fatal(err)
				}

				if len(rows) != fixture.rows {
					b.Fatalf("got %d rows, want %d", len(rows), fixture.rows)
				}
			}
		})
	}
}
