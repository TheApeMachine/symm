package main

import (
	"os"
	"path/filepath"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestWriteNativeSourceDigest(t *testing.T) {
	Convey("Nested native edits invalidate the generated Go-visible header", t, func() {
		root := t.TempDir()
		So(os.Mkdir(filepath.Join(root, "shared"), 0755), ShouldBeNil)
		source := filepath.Join(root, "shared", "solver.inc")
		So(os.WriteFile(source, []byte("original"), 0644), ShouldBeNil)
		generator := NewGenerator(root, t.TempDir())
		So(generator.WriteNativeSourceDigest(), ShouldBeNil)
		original, err := os.ReadFile(filepath.Join(root, "native_sources.h"))
		So(err, ShouldBeNil)
		So(generator.WriteNativeSourceDigest(), ShouldBeNil)
		repeated, err := os.ReadFile(filepath.Join(root, "native_sources.h"))
		So(err, ShouldBeNil)
		So(repeated, ShouldResemble, original)
		So(os.WriteFile(source, []byte("modified"), 0644), ShouldBeNil)
		So(generator.WriteNativeSourceDigest(), ShouldBeNil)
		changed, err := os.ReadFile(filepath.Join(root, "native_sources.h"))
		So(err, ShouldBeNil)
		So(string(changed), ShouldNotEqual, string(original))
	})
}
