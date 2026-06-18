package trader

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/logic"
	. "github.com/theapemachine/symm/signal"
)

func embeddedPlaybookBranches(testingTB testing.TB) []*logic.Branch {
	testingTB.Helper()

	pool := qpool.NewQ[any](context.Background(), 2, 4, nil)
	tree, err := logic.NewTree(context.Background(), pool)

	if err != nil {
		testingTB.Fatal(err)
	}

	if tree == nil || len(tree.Branches) == 0 {
		testingTB.Fatal("embedded playbook branches are empty")
	}

	return tree.Branches
}

func branchHasHoldingGate(branch *logic.Branch) bool {
	if branch == nil || branch.ConditionGroup == nil {
		return false
	}

	for _, condition := range branch.ConditionGroup.Conditions {
		if condition.Left.Type != logic.SubjectHolding {
			continue
		}

		if condition.Left.Holding != nil && condition.Left.Holding.Held {
			return true
		}
	}

	return false
}

func decisionTreeConnectFrame(crypto *Crypto) (map[string]any, bool) {
	for _, frame := range crypto.ConnectSnapshotFrames() {
		if frame["type"] == "decision_tree" {
			return frame, true
		}
	}

	return nil, false
}

func TestCryptoPublishDecisionTreeSnapshot(testingTB *testing.T) {
	Convey("Given a booted crypto trader", testingTB, func() {
		pool := productionPool(testingTB)

		defer pool.Close()

		received := make(chan map[string]any, 1)

		pool.Subscribe("ui", func(artifact *datura.Artifact) error {
			payload, decodeErr := qpool.ArtifactValue[map[string]any](artifact)

			if decodeErr != nil || payload["type"] != "decision_tree" {
				return nil
			}

			received <- payload

			return nil
		})

		crypto := NewCrypto(context.Background(), pool, NewTestTree())

		defer crypto.Close()

		reference := embeddedPlaybookBranches(testingTB)

		Convey("When PublishDecisionTreeSnapshot is called", func() {
			err := crypto.PublishDecisionTreeSnapshot(pool)

			Convey("It should publish embedded playbook branches", func() {
				So(err, ShouldBeNil)

				var frame map[string]any

				select {
				case frame = <-received:
				case <-time.After(2 * time.Second):
					So("ui decision tree frame", ShouldEqual, "received")
				}

				So(frame["type"], ShouldEqual, "decision_tree")

				branches, ok := frame["branches"].([]any)

				So(ok, ShouldBeTrue)
				So(len(branches), ShouldEqual, len(reference))
			})
		})
	})
}

func TestCryptoDecisionTreePublishMatchesConnectSnapshot(testingTB *testing.T) {
	Convey("Given embedded playbook wiring", testingTB, func() {
		pool := productionPool(testingTB)

		defer pool.Close()

		received := make(chan map[string]any, 1)

		pool.Subscribe("ui", func(artifact *datura.Artifact) error {
			payload, decodeErr := qpool.ArtifactValue[map[string]any](artifact)

			if decodeErr != nil || payload["type"] != "decision_tree" {
				return nil
			}

			received <- payload

			return nil
		})

		crypto := NewCrypto(context.Background(), pool, NewTestTree())

		defer crypto.Close()

		reference := embeddedPlaybookBranches(testingTB)

		So(crypto.PublishDecisionTreeSnapshot(pool), ShouldBeNil)

		var publishFrame map[string]any

		select {
		case publishFrame = <-received:
		case <-time.After(2 * time.Second):
			So("ui decision tree frame", ShouldEqual, "received")
		}

		connectFrame, found := decisionTreeConnectFrame(crypto)

		Convey("Publish and connect snapshot should expose the same playbook", func() {
			So(found, ShouldBeTrue)

			connectBranches, ok := connectFrame["branches"].([]*logic.Branch)

			So(ok, ShouldBeTrue)

			publishBranches, publishOK := publishFrame["branches"].([]any)

			So(publishOK, ShouldBeTrue)
			So(len(publishBranches), ShouldEqual, len(connectBranches))
			So(len(connectBranches), ShouldEqual, len(reference))
			So(branchHasHoldingGate(connectBranches[0]), ShouldBeTrue)
		})
	})
}

func BenchmarkPublishDecisionTreeSnapshot(b *testing.B) {
	pool := productionPool(b)

	defer pool.Close()

	pool.Subscribe("ui", func(artifact *datura.Artifact) error {
		return nil
	})

	crypto := NewCrypto(context.Background(), pool, NewTestTree())

	defer crypto.Close()

	b.ReportAllocs()

	for b.Loop() {
		if err := crypto.PublishDecisionTreeSnapshot(pool); err != nil {
			b.Fatal(err)
		}
	}
}
