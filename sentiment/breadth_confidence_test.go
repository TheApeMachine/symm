package sentiment

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestSentimentConfidence(t *testing.T) {
	Convey("Given breadth and leader change", t, func() {
		confidence := (&Sentiment{}).sentimentConfidence(0.8, 0.04, 0.05, 0)

		Convey("It should align confidence to the leader move", func() {
			So(confidence, ShouldBeGreaterThan, 0)
			So(confidence, ShouldBeLessThanOrEqualTo, 1)
		})
	})

	Convey("Given only peak score fallback", t, func() {
		confidence := (&Sentiment{}).sentimentConfidence(0, 0, 0, 0.6)

		Convey("It should fall back to peak score", func() {
			So(confidence, ShouldBeGreaterThan, 0)
		})
	})
}

func BenchmarkSentimentConfidence(b *testing.B) {
	sentiment := &Sentiment{}

	for b.Loop() {
		_ = sentiment.sentimentConfidence(0.7, 0.03, 0.05, 0.1)
	}
}
