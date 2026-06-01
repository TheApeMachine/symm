package replay

import (
	"os"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestResetShared(t *testing.T) {
	Convey("Given a cached replay hub", t, func() {
		path := t.TempDir() + "/capture.jsonl"

		err := os.WriteFile(path, []byte{}, 0o600)
		So(err, ShouldBeNil)

		first, err := Open(path)

		So(err, ShouldBeNil)

		ResetShared()

		second, err := Open(path)

		Convey("It should rebuild the hub after reset", func() {
			So(err, ShouldBeNil)
			So(first, ShouldNotEqual, second)
		})
	})
}
