package trader

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/logic"
)

func TestConnectSnapshotFrames(testingTB *testing.T) {
	Convey("Given a crypto trader with a loaded playbook", testingTB, func() {
		pool := productionPool(testingTB)
		crypto := NewCrypto(context.Background(), pool)

		defer pool.Close()
		defer crypto.Close()

		Convey("It should include the decision tree on connect", func() {
			frames := crypto.ConnectSnapshotFrames()

			So(len(frames), ShouldBeGreaterThanOrEqualTo, 1)
			So(frames[0]["type"], ShouldEqual, "decision_tree")

			branches, ok := frames[0]["branches"].([]*logic.Branch)

			So(ok, ShouldBeTrue)
			So(len(branches), ShouldBeGreaterThan, 0)
		})
	})
}
