package market

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken/public"
	"github.com/theapemachine/symm/replay"
)

func captureFixturePath(t *testing.T) string {
	t.Helper()

	candidate := os.Getenv("SYMM_CAPTURE_FIXTURE")

	if candidate == "" {
		t.Skip("capture integrity tests require SYMM_CAPTURE_FIXTURE=/path/to/capture.jsonl")

		return ""
	}

	if _, err := os.Stat(candidate); err != nil {
		t.Fatalf("capture fixture %q: %v", candidate, err)
	}

	absolute, err := filepath.Abs(candidate)

	if err != nil {
		t.Fatalf("capture fixture abs: %v", err)
	}

	return absolute
}

func TestCaptureReplayMaintainsBookChecksums(t *testing.T) {
	capturePath := captureFixturePath(t)

	Convey("Given the recorded Kraken book channel in "+capturePath, t, func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()

		inbound, err := replay.ScanWSRows[BookUpdate](ctx, capturePath, public.BookChannel)

		So(err, ShouldBeNil)

		books := map[string]*BookFeedState{}
		var frames, skippedPreSnapshot, divergences int

		recordedDepth := 25

		for update := range inbound {
			if update == nil {
				continue
			}

			frames++

			state, ok := books[update.Symbol]

			if !ok {
				state = NewBookFeedState(
					update.Symbol,
					"replay-integrity",
					recordedDepth,
				)
				books[update.Symbol] = state
			}

			wasDiverged := state.Diverged()

			if !state.Apply(*update) {
				if !update.IsSnapshot() {
					skippedPreSnapshot++
				}

				continue
			}

			if state.Diverged() && !wasDiverged {
				divergences++
			}
		}

		Convey("It should drop pre-snapshot deltas without checksum divergence", func() {
			So(frames, ShouldBeGreaterThan, 0)
			So(divergences, ShouldEqual, 0)
		})

		t.Logf(
			"book replay integrity: symbols=%d frames=%d skipped_pre_snapshot=%d divergences=%d",
			len(books), frames, skippedPreSnapshot, divergences,
		)
	})
}
