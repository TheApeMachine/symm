package tables_test

import (
	"context"
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/hindsight/tables"
	"github.com/theapemachine/symm/tests/tablestest"
)

func TestCatalogRuns(t *testing.T) {
	Convey("Given an Iceberg catalog with populated tables", t, func() {
		ctx := context.Background()
		catalog := tablestest.New(t)
		writer := tables.NewWriter(catalog)

		epoch := int64(1)
		now := time.Date(2026, 9, 13, 12, 0, 0, 0, time.UTC)
		price, _ := decimal.NewFromString("50000.0")
		qty, _ := decimal.NewFromString("1.5")
		fee, _ := decimal.NewFromString("5.0")

		writer.AddPosition(tables.PositionRow{
			Epoch:      epoch,
			Tick:       10,
			Symbol:     "BTC/USD",
			Status:     "OPEN",
			Qty:        qty,
			EntryPrice: price,
			EntryFee:   fee,
			EntryAt:    &now,
		})

		err := writer.Commit(ctx)
		So(err, ShouldBeNil)

		Convey("When scanning runs", func() {
			runs, err := catalog.Runs(ctx)
			So(err, ShouldBeNil)

			Convey("Then it returns the run with correct position count", func() {
				So(len(runs), ShouldEqual, 1)
				So(runs[0].ID, ShouldEqual, "1")
				So(runs[0].Positions, ShouldEqual, 1)
				So(runs[0].Integrity, ShouldEqual, "COMPLETE")
			})
		})
	})
}

func TestCatalogLifecycle(t *testing.T) {
	Convey("Given an Iceberg catalog with positions and executions", t, func() {
		ctx := context.Background()
		catalog := tablestest.New(t)
		writer := tables.NewWriter(catalog)

		epoch := int64(1)
		now := time.Date(2026, 9, 13, 12, 0, 0, 0, time.UTC)
		price, _ := decimal.NewFromString("50000.0")
		qty, _ := decimal.NewFromString("1.5")
		fee, _ := decimal.NewFromString("5.0")

		writer.AddPosition(tables.PositionRow{
			Epoch:      epoch,
			Tick:       15,
			Symbol:     "BTC/USD",
			Status:     "OPEN",
			Qty:        qty,
			EntryPrice: price,
			EntryFee:   fee,
			EntryAt:    &now,
		})

		writer.AddExecution(tables.ExecutionRow{
			Epoch:       epoch,
			Tick:        20,
			Symbol:      "BTC/USD",
			VenueAt:     now,
			OrderID:     "order-123",
			Side:        "BUY",
			OrderStatus: "filled",
			LastPrice:   price,
			LastQty:     qty,
			CumQty:      qty,
			FeeUsdEquiv: fee,
		})

		err := writer.Commit(ctx)
		So(err, ShouldBeNil)

		Convey("When reading lifecycle events", func() {
			events, err := catalog.Lifecycle(ctx, epoch)
			So(err, ShouldBeNil)

			Convey("Then position open and fills are returned", func() {
				So(len(events), ShouldBeGreaterThanOrEqualTo, 2)
				So(events[0].Symbol, ShouldEqual, "BTC/USD")
				So(events[0].CaptureSeq, ShouldEqual, 15)
			})
		})
	})
}

func TestCatalogTimeline(t *testing.T) {
	Convey("Given an Iceberg catalog with spot ticker and trade updates", t, func() {
		ctx := context.Background()
		catalog := tablestest.New(t)
		writer := tables.NewWriter(catalog)

		epoch := int64(1)
		now := time.Date(2026, 9, 13, 12, 0, 0, 0, time.UTC)

		writer.AddSpotTicker(tables.SpotTickerRow{
			Epoch:      epoch,
			Tick:       100,
			Symbol:     "BTC/USD",
			VenueAt:    now,
			ReceivedAt: now,
			Bid:        49990.0,
			BidQty:     2.0,
			Ask:        50010.0,
			AskQty:     3.0,
			Last:       50000.0,
			Volume:     10.0,
		})

		writer.AddSpotTrade(tables.SpotTradeRow{
			Epoch:      epoch,
			Tick:       105,
			Symbol:     "BTC/USD",
			VenueAt:    now.Add(time.Second),
			ReceivedAt: now.Add(time.Second),
			Price:      50005.0,
			Qty:        0.5,
			Side:       "buy",
		})

		err := writer.Commit(ctx)
		So(err, ShouldBeNil)

		Convey("When requesting timeline projection", func() {
			query := tables.TimelineQuery{
				Run:     "1",
				Symbol:  "BTC/USD",
				Buckets: 10,
			}

			timeline, err := catalog.Timeline(ctx, query)
			So(err, ShouldBeNil)

			Convey("Then buckets and symbol summaries are correctly populated", func() {
				So(timeline, ShouldNotBeNil)
				So(timeline.Symbol, ShouldEqual, "BTC/USD")
				So(len(timeline.Buckets), ShouldEqual, 10)
				So(timeline.TotalObservations, ShouldEqual, 2)
				So(len(timeline.Symbols), ShouldEqual, 1)
				So(timeline.Symbols[0].Symbol, ShouldEqual, "BTC/USD")
			})
		})
	})
}

