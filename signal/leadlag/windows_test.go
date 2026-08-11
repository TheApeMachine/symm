package leadlag

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestWindowsFromCount(t *testing.T) {
	type expectedWindows struct {
		shortWindow int
		longWindow  int
		returnLag   int
	}

	Convey("Given adversarial sample counts around every square-root transition", t, func() {
		expected := map[int]expectedWindows{
			-1: {0, 0, 0},
			0:  {0, 0, 0},
			1:  {1, 1, 1},
			2:  {2, 2, 1},
			3:  {2, 3, 2},
			4:  {2, 4, 2},
			5:  {3, 5, 3},
			15: {4, 8, 3},
			16: {4, 8, 3},
			17: {5, 10, 4},
			63: {8, 16, 4},
			64: {8, 16, 4},
			65: {9, 18, 5},
		}

		Convey("It should resolve every count to the exact documented windows", func() {
			for sampleCount, windows := range expected {
				shortWindow, longWindow, returnLag := windowsFromCount(sampleCount)
				So(shortWindow, ShouldEqual, windows.shortWindow)
				So(longWindow, ShouldEqual, windows.longWindow)
				So(returnLag, ShouldEqual, windows.returnLag)
			}
		})
	})
}
