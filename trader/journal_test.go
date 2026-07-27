package trader

import (
	"os"
	"path/filepath"
	"testing"

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
			thesis := types.NewThesis()
			thesis.Holdings.Store("BTC/USD", &types.Holding{Symbol: "BTC/USD"})
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
					value, ok := loaded[0].Holdings.Load("BTC/USD")
					So(ok, ShouldBeTrue)
					So(value.(*types.Holding).Symbol, ShouldEqual, "BTC/USD")
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
