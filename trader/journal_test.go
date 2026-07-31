package trader

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
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
			thesis := types.NewThesis()
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
					So(len(loaded[0].Findings), ShouldEqual, 1)
					So(loaded[0].Findings[0].Component, ShouldEqual, "hawkes")
				})
			})
		})

		Convey("When saving more journal data than one replay budget", func() {
			payload := strings.Repeat("x", 32*1024)
			count := journalReplayByteBudget/len(payload) + 8
			theses := make([]*types.Thesis, 0, count)
			lastSymbol := ""

			for index := range count {
				symbol := fmt.Sprintf("SYM/%04d", index)
				thesis := types.NewThesis()
				decision := types.Decision{
					Symbol:           symbol,
					ProposedQuantity: decimal.NewFromInt64(1),
					PositionStatus:   types.OPEN,
					Mark:             decimal.NewFromInt64(int64(index + 1)),
				}
				decision.EnsureID()
				thesis.Decisions = append(thesis.Decisions, decision)
				thesis.Findings = append(thesis.Findings, types.Finding{
					Symbol:    symbol,
					Component: "hawkes",
					Condition: payload,
				})
				theses = append(theses, thesis)
				lastSymbol = symbol
			}

			err := store.Save(theses)
			So(err, ShouldBeNil)

			info, err := os.Stat(store.filePath)
			So(err, ShouldBeNil)

			Convey("It bounds the saved journal and replays the newest tail", func() {
				So(info.Size(), ShouldBeLessThanOrEqualTo, int64(journalReplayByteBudget+count))

				loaded, err := store.Load()
				So(err, ShouldBeNil)
				So(len(loaded), ShouldBeLessThan, len(theses))
				So(len(loaded), ShouldBeGreaterThan, 0)
				So(loaded[len(loaded)-1].Decisions[0].Symbol, ShouldEqual, lastSymbol)
			})
		})

		Convey("When saving one thesis larger than the replay budget", func() {
			thesis := types.NewThesis()
			decision := types.Decision{
				Symbol:           "OVERSIZE/USD",
				ProposedQuantity: decimal.NewFromInt64(1),
				PositionStatus:   types.OPEN,
				Mark:             decimal.NewFromInt64(42),
			}
			decision.EnsureID()
			thesis.Decisions = append(thesis.Decisions, decision)
			thesis.Findings = append(thesis.Findings, types.Finding{
				Symbol:    "OVERSIZE/USD",
				Component: "hawkes",
				Condition: strings.Repeat("z", journalReplayByteBudget),
			})

			So(store.Save([]*types.Thesis{thesis}), ShouldBeNil)

			Convey("It preserves the newest oversize snapshot instead of dropping it", func() {
				loaded, err := store.Load()
				So(err, ShouldBeNil)
				So(loaded, ShouldHaveLength, 1)
				So(loaded[0].Decisions[0].Symbol, ShouldEqual, "OVERSIZE/USD")
			})
		})
	})
}

func BenchmarkJournalStoreSaveLoad(b *testing.B) {
	tmpDir, err := os.MkdirTemp("", "journal_bench_*")
	if err != nil {
		b.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	store := &JournalStore{filePath: filepath.Join(tmpDir, "journal.json")}
	theses := make([]*types.Thesis, 0, 16)

	for index := range 16 {
		thesis := types.NewThesis()
		decision := types.Decision{
			Symbol:           fmt.Sprintf("SYM/%04d", index),
			ProposedQuantity: decimal.NewFromInt64(1),
			PositionStatus:   types.OPEN,
			Mark:             decimal.NewFromInt64(int64(index + 1)),
		}
		decision.EnsureID()
		thesis.Decisions = append(thesis.Decisions, decision)
		thesis.Findings = append(thesis.Findings, types.Finding{
			Symbol:    decision.Symbol,
			Component: "hawkes",
			Condition: strings.Repeat("x", 1024),
		})
		theses = append(theses, thesis)
	}

	

	for b.Loop() {
		if err := store.Save(theses); err != nil {
			b.Fatal(err)
		}

		if _, err := store.Load(); err != nil {
			b.Fatal(err)
		}
	}
}
