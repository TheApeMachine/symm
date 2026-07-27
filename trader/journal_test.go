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
			holdings, findings, err := store.Load()

			Convey("It returns empty slices without error", func() {
				So(err, ShouldBeNil)
				So(len(holdings), ShouldEqual, 0)
				So(len(findings), ShouldEqual, 0)
			})
		})

		Convey("When saving holdings and findings", func() {
			testHoldings := []*types.Holding{
				{Symbol: "BTC/USD"},
			}
			testFindings := []types.Finding{
				{Symbol: "BTC/USD", Component: "hawkes", Condition: "elevated"},
			}

			err := store.Save(testHoldings, testFindings)
			So(err, ShouldBeNil)

			Convey("When reading the saved journal back", func() {
				loadedHoldings, loadedFindings, err := store.Load()

				Convey("It restores the saved payload", func() {
					So(err, ShouldBeNil)
					So(len(loadedHoldings), ShouldEqual, 1)
					So(loadedHoldings[0].Symbol, ShouldEqual, "BTC/USD")
					So(len(loadedFindings), ShouldEqual, 1)
					So(loadedFindings[0].Component, ShouldEqual, "hawkes")
				})
			})
		})
	})
}
