package sensorium

import (
	"fmt"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestAdvanceCoupled(t *testing.T) {
	Convey("A wrapped numerical rejection keeps the coupled retry contract", t, func() {
		controls := defaultPhysicsControls()
		attempts, accepted := 0, 0.0
		snapshot := func() func() {
			prior := accepted
			return func() { accepted = prior }
		}
		request := controls.MaxStep
		health, err := advanceCoupled(request, controls, snapshot, func() (float64, error) { return request, nil }, func(step float32) error {
			attempts++
			accepted += float64(step)
			if attempts == 1 {
				return fmt.Errorf("remap diagnostic: %w", &CoupledStepError{Operator: "test", Retry: true})
			}
			return nil
		})
		So(err, ShouldBeNil)
		So(health.Rejections, ShouldEqual, 1)
		So(accepted, ShouldEqual, float64(float32(request)))
		So(health.AcceptedDT, ShouldEqual, accepted)
	})
}
