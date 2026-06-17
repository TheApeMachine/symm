package resonance

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/nomagique/learning"
	"github.com/theapemachine/symm/logic"
)

func TestSignalPredictionFrames(testingTB *testing.T) {
	Convey("Given a settled batch with resonance layers", testingTB, func() {
		signal := &Signal{
			lastSettled: []settledSymbolEntry{{
				measurement: logic.Measurement{
					Symbol:     "BTC/EUR",
					ObservedAt: time.Date(2026, 6, 2, 12, 0, 0, 0, time.UTC),
				},
				layers: []learning.ResonanceLayerWire{{
					State:      []float64{0.2, 0.4},
					Prediction: []float64{0.3, 0.5},
					ErrorNorm:  0.1,
				}},
			}},
		}

		frames := signal.PredictionFrames(60)

		Convey("It should publish prediction, actual, and error points", func() {
			So(len(frames), ShouldEqual, 3)
			So(frames[0]["kind"], ShouldEqual, "prediction")
			So(frames[1]["kind"], ShouldEqual, "actual")
			So(frames[2]["kind"], ShouldEqual, "error")
		})
	})
}

func BenchmarkSignalPredictionFrames(b *testing.B) {
	signal := &Signal{
		lastSettled: []settledSymbolEntry{{
			measurement: logic.Measurement{
				Symbol:     "BTC/EUR",
				ObservedAt: time.Now(),
			},
			layers: []learning.ResonanceLayerWire{{
				State:      []float64{0.2, 0.4, 0.6},
				Prediction: []float64{0.3, 0.5, 0.7},
				ErrorNorm:  0.1,
			}},
		}},
	}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_ = signal.PredictionFrames(60)
	}
}
