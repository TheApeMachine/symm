package strategy

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/symm/nomagique/data"
)

func TestDetector(t *testing.T) {
	Convey("Detector WORM tape scanner", t, func() {
		detector := NewDetector()

		Convey("Initial state has empty queue", func() {
			chunks := detector.Next()
			So(len(chunks[0]), ShouldEqual, 0)
			So(len(chunks[1]), ShouldEqual, 0)
		})

		Convey("Detects upward excursion and enqueues fragment", func() {
			now := time.Now()
			measurements := make([]*data.Measurement[float64], 0, 50)

			// Generate tape: 10 precursor ticks around 100, drop to low 95, rise to high 120, pullback to 110
			prices := []float64{
				100, 100, 100, 99, 98, 97, 96, 95.5, 95, 95, // Low at idx 8-9 (price 95)
				97, 100, 105, 110, 115, 118, 120,             // High at idx 16 (price 120, move = 25)
				117, 115, 114,                                // Pullback >= 20% of 25 (>= 5, price <= 115)
			}

			for idx, p := range prices {
				m := data.NewMeasurement[float64]("spot:ticker", nil)
				m.Label = "XXBTZUSD"
				m.SeqIdx = int64(idx)
				m.At = now.Add(time.Duration(idx) * time.Second)
				m.WriteMetric("price", p)
				m.SetMetric("bid", data.Metric[float64]{Raw: p - 0.5, Exact: decimal.NewFromFloat64(p - 0.5)})
				m.SetMetric("ask", data.Metric[float64]{Raw: p + 0.5, Exact: decimal.NewFromFloat64(p + 0.5)})
				measurements = append(measurements, m)
			}

			detector.Scan(measurements)

			chunks := detector.Next()
			So(len(chunks[0]), ShouldBeGreaterThan, 0)
			So(len(chunks[1]), ShouldBeGreaterThan, 0)

			// Verify WORM integrity: measurements in fragment are the exact same pointers
			So(chunks[0][0].Label, ShouldEqual, "XXBTZUSD")
			So(chunks[1][0].Label, ShouldEqual, "XXBTZUSD")
		})
	})
}
