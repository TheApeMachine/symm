package strategy

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestLegalActions(t *testing.T) {
	Convey("Given position state conditioning", t, func() {
		Convey("When flat, legal actions are strictly Enter and Wait", func() {
			actions := LegalActions(false)
			So(len(actions), ShouldEqual, 2)
			So(actions[0], ShouldEqual, ActionEnter)
			So(actions[1], ShouldEqual, ActionWait)
		})

		Convey("When holding, legal actions are strictly Exit and Hold", func() {
			actions := LegalActions(true)
			So(len(actions), ShouldEqual, 2)
			So(actions[0], ShouldEqual, ActionExit)
			So(actions[1], ShouldEqual, ActionHold)
		})
	})
}
