package perspectives

import (
	"testing"

	"github.com/smartystreets/goconvey/convey"
)

func TestNewDrivePerspective(t *testing.T) {
	convey.Convey("Given the executed-flow drive playbook", t, func() {
		drive := NewDrivePerspective()

		convey.Convey("When aggressive drive clears its own noise floor", func() {
			action := drive.Decide([]Measurement{measurement(CategoryAggressiveDrive, 1.2)}, nil)

			convey.Convey("It authorizes entry", func() {
				convey.So(action, convey.ShouldNotBeNil)
				convey.So(*action, convey.ShouldEqual, ActionEnter)
			})
		})

		convey.Convey("When hidden absorption clears its own noise floor", func() {
			action := drive.Decide([]Measurement{measurement(CategoryHiddenAbsorption, 1.2)}, nil)

			convey.Convey("It authorizes entry", func() {
				convey.So(action, convey.ShouldNotBeNil)
				convey.So(*action, convey.ShouldEqual, ActionEnter)
			})
		})

		convey.Convey("When the drive reading is under the noise floor", func() {
			action := drive.Decide([]Measurement{measurement(CategoryAggressiveDrive, 0.8)}, nil)

			convey.Convey("It does not authorize entry", func() {
				convey.So(action, convey.ShouldBeNil)
			})
		})
	})
}

func BenchmarkDrivePerspectiveWalk(b *testing.B) {
	drive := NewDrivePerspective()
	measurements := []Measurement{measurement(CategoryAggressiveDrive, 1.2)}

	for b.Loop() {
		_ = drive.Decide(measurements, nil)
	}
}
