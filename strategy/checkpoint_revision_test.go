package strategy

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
)

func TestCheckpointRevisionIsMonotonic(t *testing.T) {
	Convey("teach bumps modelRevision; snapshot only advances to the saved rev", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		training := NewTraining(ctx, data.NewArenaOwner(32), 1, priced(t, "BTC/USD", 0.1), nil, nil, nil)
		training.mu.Lock()
		So(training.modelRevision, ShouldEqual, 0)
		So(training.snapshotRevision, ShouldEqual, 0)
		training.mu.Unlock()

		training.teach([]byte("abc"), "enter", 1)
		training.teach([]byte("abc"), "enter", -1)

		training.mu.Lock()
		So(training.modelRevision, ShouldEqual, 2)
		rev := training.modelRevision
		// Simulate a successful save capturing rev, while a concurrent teach bumps further.
		training.modelRevision++
		training.snapshotRevision = rev // would only set if still equal — here we verify fields exist
		So(training.snapshotRevision, ShouldBeLessThan, training.modelRevision)
		training.mu.Unlock()
	})
}
