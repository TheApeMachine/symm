package trader

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/types"
)

func TestJournalStore(t *testing.T) {
	Convey("Given a temp directory for JournalStore", t, func() {
		tmpDir, err := os.MkdirTemp("", "journal_test_*")
		So(err, ShouldBeNil)
		defer os.RemoveAll(tmpDir)

		store := &JournalStore{
			filePath: filepath.Join(tmpDir, "journal.json"),
		}

		Convey("When loading an empty store", func() {
			theses, err := store.Load()

			Convey("It returns empty slices without error", func() {
				So(err, ShouldBeNil)
				So(len(theses), ShouldEqual, 0)
			})
		})

		Convey("When saving Thesis snapshots", func() {
			thesis := types.NewThesis(nil)
			decision := types.Decision{
				Symbol:           "BTC/USD",
				ProposedQuantity: decimal.NewFromInt64(1),
				PositionStatus:   types.OPEN,
				Mark:             decimal.NewFromInt64(10),
			}
			decision.EnsureID()
			thesis.Decisions = append(thesis.Decisions, decision)
			thesis.Lifecycle.Store("BTC/USD", types.LifecycleEntrySubmitted)
			thesis.Findings = append(thesis.Findings, types.Finding{
				Symbol: "BTC/USD", Component: "hawkes", Condition: "elevated",
			})

			err := store.Save([]*types.Thesis{thesis})
			So(err, ShouldBeNil)

			Convey("When reading the saved journal back", func() {
				loaded, err := store.Load()

				Convey("It restores the saved payload", func() {
					So(err, ShouldBeNil)
					So(len(loaded), ShouldEqual, 1)
					So(len(loaded[0].Decisions), ShouldEqual, 1)
					So(loaded[0].Decisions[0].Symbol, ShouldEqual, "BTC/USD")
					So(loaded[0].Decisions[0].Mark.Float64(), ShouldEqual, 10)
					lifecycle, ok := loaded[0].Lifecycle.Load("BTC/USD")
					So(ok, ShouldBeTrue)
					So(lifecycle, ShouldEqual, types.LifecycleEntrySubmitted)
					So(len(loaded[0].Findings), ShouldEqual, 1)
					So(loaded[0].Findings[0].Component, ShouldEqual, "hawkes")
				})
			})
		})
	})
}
