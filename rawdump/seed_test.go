package rawdump

import (
	"os"
	"path/filepath"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestObservationSeed(t *testing.T) {
	Convey("Given a raw JSONL dump with fused observations", t, func() {
		dir := t.TempDir()
		path := filepath.Join(dir, "cvd_raw.jsonl")

		err := os.WriteFile(path, []byte(
			`{"fused":1.5}`+"\n"+
				`{"fused":2.5}`+"\n",
		), 0o644)
		So(err, ShouldBeNil)

		observations, err := ObservationSeedFromPath(path, "fused", 10)

		So(err, ShouldBeNil)
		So(len(observations), ShouldEqual, 2)
		So(observations[0], ShouldEqual, 1.5)
	})
}
