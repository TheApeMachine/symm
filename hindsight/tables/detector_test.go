package tables_test

import (
	"testing"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/hindsight/tables"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/tests/market"
)

func TestStreamingDetectorProcess(t *testing.T) {
	Convey("Volume regime changes resolve actual anchors with authoritative quote costs", t, func() {
		detector := tables.NewStreamingDetector(1, market.TrainingPrice(t.Context()))
		var records []tables.ExcursionRecord
		for _, frame := range market.TrainingTape(6) {
			for _, measurement := range frame.Peers {
				record, err := detector.Process(measurement)
				So(err, ShouldBeNil)
				if record != nil {
					records = append(records, *record)
				}
			}
		}
		So(len(records), ShouldBeGreaterThan, 1)
		var profitable, losing bool
		for _, record := range records {
			So(record.AnchorTick, ShouldBeGreaterThan, 0)
			So(record.ExitTick, ShouldBeGreaterThan, record.AnchorTick)
			So(record.Fee, ShouldBeGreaterThan, 0)
			profitable = profitable || record.Profit > 0
			losing = losing || record.Profit < 0
		}
		So(profitable, ShouldBeTrue)
		So(losing, ShouldBeTrue)

		// First record has no prior structural boundary, so precursorStart is 0 and status is unsupported
		So(records[0].PrecursorStartTick, ShouldEqual, 0)
		So(records[0].Status, ShouldEqual, "unsupported")

		// Subsequent records have valid precursor start ticks
		if len(records) > 1 {
			So(records[1].PrecursorStartTick, ShouldEqual, records[0].AnchorTick)
			So(records[1].Status, ShouldEqual, "baseline_shift")
		}

		// Ensure fragment direction is one of the 4 canonical classes
		validDirections := map[string]bool{"UP": true, "DOWN": true, "CHOP": true, "FLAT": true}
		for _, record := range records {
			So(validDirections[record.Direction], ShouldBeTrue)
			// Verify Profit is based on ExitPrice, not ExtremumPrice (no peak profit leakage)
			if record.ExtremumPrice > record.ExitPrice && record.Direction == "UP" {
				// Causal profit must be strictly less than the hypothetical peak profit
				// (ExitPrice is less than ExtremumPrice)
				So(record.Profit, ShouldBeLessThan, record.PositionSize*(record.ExtremumPrice-record.EntryPrice)/record.EntryPrice)
			}
		}
	})

	Convey("Futures and quotes do not advance the volume baseline", t, func() {
		detector := tables.NewStreamingDetector(1, market.TrainingPrice(t.Context()))
		for _, frame := range market.TrainingTape(2) {
			quote, trade := frame.Peers[0], frame.Peers[1]
			trade.Metadata["volume-unit"] = "contracts"
			for _, measurement := range []*data.Measurement[float64]{quote, trade} {
				record, err := detector.Process(measurement)
				So(err, ShouldBeNil)
				So(record, ShouldBeNil)
			}
		}
		So(detector.Anchor("BTC/USD"), ShouldEqual, 0)
	})

	Convey("Quiet flat tape without material excursion completes as FLAT fragment", t, func() {
		price := market.TrainingPrice(t.Context())
		detector := tables.NewStreamingDetector(1, price)

		var flatRecord *tables.ExcursionRecord
		basePrice := 100.0

		for tick := int64(1); tick <= 80; tick++ {
			quote := data.NewMeasurement[float64]("quote", nil)
			quote.SeqIdx, quote.Label = tick, "BTC/USD"
			quote.Metadata["venue"], quote.Metadata["volume-unit"] = "true", "base"
			quote.Provenance["owner"], quote.Provenance["channel"] = "quote", "ticker"
			quote.Metrics["bid"] = data.Metric[float64]{Label: "bid", Raw: basePrice, Exact: decimal.NewFromFloat64(basePrice)}
			quote.Metrics["ask"] = data.Metric[float64]{Label: "ask", Raw: basePrice + 0.01, Exact: decimal.NewFromFloat64(basePrice + 0.01)}

			trade := data.NewMeasurement[float64]("trade", nil)
			trade.SeqIdx, trade.Label = tick, "BTC/USD"
			trade.Metadata["venue"], trade.Metadata["volume-unit"] = "true", "base"
			trade.Provenance["owner"], trade.Provenance["channel"] = "trade", "trade"
			trade.Metrics["price"] = data.Metric[float64]{Label: "price", Raw: basePrice + 0.005, Exact: decimal.NewFromFloat64(basePrice + 0.005)}
			trade.Metrics["qty"] = data.Metric[float64]{Label: "qty", Raw: 1.0, Exact: decimal.NewFromFloat64(1.0)}

			for _, m := range []*data.Measurement[float64]{quote, trade} {
				rec, err := detector.Process(m)
				So(err, ShouldBeNil)
				if rec != nil {
					flatRecord = rec
					break
				}
			}
			if flatRecord != nil {
				break
			}
		}

		So(flatRecord, ShouldNotBeNil)
		So(flatRecord.Direction, ShouldEqual, "FLAT")
		So(flatRecord.AnchorTick, ShouldBeGreaterThan, 0)
		So(flatRecord.ExitTick, ShouldBeGreaterThan, flatRecord.AnchorTick)
	})
}

func BenchmarkStreamingDetectorProcess(b *testing.B) {
	detector := tables.NewStreamingDetector(1, market.TrainingPrice(b.Context()))
	frames := market.TrainingTape(6)
	b.ReportAllocs()

	for index := 0; b.Loop(); index++ {
		for _, measurement := range frames[index%len(frames)].Peers {
			if _, err := detector.Process(measurement); err != nil {
				b.Fatal(err)
			}
		}
	}
}