func TestCatalogCaptures(t *testing.T) {
	Convey("Given an Iceberg catalog with ticker and level3 updates", t, func() {
		ctx := context.Background()
		catalog := tablestest.New(t)
		writer := tables.NewWriter(catalog)

		epoch := int64(1)
		now := time.Date(2026, 9, 13, 12, 0, 0, 0, time.UTC)

		writer.AddSpotTicker(tables.SpotTickerRow{
			Epoch:      epoch,
			Tick:       50,
			Symbol:     "BTC/USD",
			VenueAt:    now,
			ReceivedAt: now,
			Last:       50000.0,
		})

		writer.AddSpotLevel3(tables.SpotLevel3Row{
			Epoch:      epoch,
			Tick:       55,
			Symbol:     "BTC/USD",
			VenueAt:    now,
			ReceivedAt: now,
			Side:       "buy",
			Event:      "add",
			OrderID:    "ord-1",
			LimitPrice: 49999.0,
			OrderQty:   1.0,
		})

		err := writer.Commit(ctx)
		So(err, ShouldBeNil)

		Convey("When querying captures strictly after tick 40", func() {
			captures, err := catalog.Captures(ctx, epoch, 40)
			So(err, ShouldBeNil)

			Convey("Then all frames after tick 40 are listed in order", func() {
				So(len(captures), ShouldEqual, 2)
				So(captures[0].Identity.Sequence, ShouldEqual, 50)
				So(captures[1].Identity.Sequence, ShouldEqual, 55)
			})
		})
	})
}

func TestCatalogEnvelopeAt(t *testing.T) {
	Convey("Given an Iceberg catalog with a spot ticker frame", t, func() {
		ctx := context.Background()
		catalog := tablestest.New(t)
		writer := tables.NewWriter(catalog)

		epoch := int64(1)
		now := time.Date(2026, 9, 13, 12, 0, 0, 0, time.UTC)

		writer.AddSpotTicker(tables.SpotTickerRow{
			Epoch:      epoch,
			Tick:       75,
			Symbol:     "BTC/USD",
			VenueAt:    now,
			ReceivedAt: now,
			Last:       50000.0,
		})

		err := writer.Commit(ctx)
		So(err, ShouldBeNil)

		Convey("When retrieving envelope at tick 75", func() {
			envelope, err := catalog.EnvelopeAt(ctx, epoch, 75)
			So(err, ShouldBeNil)

			Convey("Then the envelope manifests and capture metadata match", func() {
				So(envelope, ShouldNotBeNil)
				So(envelope.Sequence, ShouldEqual, 75)
				So(len(envelope.Manifests), ShouldEqual, 1)
				So(envelope.Manifests[0].Symbol, ShouldEqual, "BTC/USD")
			})
		})
	})
}

func TestCatalogResidentAt(t *testing.T) {
	Convey("Given an Iceberg catalog with measurement observations", t, func() {
		ctx := context.Background()
		catalog := tablestest.New(t)
		writer := tables.NewWriter(catalog)

		epoch := int64(1)
		now := time.Date(2026, 9, 13, 12, 0, 0, 0, time.UTC)

		writer.AddMeasurement(tables.MeasurementRow{
			Epoch:      epoch,
			Tick:       200,
			Source:     "toxicity",
			Symbol:     "BTC/USD",
			VenueAt:    now,
			ObservedAt: now,
			Maturity:   0.85,
			SNR:        12.4,
			SNRDefined: true,
			Metrics: map[string]float64{
				"flow_toxicity": 0.42,
			},
		})

		err := writer.Commit(ctx)
		So(err, ShouldBeNil)

		Convey("When querying resident state at tick 205", func() {
			resident, err := catalog.ResidentAt(ctx, epoch, "BTC/USD", 205, 64)
			So(err, ShouldBeNil)

			Convey("Then the resident signals contain the toxicity measurement", func() {
				So(resident, ShouldNotBeNil)
				So(len(resident.Signals), ShouldEqual, 1)
				So(resident.Signals[0].Source, ShouldEqual, "toxicity")
				So(len(resident.Signals[0].Metrics), ShouldEqual, 1)
				So(resident.Signals[0].Metrics[0].Key, ShouldEqual, "flow_toxicity")
				So(resident.Signals[0].Metrics[0].Raw, ShouldEqual, 0.42)
			})
		})
	})
}
