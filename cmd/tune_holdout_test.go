package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestResolveTuneReplayPaths(t *testing.T) {
	Convey("Given auto-holdout on a long capture", t, func() {
		path := filepath.Join(t.TempDir(), "capture.jsonl")
		lines := make([]string, 250)

		for index := range lines {
			lines[index] = `{"transport":"ws"}`
		}

		So(os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o644), ShouldBeNil)

		paths, err := resolveTuneReplayPaths(path, nil, true)
		So(err, ShouldBeNil)

		if paths.cleanup != nil {
			defer paths.cleanup()
		}

		Convey("It should reserve a holdout tail", func() {
			So(paths.trainPath, ShouldNotEqual, path)
			So(len(paths.holdoutPaths), ShouldEqual, 1)
		})
	})

	Convey("Given explicit holdout paths", t, func() {
		paths, err := resolveTuneReplayPaths("runs/capture.jsonl", []string{"runs/holdout.jsonl"}, true)

		Convey("It should not auto-split", func() {
			So(err, ShouldBeNil)
			So(paths.trainPath, ShouldEqual, "runs/capture.jsonl")
			So(paths.holdoutPaths, ShouldResemble, []string{"runs/holdout.jsonl"})
			So(paths.cleanup, ShouldBeNil)
		})
	})
}
