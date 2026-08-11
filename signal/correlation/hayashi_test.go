package correlation

import (
	"math"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	nomcorrelation "github.com/theapemachine/nomagique/correlation"
)

func TestSupportedCorrelation(t *testing.T) {
	Convey("Given asynchronous paths with unsupported prefixes and suffixes", t, func() {
		start := time.Unix(1_700_000_000, 0).UTC()
		left := []nomcorrelation.Sample{
			{At: start, Value: 50},
			{At: start.Add(time.Second), Value: 100},
			{At: start.Add(2 * time.Second), Value: 110},
			{At: start.Add(3 * time.Second), Value: 121},
			{At: start.Add(4 * time.Second), Value: 200},
		}
		right := []nomcorrelation.Sample{
			{At: start.Add(1500 * time.Millisecond), Value: 200},
			{At: start.Add(2500 * time.Millisecond), Value: 220},
			{At: start.Add(3500 * time.Millisecond), Value: 242},
		}
		leftLogs := sampleLogs(left)
		rightLogs := sampleLogs(right)

		correlationValue, support, ok := supportedCorrelation(
			left, right, leftLogs, rightLogs,
		)

		Convey("It should normalize only overlapping return support", func() {
			So(ok, ShouldBeTrue)
			So(support, ShouldEqual, 2)
			So(correlationValue, ShouldEqual, 1.0)
		})
	})

	Convey("Given only one supported return on either path", t, func() {
		start := time.Unix(1_700_000_000, 0).UTC()
		left := []nomcorrelation.Sample{
			{At: start, Value: 100},
			{At: start.Add(time.Second), Value: 110},
		}
		right := []nomcorrelation.Sample{
			{At: start, Value: 200},
			{At: start.Add(time.Second), Value: 220},
		}

		_, support, ok := supportedCorrelation(
			left, right, sampleLogs(left), sampleLogs(right),
		)

		So(ok, ShouldBeFalse)
		So(support, ShouldEqual, 0)
	})
}

func TestHayashiMoments(t *testing.T) {
	Convey("Given two aligned paths with hand-constructed log returns", t, func() {
		start := time.Unix(1_700_000_100, 0).UTC()
		left := []nomcorrelation.Sample{
			{At: start, Value: 1},
			{At: start.Add(time.Second), Value: 1},
			{At: start.Add(2 * time.Second), Value: 1},
		}
		right := append([]nomcorrelation.Sample(nil), left...)
		covariance, leftVariance, rightVariance, support := hayashiMoments(
			left,
			right,
			[]float64{0, 1, 3},
			[]float64{0, 2, 6},
		)

		Convey("It should sum only pairwise overlapping increments exactly", func() {
			So(covariance, ShouldEqual, 10.0)
			So(leftVariance, ShouldEqual, 5.0)
			So(rightVariance, ShouldEqual, 20.0)
			So(support, ShouldEqual, 2)
		})
	})

	Convey("Given intervals that only touch at one endpoint", t, func() {
		start := time.Unix(1_700_000_200, 0).UTC()
		left := []nomcorrelation.Sample{
			{At: start, Value: 1},
			{At: start.Add(time.Second), Value: 2},
			{At: start.Add(2 * time.Second), Value: 4},
		}
		right := []nomcorrelation.Sample{
			{At: start.Add(2 * time.Second), Value: 1},
			{At: start.Add(3 * time.Second), Value: 2},
			{At: start.Add(4 * time.Second), Value: 4},
		}
		covariance, leftVariance, rightVariance, support := hayashiMoments(
			left,
			right,
			sampleLogs(left),
			sampleLogs(right),
		)

		Convey("It should not invent covariance from non-overlapping support", func() {
			So(covariance, ShouldEqual, 0.0)
			So(leftVariance, ShouldEqual, 0.0)
			So(rightVariance, ShouldEqual, 0.0)
			So(support, ShouldEqual, 0)
		})
	})
}

func sampleLogs(samples []nomcorrelation.Sample) []float64 {
	logs := make([]float64, 0, len(samples))

	for _, sample := range samples {
		logs = append(logs, math.Log(sample.Value))
	}

	return logs
}
