package replay

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/smartystreets/goconvey/convey"
)

func TestSplitHoldout(t *testing.T) {
	convey.Convey("Given a replay with enough lines", t, func() {
		path := filepath.Join(t.TempDir(), "capture.jsonl")
		lines := make([]string, 250)

		for index := range lines {
			lines[index] = `{"ts":"2026-01-01T00:00:00Z","transport":"ws","channel":"ticker","payload":{}}`
		}

		convey.So(os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o644), convey.ShouldBeNil)

		trainPath, holdoutPath, ok, err := SplitHoldout(path, 0.2, 200)
		t.Cleanup(func() { RemoveSplitFiles(trainPath, holdoutPath) })

		convey.Convey("It should split into train and holdout files", func() {
			convey.So(err, convey.ShouldBeNil)
			convey.So(ok, convey.ShouldBeTrue)

			trainPayload, readErr := os.ReadFile(trainPath)
			convey.So(readErr, convey.ShouldBeNil)
			convey.So(len(nonEmptyLines(trainPayload)), convey.ShouldEqual, 200)

			holdoutPayload, readErr := os.ReadFile(holdoutPath)
			convey.So(readErr, convey.ShouldBeNil)
			convey.So(len(nonEmptyLines(holdoutPayload)), convey.ShouldEqual, 50)
		})
	})

	convey.Convey("Given a short replay", t, func() {
		path := filepath.Join(t.TempDir(), "short.jsonl")
		convey.So(os.WriteFile(path, []byte(`{"transport":"ws"}`+"\n"), 0o644), convey.ShouldBeNil)

		_, _, ok, err := SplitHoldout(path, 0.2, 200)

		convey.Convey("It should skip holdout split", func() {
			convey.So(err, convey.ShouldBeNil)
			convey.So(ok, convey.ShouldBeFalse)
		})
	})
}
