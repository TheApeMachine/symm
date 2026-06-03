package log

import (
	"bytes"
	"io"
	"os"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestTuneLog(t *testing.T) {
	Convey("Given stderr redirected", t, func() {
		original := os.Stderr
		reader, writer, err := os.Pipe()

		So(err, ShouldBeNil)

		os.Stderr = writer

		TuneLog("phase %s", "bootstrap")

		writer.Close()
		os.Stderr = original

		var buffer bytes.Buffer
		_, _ = io.Copy(&buffer, reader)

		Convey("It should prefix tune progress lines", func() {
			So(buffer.String(), ShouldContainSubstring, "symm tune: phase bootstrap")
		})
	})
}
