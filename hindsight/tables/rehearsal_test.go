package tables_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/hindsight/tables"
	"github.com/theapemachine/symm/tests/tablestest"
	"github.com/theapemachine/symm/types"
)

type testTape struct {
	mu           sync.Mutex
	fragments    []types.ReplayFragment
	closed       atomic.Bool
	budget       atomic.Uint64
	observations atomic.Uint64
	runs         atomic.Uint64
}

func newTestTape() *testTape {
	return &testTape{}
}

func (t *testTape) Publish(fragment types.ReplayFragment) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.fragments = append(t.fragments, fragment)
}

func (t *testTape) Close() {
	t.closed.Store(true)
}

func (t *testTape) SetBudget(budget uint64) {
	t.budget.Store(budget)
}

func (t *testTape) AddObservations(count uint64) {
	t.observations.Add(count)
}

func (t *testTape) AddRuns(count uint64) {
	t.runs.Add(count)
}

func TestLoadRehearsalTape(t *testing.T) {
	Convey("Given an Iceberg catalog with historical ticks", t, func() {
		ctx := context.Background()
		catalog := tablestest.New(t)
		writer := tables.NewWriter(catalog)

		epoch := int64(100)
		now := time.Date(2026, 9, 14, 10, 0, 0, 0, time.UTC)

		// Populate 16 ticker ticks for BTC/USD
		for i := int64(1); i <= 16; i++ {
			writer.AddSpotTicker(tables.SpotTickerRow{
				Epoch:      epoch,
				Tick:       i,
				Symbol:     "BTC/USD",
				VenueAt:    now.Add(time.Duration(i) * time.Second),
				ReceivedAt: now.Add(time.Duration(i) * time.Second),
				Bid:        50000.0 + float64(i*10),
				Ask:        50001.0 + float64(i*10),
				Last:       50000.5 + float64(i*10),
			})
		}

		err := writer.Commit(ctx)
		So(err, ShouldBeNil)

		tape := newTestTape()

		Convey("When LoadRehearsalTape is called", func() {
			err := catalog.LoadRehearsalTape(ctx, tape)
			So(err, ShouldBeNil)

			Convey("Then tape fragments are discovered and published", func() {
				So(tape.closed.Load(), ShouldBeTrue)
				So(tape.runs.Load(), ShouldEqual, 1)
				So(tape.observations.Load(), ShouldBeGreaterThan, 0)

				tape.mu.Lock()
				defer tape.mu.Unlock()
				So(len(tape.fragments), ShouldBeGreaterThan, 0)

				fragment := tape.fragments[0]
				So(fragment.Symbol, ShouldEqual, "BTC/USD")
				So(len(fragment.Frames), ShouldBeGreaterThan, 0)
				So(fragment.AnchorIndex, ShouldBeLessThan, len(fragment.Frames))
				So(fragment.ExtremumIndex, ShouldBeGreaterThanOrEqualTo, fragment.AnchorIndex)
			})
		})

		Convey("When multiple symbols exist including flat stagnant quotes", func() {
			writerMulti := tables.NewWriter(catalog)
			epochMulti := int64(200)

			// 16 moving ticks for BTC/USD
			for tickIdx := int64(1); tickIdx <= 16; tickIdx++ {
				writerMulti.AddSpotTicker(tables.SpotTickerRow{
					Epoch:      epochMulti,
					Tick:       tickIdx,
					Symbol:     "BTC/USD",
					VenueAt:    now.Add(time.Duration(tickIdx) * time.Second),
					ReceivedAt: now.Add(time.Duration(tickIdx) * time.Second),
					Bid:        60000.0 + float64(tickIdx*5),
					Ask:        60001.0 + float64(tickIdx*5),
					Last:       60000.5 + float64(tickIdx*5),
				})
			}

			// 16 moving ticks for ETH/USD
			for tickIdx := int64(1); tickIdx <= 16; tickIdx++ {
				writerMulti.AddSpotTicker(tables.SpotTickerRow{
					Epoch:      epochMulti,
					Tick:       tickIdx,
					Symbol:     "ETH/USD",
					VenueAt:    now.Add(time.Duration(tickIdx) * time.Second),
					ReceivedAt: now.Add(time.Duration(tickIdx) * time.Second),
					Bid:        3000.0 + float64(tickIdx*2),
					Ask:        3001.0 + float64(tickIdx*2),
					Last:       3000.5 + float64(tickIdx*2),
				})
			}

			// 16 flat stagnant ticks for IAG/USD (constant price)
			for tickIdx := int64(1); tickIdx <= 16; tickIdx++ {
				writerMulti.AddSpotTicker(tables.SpotTickerRow{
					Epoch:      epochMulti,
					Tick:       tickIdx,
					Symbol:     "IAG/USD",
					VenueAt:    now.Add(time.Duration(tickIdx) * time.Second),
					ReceivedAt: now.Add(time.Duration(tickIdx) * time.Second),
					Bid:        0.05,
					Ask:        0.05,
					Last:       0.05,
				})
			}

			errMulti := writerMulti.Commit(ctx)
			So(errMulti, ShouldBeNil)

			tapeMulti := newTestTape()
			errLoad := catalog.LoadRehearsalTape(ctx, tapeMulti)
			So(errLoad, ShouldBeNil)

			tapeMulti.mu.Lock()
			defer tapeMulti.mu.Unlock()

			Convey("Then moving symbols are discovered and flat stagnant tickers do not starve the tape", func() {
				symbolsFound := make(map[string]bool)

				for _, frag := range tapeMulti.fragments {
					symbolsFound[frag.Symbol] = true
				}

				So(symbolsFound["BTC/USD"], ShouldBeTrue)
				So(symbolsFound["ETH/USD"], ShouldBeTrue)
				// Flat stagnant symbol should not be published when moving symbols exist
				So(symbolsFound["IAG/USD"], ShouldBeFalse)
			})
		})
	})
}
