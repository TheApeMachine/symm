package perspectives

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

type stubFeedback struct {
	mse float64
}

func (feedback *stubFeedback) MSE() float64 {
	return feedback.mse
}

func TestFeedbackInterface(t *testing.T) {
	Convey("Given a feedback implementation", t, func() {
		feedback := &stubFeedback{mse: 0.042}

		Convey("It should expose mean squared error", func() {
			var iface Feedback = feedback

			So(iface.MSE(), ShouldAlmostEqual, 0.042, 0.0001)
		})
	})
}
