package replay

import (
	"os"
	"strings"
	"testing"

	"github.com/smartystreets/goconvey/convey"
)

func TestWalkForwardHoldouts(t *testing.T) {
	convey.Convey("Given a long capture fixture", t, func() {
		path := writeTempCapture(t, 500)

		holdouts, cleanup, err := WalkForwardHoldouts(path, 3, 0.2)

		defer cleanup()

		convey.Convey("It should produce multiple expanding holdout tails", func() {
			convey.So(err, convey.ShouldBeNil)
			convey.So(len(holdouts), convey.ShouldBeGreaterThanOrEqualTo, 2)
		})
	})
}

func writeTempCapture(t *testing.T, lines int) string {
	t.Helper()

	file, err := os.CreateTemp("", "symm-capture-*.jsonl")

	if err != nil {
		t.Fatal(err)
	}

	builder := strings.Builder{}

	for index := 0; index < lines; index++ {
		builder.WriteString(`{"ts":"2026-05-23T12:00:00Z","transport":"ws","channel":"trade","payload":{"channel":"trade","type":"update","data":[]}}`)

		if index < lines-1 {
			builder.WriteByte('\n')
		}
	}

	if _, err := file.WriteString(builder.String()); err != nil {
		t.Fatal(err)
	}

	if err := file.Close(); err != nil {
		t.Fatal(err)
	}

	return file.Name()
}
