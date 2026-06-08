package telemetry

import (
	"container/ring"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/symm/logic"
)

func TestGaugeMeans(t *testing.T) {
	Convey("Given readings in the ring", t, func() {
		readings := ring.New(8)
		readings.Value = &Reading{Confidence: 0.8, Surprise: 2.0}
		readings = readings.Next()
		readings.Value = &Reading{Confidence: 0.4, Surprise: 1.0}
		readings = readings.Next()

		meanConfidence, meanSurprise, confidenceCount, surpriseCount := means(readings)

		Convey("It should average positive confidence and surprise", func() {
			So(meanConfidence/confidenceCount, ShouldAlmostEqual, 0.6, 1e-9)
			So(meanSurprise/surpriseCount, ShouldAlmostEqual, 1.5, 1e-9)
		})
	})

	Convey("Given no positive readings", t, func() {
		readings := ring.New(8)

		_, _, confidenceCount, surpriseCount := means(readings)

		Convey("It should report zero samples", func() {
			So(confidenceCount, ShouldEqual, 0)
			So(surpriseCount, ShouldEqual, 0)
		})
	})
}

func TestNewGauge(t *testing.T) {
	Convey("Given telemetry capacity config", t, func() {
		viper.Set("telemetry.gauge.readings_capacity", 32)

		gauge, err := NewGauge(nil, logic.SourceNone)

		Convey("It should allocate the reading ring", func() {
			So(err, ShouldBeNil)
			So(gauge.readings.Len(), ShouldEqual, 32)
		})
	})
}

func means(readings *ring.Ring) (
	meanConfidence float64,
	meanSurprise float64,
	confidenceCount float64,
	surpriseCount float64,
) {
	readings.Do(func(value any) {
		if value == nil {
			return
		}

		reading, ok := value.(*Reading)

		if !ok {
			return
		}

		if reading.Confidence > 0 {
			meanConfidence += reading.Confidence
			confidenceCount++
		}

		if reading.Surprise > 0 {
			meanSurprise += reading.Surprise
			surpriseCount++
		}
	})

	return meanConfidence, meanSurprise, confidenceCount, surpriseCount
}

func BenchmarkGaugeMeans(b *testing.B) {
	readings := ring.New(256)

	for index := range 64 {
		readings.Value = &Reading{
			Confidence: 0.5,
			Surprise:   1.2 + float64(index)*0.01,
		}
		readings = readings.Next()
	}

	b.ReportAllocs()

	for b.Loop() {
		meanConfidence, meanSurprise, confidenceCount, surpriseCount := means(readings)
		_ = meanConfidence / confidenceCount
		_ = meanSurprise / surpriseCount
	}
}
