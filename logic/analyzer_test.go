package logic

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/audit"
	"github.com/theapemachine/symm/logic/category"
	"github.com/theapemachine/symm/types"
)

type analyzerTestSolver struct {
	update func(*types.Thesis)
}

func (solver *analyzerTestSolver) Update(thesis *types.Thesis) error {
	if solver.update != nil {
		solver.update(thesis)
	}

	return nil
}

func (solver *analyzerTestSolver) Close() error {
	return nil
}

func TestAnalyzerOnSignal(t *testing.T) {
	Convey("Given an analyzer thesis gate", t, func() {
		analyzer := &Analyzer{subscribers: &sync.Map{}}
		subscription := types.NewSubscription[any]()
		analyzer.subscribers.Store("thesis", []*types.Subscription[any]{subscription})

		Convey("It should not publish partial signal input", func() {
			thesis := types.NewThesis()
			analyzer.onSignal(thesis)

			select {
			case <-subscription.Channel:
				So("published partial thesis", ShouldEqual, "")
			case <-time.After(20 * time.Millisecond):
			}
		})
	})

	Convey("Given ready signal evidence and deterministic logic solvers", t, func() {
		path := filepath.Join(t.TempDir(), "audit.jsonl")
		recorder, err := audit.NewRecorder(path)
		So(err, ShouldBeNil)

		ui := make(chan []byte, 4)
		analyzer := &Analyzer{
			ui:            ui,
			recorder:      recorder,
			categoryGraph: category.NewGraph(),
			subscribers:   &sync.Map{},
			solvers: []Solver{
				&analyzerTestSolver{update: func(thesis *types.Thesis) {
					thesis.Resonance.Store("resonance", map[string]any{"confidence": 0.8})
				}},
				&analyzerTestSolver{update: func(thesis *types.Thesis) {
					thesis.Causal.Store("BTC/USD", map[string]any{"effect": 0.4})
				}},
				&analyzerTestSolver{update: func(thesis *types.Thesis) {
					thesis.Graphs.Store("market_graph", "graph")
				}},
			},
		}
		at := time.Unix(1, 0).UTC()
		thesis := types.NewThesis()
		thesis.AppendMeasurements(
			types.SourcePumpDump,
			[]*types.Measurement{{
				Source:   types.SourcePumpDump,
				Symbol:   "BTC/USD",
				At:       at,
				Maturity: 1,
				Validity: types.MeasurementValidity{State: types.ValidityValid},
				Metrics: map[string]types.MetricSample{
					string(types.MetricIgnition): {Raw: 0.8},
				},
			}},
			types.Stamp{At: at, Entity: types.MarketTrade},
		)

		Convey("It should emit categories and logic completion with durable phases", func() {
			analyzer.onSignal(thesis)

			keys := make([]string, 0, 2)

			for range 2 {
				var frame map[string]any
				So(json.Unmarshal(<-ui, &frame), ShouldBeNil)

				for key := range frame {
					keys = append(keys, key)
				}
			}

			So(keys, ShouldContain, "categories")
			So(keys, ShouldContain, "logic")
			So(thesis.Readiness().Graph, ShouldBeTrue)

			So(recorder.Close(), ShouldBeNil)
			file, openErr := os.Open(path)
			So(openErr, ShouldBeNil)
			defer file.Close()

			scanner := bufio.NewScanner(file)
			phases := make([]string, 0, 3)

			for scanner.Scan() {
				var row map[string]any
				So(json.Unmarshal(scanner.Bytes(), &row), ShouldBeNil)

				value := row["value"].(map[string]any)
				phases = append(phases, value["phase"].(string))
			}

			So(scanner.Err(), ShouldBeNil)
			So(phases, ShouldResemble, []string{
				"categories_compose",
				"categories_commit",
				"analyze_end",
			})
		})
	})
}
