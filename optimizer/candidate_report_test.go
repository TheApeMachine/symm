package optimizer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/market/perspectives"
)

func TestNewCandidateReporter(t *testing.T) {
	convey.Convey("Given an empty report path", t, func() {
		reporter, err := newCandidateReporter("")

		convey.Convey("It should disable candidate reporting", func() {
			convey.So(err, convey.ShouldBeNil)
			convey.So(reporter, convey.ShouldBeNil)
		})
	})

	convey.Convey("Given a candidate report file path", t, func() {
		path := filepath.Join(t.TempDir(), "reports", "candidates.jsonl")
		reporter, err := newCandidateReporter(path)

		convey.Convey("It should construct a JSONL reporter", func() {
			convey.So(err, convey.ShouldBeNil)
			convey.So(reporter, convey.ShouldNotBeNil)
		})

		convey.Convey("It should write one JSON object per candidate", func() {
			writeErr := reporter.Write(CandidateScore{
				Candidate: 1,
				Score:     10,
				Branches: perspectives.BranchList{{
					Category: perspectives.CategoryLaminar,
				}},
			})
			closeErr := reporter.Close()
			raw, readErr := os.ReadFile(path)

			convey.So(writeErr, convey.ShouldBeNil)
			convey.So(closeErr, convey.ShouldBeNil)
			convey.So(readErr, convey.ShouldBeNil)
			convey.So(strings.TrimSpace(string(raw)), convey.ShouldContainSubstring, `"candidate":1`)
		})
	})
}
