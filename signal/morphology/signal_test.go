package morphology_test

import (
	"context"
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique/data"
	nmruntime "github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/signal/morphology"
)

func ingress(label string, at time.Time, seq int64) *data.Measurement {
	prior := data.NewMeasurement(1, label, "spot:trade", seq, seq)
	prior.At, prior.From = at, at
	prior.Write(data.NewMetric("price", 12, data.UnitCurrency, data.TimescaleInstantaneous))
	return prior
}
func metric(measurement *data.Measurement, label string) (float64, bool) {
	for entry := range measurement.Read(label) {
		if entry.Err != nil || entry.Metric == nil {
			return 0, false
		}
		return entry.Metric.Raw, true
	}
	return 0, false
}
func metricValue(measurement *data.Measurement, label string) float64 {
	value, held := metric(measurement, label)
	So(held, ShouldBeTrue)
	return value
}
func writeBook(t *testing.T, books *broker.Book, symbol string, at time.Time, bids, asks []float64) {
	t.Helper()
	var orders [2][]kraken.Level3Order
	for side, values := range [2][]float64{bids, asks} {
		for index := 0; index < len(values); index += 2 {
			orders[side] = append(orders[side], kraken.Level3Order{
				OrderID:    fmt.Sprintf("%s-%d-%d", symbol, side, index),
				LimitPrice: decimal.NewFromFloat64(values[index]), OrderQty: decimal.NewFromFloat64(values[index+1]), Timestamp: at, Event: "add",
			})
		}
	}
	if err := books.Update(&kraken.Level3{Channel: "level3", Type: "snapshot", Data: []kraken.Level3Data{{Symbol: symbol, Bids: orders[0], Asks: orders[1]}}}); err != nil {
		t.Fatal(err)
	}
}
func TestMorphologyLevel3Metrics(t *testing.T) {
	Convey("Given a real book source and the single morphology pipeline", t, func() {
		ctx := context.Background()
		books := broker.NewBook(ctx, spot.NewNormalizer())
		instrument := morphology.NewSignal(ctx, books)
		instrument.Transition(nmruntime.READY)
		at := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
		Convey("Mirrored shapes expose six measurements, not an invented first change", func() {
			writeBook(t, books, "BTC/USD", at, []float64{10, .1, 6, 1.0 / 6}, []float64{14, 1.0 / 14, 18, 1.0 / 18})
			prior := ingress("BTC/USD", at, 1)
			result := instrument.Step(prior)
			So(result, ShouldNotBeNil)
			So(instrument.Error(), ShouldBeNil)
			So(metricValue(result, "book_shape_distance"), ShouldAlmostEqual, 0, 1e-10)
			So(metricValue(result, "book_shape_ks"), ShouldAlmostEqual, 0, 1e-10)
			So(metricValue(result, "concentration:bid"), ShouldAlmostEqual, .5, 1e-10)
			So(metricValue(result, "concentration:ask"), ShouldAlmostEqual, .5, 1e-10)
			So(metricValue(result, "entropy:bid"), ShouldAlmostEqual, math.Log(2), 1e-10)
			So(metricValue(result, "entropy:ask"), ShouldAlmostEqual, math.Log(2), 1e-10)
			_, present := metric(result, "morphology_change")
			So(present, ShouldBeFalse)
			So(prior.From.Equal(at), ShouldBeTrue)
			So(prior.At.Equal(at), ShouldBeTrue)
			Convey("Moving both outer levels changes the book although bilateral distance stays zero", func() {
				nextAt := at.Add(time.Second)
				writeBook(t, books, "BTC/USD", nextAt, []float64{10, .1, 2, .5}, []float64{14, 1.0 / 14, 22, 1.0 / 22})
				changed := instrument.Step(ingress("BTC/USD", nextAt, 2))
				So(changed, ShouldNotBeNil)
				So(metricValue(changed, "book_shape_distance"), ShouldAlmostEqual, 0, 1e-10)
				So(metricValue(changed, "morphology_change"), ShouldAlmostEqual, .5, 1e-10)
				repeated := instrument.Step(ingress("BTC/USD", nextAt.Add(time.Second), 3))
				So(metricValue(repeated, "morphology_change"), ShouldAlmostEqual, 0, 1e-10)
			})
		})
		Convey("An unavailable book does not manufacture geometry", func() {
			So(instrument.Step(ingress("BTC/USD", at, 1)), ShouldBeNil)
			So(instrument.Error(), ShouldBeNil)
		})
	})
}
func TestMorphologySignalRequiresBookManager(t *testing.T) {
	Convey("A missing book source is a construction failure", t, func() {
		instrument := morphology.NewSignal(context.Background(), nil)
		So(instrument.Error(), ShouldNotBeNil)
		So(instrument.Status(), ShouldNotEqual, nmruntime.READY)
	})
}
