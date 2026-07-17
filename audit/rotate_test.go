package audit

import (
	"os"
	"path/filepath"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestRotate(t *testing.T) {
	Convey("Given an existing audit log", t, func() {
		dir := t.TempDir()
		path := filepath.Join(dir, "runtime-audit.jsonl")
		So(os.WriteFile(path, []byte("{\"tick\":1}\n"), 0o644), ShouldBeNil)

		Convey("When Rotate runs", func() {
			So(Rotate(path), ShouldBeNil)

			Convey("Then the original path is free and a stamped sibling exists", func() {
				_, err := os.Stat(path)
				So(os.IsNotExist(err), ShouldBeTrue)
				entries, err := os.ReadDir(dir)
				So(err, ShouldBeNil)
				So(entries, ShouldHaveLength, 1)
				So(entries[0].Name(), ShouldStartWith, "runtime-audit.jsonl.")
			})
		})
	})

	Convey("Given a missing audit log", t, func() {
		So(Rotate(filepath.Join(t.TempDir(), "missing.jsonl")), ShouldBeNil)
	})
}
