package graph

import (
	"fmt"
	"math"
	"slices"
	"strconv"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/logic/category"
	"github.com/theapemachine/symm/nomagique/learning"
	"github.com/theapemachine/symm/nomagique/transport"
	nmtypes "github.com/theapemachine/symm/nomagique/types"
	wire "github.com/theapemachine/symm/telemetry/generated/telemetry"
	"github.com/theapemachine/symm/types"
)

func graphForecast(value float64) *learning.RLSOutput {
	return &learning.RLSOutput{Value: value, Ready: true, Scale: 0.01, DegreesOfFreedom: 1}
}

func graphResonanceManifold() *learning.ResonanceManifold {
	coder := learning.NewResonanceManifold([]int{1, 2, 1}, 1, 0.1)

	for index := 0; index < 8; index++ {
		input := []float64{float64(index+1) / 10}
		_, _ = coder.SettleFromBatchOptions(input, []float64{0.01}, true, true)
	}

	return coder
}

func TestBuildGraph(t *testing.T) {
	Convey("Given a planner graph still unpublished", t, func() {
		thesis := types.NewThesis(t.Context(), nil)
		thesis.At = time.Unix(8, 0).UTC()
		symbol := thesis.Symbol("BOUNDARY/USD")
		stale := types.NewGraph(time.Unix(1, 0).UTC())
		symbol.Graphs.Push(stale)
		relationConfidence := 0.4933118837060111
		measurement := newTestMeasurement(
			"boundary-drive",
			types.SourceCVD,
			symbol.Symbol,
			thesis.At,
		)
		putTestMetric(
			measurement,
			types.MetricDrive,
			relationConfidence,
			&relationConfidence,
			nmtypes.UnitDimensionless,
		)
		measurement.Maturity = 1
		putTestMetric(
			measurement,
			types.MetricHypothesisSeparation,
			1,
			nil,
			nmtypes.UnitDimensionless,
		)
		symbol.AppendMeasurement(measurement)
		symbol.Categories.Push([]types.Category{{
			Symbol:     symbol.Symbol,
			Type:       types.CategoryAggressiveDrive,
			Supporting: []string{"cvd:drive"},
			Confidence: relationConfidence,
			Maturity:   1,
		}})
		solver := NewSolver(thesis, nil, nil)
		defer solver.Close()
		solver.buildGraph(symbol.Symbol, symbol)

		Convey("It should drain measurements and replace the unpublished graph", func() {
			So(symbol.Measurements.Length(
				symbol.MeasurementConsumers[types.MeasurementConsumerGraph],
			), ShouldEqual, uint64(0))

			var graph *Graph

			for candidate := range symbol.MarketGraphs(
				symbol.GraphConsumers[types.GraphConsumerPlanner],
			) {
				graph = candidate
			}

			So(graph, ShouldNotBeNil)
			So(graph, ShouldNotEqual, stale)
			So(graph.ReadyForSearch(), ShouldBeTrue)
		})
	})
}

func TestUpdate(t *testing.T) {
	Convey("Given a structurally ready graph below the trade-confidence floor", t, func() {
		thesis := types.NewThesis(t.Context(), nil)
		thesis.At = time.Unix(8, 0).UTC()
		symbol := thesis.Symbol("BOUNDARY/USD")
		relationConfidence := 0.4933118837060111
		measurement := newTestMeasurement(
			"boundary-drive",
			types.SourceCVD,
			symbol.Symbol,
			thesis.At,
		)
		putTestMetric(
			measurement,
			types.MetricDrive,
			relationConfidence,
			&relationConfidence,
			nmtypes.UnitDimensionless,
		)
		measurement.Maturity = 1
		putTestMetric(
			measurement,
			types.MetricHypothesisSeparation,
			1,
			nil,
			nmtypes.UnitDimensionless,
		)
		symbol.AppendMeasurement(measurement)
		symbol.Categories.Push([]types.Category{{
			Symbol:     symbol.Symbol,
			Type:       types.CategoryAggressiveDrive,
			Supporting: []string{"cvd:drive"},
			Confidence: relationConfidence,
			Maturity:   1,
		}})
		solver := NewSolver(thesis, nil, nil)
		defer solver.Close()

		solver.buildGraph(symbol.Symbol, symbol)

		Convey("It should publish the complete structure for forecast evaluation", func() {
			var graph *Graph

			for candidate := range symbol.MarketGraphs(
				symbol.GraphConsumers[types.GraphConsumerPlanner],
			) {
				graph = candidate
			}

			So(graph, ShouldNotBeNil)
			So(graph.ReadyForSearch(), ShouldBeTrue)
			So(graph.OpportunitySummary().Confidence, ShouldEqual, 1)
			So(graph.OpportunitySummary().Score, ShouldAlmostEqual, relationConfidence, 1e-12)
		})
	})

	Convey("Given a reset lifecycle with only stale category state", t, func() {
		thesis := types.NewThesis(t.Context(), nil)
		thesis.At = time.Unix(10, 0).UTC()
		symbol := thesis.Symbol("BTC/USD")
		symbol.Categories.Push([]types.Category{{
			Symbol:     "BTC/USD",
			Type:       types.CategoryAggressiveDrive,
			Supporting: []string{"cvd:drive"},
		}})
		solver := NewSolver(thesis, nil, nil)
		defer solver.Close()
		thesis.Work(types.SourceGraph).Push(symbol)

		Convey("It should wait for the first measurement of the new lifecycle", func() {
			var graph *Graph

			for candidate := range symbol.MarketGraphs(
				symbol.GraphConsumers[types.GraphConsumerPlanner],
			) {
				graph = candidate
			}

			So(graph, ShouldBeNil)
		})
	})

	Convey("Given a symbol with every category-bearing signal measurement", t, func() {
		thesis := types.NewThesis(t.Context(), nil)
		thesis.At = time.Unix(9, 0).UTC()
		symbol := thesis.Symbol("BTC/USD")
		categories := map[types.CategoryType]struct{}{}
		references := map[string]struct{}{}

		for index, schema := range types.CategorySchemas {
			value := 1 / float64(index+2)
			metricKey := types.MetricKey(schema.Metric, schema.Side)
			groups, sourceRegistered := types.SignalMetricGroups[schema.Source]
			_, metricRegistered := groups[metricKey]

			if !sourceRegistered || !metricRegistered {
				continue
			}

			// Corroborated categories share one measurement per distinct
			// source metric, exactly as the signals publish them.
			reference := string(schema.Source) + ":" + metricKey

			if _, seen := references[reference]; seen {
				categories[schema.Category] = struct{}{}
				continue
			}

			references[reference] = struct{}{}
			measurement := newTestMeasurement(
				fmt.Sprintf("%s:%s:%d", schema.Source, metricKey, index),
				schema.Source,
				"BTC/USD",
				thesis.At,
			)
			putTestMetricSide(
				measurement,
				schema.Metric,
				schema.Side,
				value,
				&value,
				nmtypes.UnitDimensionless,
			)
			measurement.Maturity = 1
			putTestMetric(
				measurement,
				types.MetricHypothesisSeparation,
				1,
				nil,
				nmtypes.UnitDimensionless,
			)
			symbol.AppendMeasurement(measurement)
			categories[schema.Category] = struct{}{}
		}

		categorySolver := category.NewSolver(t.Context(), thesis, nil, nil, nil)
		defer categorySolver.Close()
		thesis.Work(types.SourceCategory).Push(symbol)
		deadline := time.Now().Add(3 * time.Second)

		for symbol.Categories.Length(
			symbol.CategoryConsumers[types.CategoryConsumerGraph],
		) == 0 && time.Now().Before(deadline) {
			time.Sleep(time.Millisecond)
		}

		if symbol.Categories.Length(
			symbol.CategoryConsumers[types.CategoryConsumerGraph],
		) == 0 {
			t.Fatal("category solver stopped before publishing graph input")
		}

		graphSolver := NewSolver(thesis, nil, nil)
		defer graphSolver.Close()
		graphSolver.buildGraph("BTC/USD", symbol)

		Convey("It should compile a connected evidence graph rather than one winning edge", func() {
			var graph *Graph

			for candidate := range symbol.MarketGraphs(
				symbol.GraphConsumers[types.GraphConsumerPlanner],
			) {
				graph = candidate
			}

			So(graph, ShouldNotBeNil)
			incident := make(map[string]int, len(graph.Nodes))

			for _, edge := range graph.Edges {
				incident[edge.From]++
				incident[edge.To]++
			}

			for _, node := range graph.Nodes {
				So(incident[node.ID], ShouldBeGreaterThan, 0)
			}

			// Corroborated categories share supporting references with the
			// single-axis categories they extend; each ordered sharing pair
			// carries one redundant-with relation.
			sharedLinks := 0

			for _, node := range graph.Nodes {
				if node.Kind != KindCategory {
					continue
				}

				supporting, _ := node.Metadata["supporting"].([]string)

				for _, peer := range graph.Nodes {
					if peer.Kind != KindCategory || peer.ID == node.ID {
						continue
					}

					peerSupporting, _ := peer.Metadata["supporting"].([]string)

					for _, evidence := range supporting {
						if slices.Contains(peerSupporting, evidence) {
							sharedLinks++
							break
						}
					}
				}
			}

			thesisEdges := 0

			for _, edge := range graph.Edges {
				if edge.To == graph.DecisionTarget {
					thesisEdges++
				}
			}

			// In the hierarchical graph, category nodes feed the thesis decision target,
			// while raw measurements feed into category nodes.
			So(thesisEdges, ShouldEqual, len(categories))
			So(graph.Nodes, ShouldHaveLength, len(references)+len(categories)+1)
		})
	})

	Convey("Given completed upstream stages without a causal estimate", t, func() {
		thesis := types.NewThesis(t.Context(), nil)
		thesis.At = time.Unix(1, 0).UTC()
		bitcoin := thesis.Symbol("BTC/USD")
		drive := 0.8
		separation := 0.75
		bitcoinMeasurement := newTestMeasurement("cvd-measurement", types.SourceCVD, "BTC/USD", thesis.At)
		putTestMetric(bitcoinMeasurement, types.MetricDrive, drive, &drive, nmtypes.UnitDimensionless)
		putTestMetric(bitcoinMeasurement, types.MetricHypothesisSeparation, separation, &separation, nmtypes.UnitDimensionless)
		bitcoinMeasurement.Maturity = 1
		bitcoin.AppendMeasurement(bitcoinMeasurement)
		bitcoin.Categories.Push([]types.Category{{
			Symbol:     "BTC/USD",
			Type:       types.CategoryAggressiveDrive,
			Confidence: 0.6,
			Strength:   drive,
			Supporting: []string{"cvd:drive"},
		}})
		thesis.Symbol("ETH/USD")

		solver := NewSolver(thesis, nil, nil)
		defer solver.Close()
		thesis.Work(types.SourceGraph).Push(bitcoin)

		Convey("It should compile the measurement, its category, the hypothesis, and both enrichment layers", func() {
			graph := pollBuiltGraph(bitcoin, func(g *Graph) bool {
				return len(g.Nodes) > 0
			})

			So(graph, ShouldNotBeNil)
			So(graph.Nodes, ShouldHaveLength, 3)
			So(graph.Edges, ShouldHaveLength, 2)
			So(graph.Edges[0].Relation, ShouldEqual, RelationSupports)
			So(graph.Edges[0].Evidence,
				ShouldResemble, []string{"cvd-measurement", "cvd:drive"})
			So(graph.Edges[0].Quality, ShouldBeNil)
		})
	})

	Convey("Given graph evidence for the focused and unfocused pairs", t, func() {
		previousFocus := types.Focus()
		types.SetFocus("BTC/USD")
		Reset(func() { types.SetFocus(previousFocus) })
		consumer := transport.NewConsumer[*types.UIFrame](
			graphTestConsumer, func() {},
		)
		ui := transport.NewMapReduce[*types.UIFrame](
			[]*transport.Consumer[*types.UIFrame]{consumer},
			nil,
			nil,
		)
		thesis := types.NewThesis(t.Context(), ui)
		thesis.At = time.Unix(2, 0).UTC()

		for _, symbolName := range []string{"BTC/USD", "ETH/USD"} {
			symbol := thesis.Symbol(symbolName)
			drive := 0.6
			quality := 0.7
			measurement := newTestMeasurement(
				symbolName+"-measurement",
				types.SourceCVD,
				symbolName,
				thesis.At,
			)
			putTestMetric(measurement, types.MetricDrive, drive, &drive, nmtypes.UnitDimensionless)
			putTestMetric(measurement, types.MetricHypothesisSeparation, quality, &quality, nmtypes.UnitDimensionless)
			symbol.AppendMeasurement(measurement)
			symbol.Categories.Push([]types.Category{{
				Symbol: symbolName, Type: types.CategoryAggressiveDrive,
				Strength: drive, Confidence: 0.8,
				Supporting: []string{"cvd:drive"},
			}})

		}

		solver := NewSolver(thesis, nil, nil)
		defer solver.Close()
		thesis.Symbols.Range(func(_, value any) bool {
			thesis.Work(types.SourceGraph).Push(value.(*types.Symbol))
			return true
		})

		Convey("It should bootstrap and publish only the graph selected by the UI focus", func() {
			var payload *types.UIFrame

			deadline := time.Now().Add(3 * time.Second)

			for payload == nil && time.Now().Before(deadline) {
				if frame, ok := ui.Pop(consumer); ok &&
					frame.Type == wire.FrameGraphFrame {
					payload = frame
				} else {
					time.Sleep(time.Millisecond)
				}
			}

			So(payload, ShouldNotBeNil)
			So(payload.Type, ShouldEqual, wire.FrameGraphFrame)
			graph := payload.Value.(*wire.GraphFrameT)
			hasFocusedMeasurement := false
			hasSupport := false

			for _, node := range graph.Nodes {
				So(node.Id, ShouldNotContainSubstring, "ETH/USD")

				if node.Id == "meas:BTC/USD:cvd:drive" {
					hasFocusedMeasurement = true
				}
			}

			for _, edge := range graph.Edges {
				if edge.Relation == string(types.RelationSupports) {
					hasSupport = true
				}
			}

			So(hasFocusedMeasurement, ShouldBeTrue)
			So(hasSupport, ShouldBeTrue)
		})
	})
}

/*
pollBuiltGraph drains the symbol's rebuilt graphs until a graph meeting ready
appears or the deadline passes. Returns the last graph drained.
*/
func pollBuiltGraph(symbol *types.Symbol, ready func(*Graph) bool) *Graph {
	var graph *Graph

	deadline := time.Now().Add(750 * time.Millisecond)

	for time.Now().Before(deadline) {
		graph = nil

		for candidate := range symbol.MarketGraphs(
			symbol.GraphConsumers[types.GraphConsumerPlanner],
		) {
			graph = candidate
		}

		if graph != nil && ready(graph) {
			return graph
		}

		time.Sleep(time.Millisecond)
	}

	return graph
}

func TestAddEdge(t *testing.T) {
	Convey("Given an edge whose target is not a registered graph node", t, func() {
		graph := NewGraph(time.Unix(1, 0).UTC())
		graph.AddNode(&Node{ID: "source"})
		edge := &Edge{From: "source", To: "missing"}

		Convey("It should reject a dangling relationship", func() {
			So(func() { graph.AddEdge(edge) }, ShouldPanic)
			So(graph.Edges, ShouldBeEmpty)
			So(graph.Adjacency[edge.From], ShouldBeEmpty)
		})

		Convey("It should connect the edge once both nodes exist", func() {
			graph.AddNode(&Node{ID: edge.To})
			graph.AddEdge(edge)

			So(graph.Edges, ShouldHaveLength, 1)
			So(graph.Adjacency[edge.From], ShouldResemble, []string{edge.To})
		})
	})

	Convey("Given a relationship whose direction changes over its lifecycle", t, func() {
		graph := NewGraph(time.Unix(1, 0).UTC())
		graph.AddNode(&Node{ID: "forecast"})
		graph.AddNode(&Node{ID: "causal"})
		graph.AddEdge(&Edge{
			From: "forecast", To: "causal", Relation: RelationSupports,
			Weight: 0.8, Confidence: 0.9, Evidence: []string{"older"},
		})
		graph.AddEdge(&Edge{
			From: "forecast", To: "causal", Relation: RelationContradicts,
			Weight: 0.6, Confidence: 0.7, Evidence: []string{"newer"},
		})

		Convey("It should replace the obsolete claim without duplicating the target", func() {
			So(graph.Edges, ShouldHaveLength, 1)
			So(graph.Edges[0].Relation, ShouldEqual, RelationContradicts)
			So(graph.Edges[0].Evidence, ShouldResemble, []string{"newer"})
			So(graph.Adjacency["forecast"], ShouldResemble, []string{"causal"})
			weight, confidence := graph.EdgeValue("forecast", "causal")
			So(weight, ShouldEqual, -0.6)
			So(confidence, ShouldEqual, 0.7)
		})
	})
}

func TestGraphReadyForSearch(t *testing.T) {
	Convey("Given a calibrated forecast with a confidence-weighted evidence edge", t, func() {
		graph := NewGraph(time.Unix(1, 0).UTC())
		graph.Forecast = graphForecast(0.01)
		graph.ForecastHorizon = 2
		graph.DecisionTarget = "causal"
		forecastID := "res:BTC/USD:forecast"
		graph.AddNode(&Node{
			ID: forecastID, Symbol: "BTC/USD", Kind: KindResonance, Value: 0.01, Confidence: 1,
		})
		graph.AddNode(&Node{ID: "causal", Kind: KindHypothesis, Value: 0, Confidence: 1})
		graph.AddEdge(&Edge{
			From: forecastID, To: "causal", Relation: RelationSupports,
			Weight: 0.5, Confidence: 1,
		})

		Convey("The dimensionless edge should be sufficient regardless of target units", func() {
			So(graph.ReadyForSearch(), ShouldBeTrue)

			graph.Edges[0].Confidence = 0
			So(graph.ReadyForSearch(), ShouldBeFalse)
		})
	})
}

func TestGraphSearchableEnough(t *testing.T) {
	Convey("Given a decision proposition with one supporting evidence edge", t, func() {
		newGraph := func(confidence float64) *Graph {
			graph := NewGraph(time.Unix(1, 0).UTC())
			graph.DecisionTarget = "hyp:long"
			graph.AddNode(&Node{
				ID: "hyp:long", Symbol: "BTC/USD",
				Kind: KindHypothesis, Confidence: 1,
			})
			graph.AddNode(&Node{
				ID: "cat:ignition", Symbol: "BTC/USD",
				Kind: KindCategory, Value: 0.9, Strength: 0.9, Confidence: confidence,
			})
			graph.AddEdge(&Edge{
				From: "cat:ignition", To: "hyp:long",
				Relation: RelationSupports, Weight: 0.9, Confidence: confidence,
			})
			return graph
		}

		Convey("A sparse proposition below the confidence floor should defer, not search", func() {
			So(newGraph(0.1).SearchableEnough(0.5), ShouldBeFalse)
		})

		Convey("A decisive proposition at or above the floor should be searchable", func() {
			So(newGraph(0.8).SearchableEnough(0.5), ShouldBeTrue)
		})
	})

	Convey("Given a graph with no decision proposition", t, func() {
		graph := NewGraph(time.Unix(1, 0).UTC())

		Convey("It should never be searchable", func() {
			So(graph.SearchableEnough(0), ShouldBeFalse)
			So((*Graph)(nil).SearchableEnough(0), ShouldBeFalse)
		})
	})
}

func TestConnectLongOpportunity(t *testing.T) {
	Convey("Given identical evidence nodes registered in opposite orders", t, func() {
		at := time.Unix(3, 0).UTC()
		symbol := types.NewSymbol("ORDER/USD")
		categoryTypes := []types.CategoryType{
			types.AggressiveDrive,
			types.MechanicalCollapse,
			types.DenseNeutrality,
		}
		nodes := make([]*Node, 0, 48)

		for index := range 48 {
			categoryType := categoryTypes[index%len(categoryTypes)]
			nodes = append(nodes, &Node{
				ID:         fmt.Sprintf("cat:ORDER/USD:%02d", index),
				Symbol:     symbol.Symbol,
				Kind:       KindCategory,
				Value:      float64(index+1) / float64(index%7+2),
				Strength:   float64(index+1) / float64(index%7+2),
				Confidence: float64((index*17)%47+1) / 48,
				At:         at,
				Metadata: map[string]any{
					"type": string(categoryType),
				},
			})
		}

		buildGraph := func(reverse bool) *Graph {
			graph := NewGraph(at)

			if reverse {
				for index := len(nodes) - 1; index >= 0; index-- {
					graph.AddNode(nodes[index])
				}
			} else {
				for _, node := range nodes {
					graph.AddNode(node)
				}
			}

			err := (&Solver{}).connectLongOpportunity(symbol, graph)
			So(err, ShouldBeNil)

			return graph
		}

		forward := buildGraph(false)
		reverse := buildGraph(true)

		Convey("It should produce bit-identical ordered evidence and reductions", func() {
			So(forward.Edges, ShouldResemble, reverse.Edges)
			forwardSummary := forward.OpportunitySummary()
			reverseSummary := reverse.OpportunitySummary()
			So(forwardSummary.Hypothesis, ShouldEqual, reverseSummary.Hypothesis)
			So(math.Float64bits(forwardSummary.Support), ShouldEqual,
				math.Float64bits(reverseSummary.Support))
			So(math.Float64bits(forwardSummary.Contradiction), ShouldEqual,
				math.Float64bits(reverseSummary.Contradiction))
			So(math.Float64bits(forwardSummary.Conditions), ShouldEqual,
				math.Float64bits(reverseSummary.Conditions))
			So(math.Float64bits(forwardSummary.Balance), ShouldEqual,
				math.Float64bits(reverseSummary.Balance))
			So(math.Float64bits(forwardSummary.Confidence), ShouldEqual,
				math.Float64bits(reverseSummary.Confidence))
			So(math.Float64bits(forwardSummary.Score), ShouldEqual,
				math.Float64bits(reverseSummary.Score))
			So(math.Float64bits(forwardSummary.Direction), ShouldEqual,
				math.Float64bits(reverseSummary.Direction))
			So(forwardSummary.Ready, ShouldEqual, reverseSummary.Ready)
		})
	})

	Convey("Given a category node at the admission boundary", t, func() {
		at := time.Unix(1, 0).UTC()
		symbol := types.NewSymbol("BTC/USD")
		drive := 0.8
		separation := 0.9
		graph := NewGraph(at)
		categoryNode := &types.Node{
			ID:         "cat:BTC/USD:aggressive_drive",
			Symbol:     symbol.Symbol,
			Kind:       KindCategory,
			Value:      drive,
			Confidence: 0.9 * 0.9,
			Maturity:   0.9,
			At:         at,
			Metadata: map[string]any{
				"type":                  string(types.CategoryAggressiveDrive),
				"hypothesis_separation": separation,
			},
		}
		graph.AddNode(categoryNode)

		err := (&Solver{}).connectLongOpportunity(symbol, graph)

		Convey("It should weight the category vote by maturity, separation, and magnitude", func() {
			So(err, ShouldBeNil)
			driveNode := graph.Nodes[categoryNode.ID]
			So(driveNode, ShouldNotBeNil)
			So(driveNode.Confidence, ShouldAlmostEqual, 0.9*0.9, 1e-12)
			So(driveNode.Maturity, ShouldEqual, 0.9)
			So(driveNode.Metadata["hypothesis_separation"], ShouldEqual, separation)

			var directional *Edge

			for _, edge := range graph.Edges {
				if edge.From == driveNode.ID && edge.To == graph.DecisionTarget {
					directional = edge
					break
				}
			}

			So(directional, ShouldNotBeNil)
			So(directional.Relation, ShouldEqual, RelationSupports)
			So(directional.Weight, ShouldAlmostEqual, 0.9*0.9, 1e-12)
			So(directional.Confidence, ShouldAlmostEqual, 0.9*0.9, 1e-12)
			So(graph.OpportunitySummary().Confidence, ShouldAlmostEqual, 0.9*0.9, 1e-12)
			So(graph.SearchableEnough(0.5), ShouldBeTrue)
		})
	})

	Convey("Given a raw measurement without a category or causal synthesis node", t, func() {
		at := time.Unix(2, 0).UTC()
		symbol := types.NewSymbol("BTC/USD")
		measurement := newTestMeasurement(
			"cvd-raw", types.SourceCVD, symbol.Symbol, at,
		)
		putTestMetric(
			measurement,
			types.MetricDrive,
			0.8,
			nil,
			nmtypes.UnitDimensionless,
		)
		symbol.AppendMeasurement(measurement)
		graph := NewGraph(at)
		_, err := newMeasurementCompiler().addNodes(
			symbol.Symbol,
			symbol.MarketMeasurements(
				symbol.MeasurementConsumers[types.MeasurementConsumerGraph],
			),
			graph,
		)
		So(err, ShouldBeNil)

		err = (&Solver{}).connectLongOpportunity(symbol, graph)

		Convey("It should keep the raw metric in the graph but not bypass the category layer to decision target", func() {
			So(err, ShouldBeNil)
			So(graph.Nodes[measurementNodeID(measurement, "drive")], ShouldNotBeNil)
			So(graph.Edges, ShouldBeEmpty)
			So(graph.SearchableEnough(0.5), ShouldBeFalse)
		})
	})

	Convey("Given a Pearl causal do-expectation node", t, func() {
		at := time.Unix(3, 0).UTC()
		symbol := types.NewSymbol("BTC/USD")
		graph := NewGraph(at)
		causalNode := &types.Node{
			ID:         "causal:BTC/USD:doExpectation",
			Symbol:     symbol.Symbol,
			Kind:       KindCausal,
			Value:      0.7,
			Strength:   0.8,
			Confidence: 0.8 * 0.9,
			Maturity:   0.8,
			At:         at,
			Metadata: map[string]any{
				"hypothesis_separation": 0.9,
			},
		}
		graph.AddNode(causalNode)

		err := (&Solver{}).connectLongOpportunity(symbol, graph)

		Convey("It should vote directly on the decision target", func() {
			So(err, ShouldBeNil)
			targetNode := graph.Nodes[causalNode.ID]
			So(targetNode, ShouldNotBeNil)
			So(targetNode.Value, ShouldEqual, 0.7)
			So(targetNode.Confidence, ShouldAlmostEqual, 0.8*0.9, 1e-12)

			var directional *Edge

			for _, edge := range graph.Edges {
				if edge.From == targetNode.ID && edge.To == graph.DecisionTarget {
					directional = edge
					break
				}
			}

			So(directional, ShouldNotBeNil)
			So(directional.Relation, ShouldEqual, RelationSupports)
			So(directional.Weight, ShouldAlmostEqual, 0.8*0.9, 1e-12)
			So(graph.OpportunitySummary().Ready, ShouldBeTrue)
		})
	})
}

func TestAddNode(t *testing.T) {
	Convey("Given a node without an identity", t, func() {
		graph := NewGraph(time.Unix(1, 0).UTC())

		Convey("It should reject the node instead of silently losing it", func() {
			So(func() { graph.AddNode(&Node{}) }, ShouldPanic)
			So(graph.Nodes, ShouldBeEmpty)
		})
	})
}

func TestInferStructuralEdges(t *testing.T) {
	Convey("Given categories with conflicting evidence", t, func() {
		at := time.Unix(1, 0).UTC()
		thesis := types.NewThesis(t.Context(), nil)
		symbol := types.NewSymbol("BTC/USD")
		symbol.Categories.Push([]types.Category{
			{
				Type:       types.CategoryAggressiveDrive,
				Strength:   0.8,
				Confidence: 0.9,
				Supporting: []string{"drive"},
			},
			{
				Type:       types.CategoryExhaustion,
				Strength:   0.6,
				Confidence: 0.7,
				Opposing:   []string{"drive"},
			},
		})
		thesis.Symbols.Store("BTC/USD", symbol)
		graph := NewGraph(at)
		solver := NewSolver(thesis, nil, nil)
		categories := solver.popCategories(symbol)
		solver.extractCategoryNodes(symbol, categories, graph)

		err := solver.inferStructuralEdges(
			symbol, categories, types.Cognition{}, graph,
		)

		Convey("It connects the category hypotheses directly", func() {
			So(err, ShouldBeNil)
			So(graph.Edges, ShouldHaveLength, 1)
			So(graph.Edges[0].Relation, ShouldEqual, RelationContradicts)
			So(graph.Edges[0].From, ShouldEqual, "cat:BTC/USD:aggressive_drive")
			So(graph.Edges[0].To, ShouldEqual, "cat:BTC/USD:exhaustion")
		})
	})

	Convey("Given canonical causal intervention and expectation nodes", t, func() {
		at := time.Unix(1, 0).UTC()
		graph := NewGraph(at)
		solver := NewSolver(types.NewThesis(t.Context(), nil), nil, nil)

		for _, symbol := range []string{"BTC/USD", "ETH/USD"} {
			graph.AddNode(&Node{
				ID:         "causal:" + symbol + ":intervention",
				Symbol:     symbol,
				Kind:       KindCausal,
				Value:      0.5,
				Strength:   2301.376798,
				Confidence: 0.58,
				At:         at,
			})
			graph.AddNode(&Node{
				ID:         "causal:" + symbol + ":doExpectation",
				Symbol:     symbol,
				Kind:       KindCausal,
				Strength:   2301.376798,
				Confidence: 0.58,
				At:         at,
			})
		}

		graph.AddNode(&Node{
			ID:         "causal:BTC/USD:not_intervention",
			Symbol:     "BTC/USD",
			Kind:       KindCausal,
			Confidence: 1,
			At:         at,
		})

		err := solver.inferStructuralEdges(
			types.NewSymbol("BTC/USD"), nil, types.Cognition{}, graph,
		)
		So(err, ShouldBeNil)

		Convey("Each intervention should condition its symbol's do-expectation", func() {
			conditionEdges := 0

			for _, edge := range graph.Edges {
				if edge.Relation == RelationConditions {
					conditionEdges++
					So(graph.Nodes[edge.From].Symbol,
						ShouldEqual, graph.Nodes[edge.To].Symbol)
					So(edge.Weight, ShouldBeGreaterThan, 0)
					So(edge.Weight, ShouldBeLessThan, 1)
					So(edge.Confidence, ShouldBeLessThan, 1)
				}
			}

			So(conditionEdges, ShouldEqual, 2)
		})
	})

	Convey("Given unrelated graph nodes", t, func() {
		at := time.Unix(10, 0).UTC()
		graph := NewGraph(at)
		solver := NewSolver(types.NewThesis(t.Context(), nil), nil, nil)

		for index := 0; index < 256; index++ {
			nodeID := "cat:BTC/USD:inactive:" + strconv.Itoa(index)
			graph.AddNode(&Node{
				ID:     nodeID,
				Symbol: "BTC/USD",
				Kind:   KindCategory,
				At:     at.Add(-time.Duration(index) * time.Second),
			})
		}

		graph.AddNode(&Node{
			ID:         "res:BTC/USD:forecast",
			Symbol:     "BTC/USD",
			Kind:       KindResonance,
			Value:      0.5,
			Confidence: 0.8,
			At:         at,
		})
		graph.AddNode(&Node{
			ID:         "causal:BTC/USD:uplift",
			Symbol:     "BTC/USD",
			Kind:       KindCausal,
			Value:      0.25,
			Confidence: 0.5,
			At:         at,
		})

		err := solver.inferStructuralEdges(
			types.NewSymbol("BTC/USD"), nil, types.Cognition{}, graph,
		)
		So(err, ShouldBeNil)

		Convey("It omits zero-confidence pair edges while retaining evidence-bearing edges", func() {
			counts := make(map[RelationType]int)

			for _, edge := range graph.Edges {
				counts[edge.Relation]++
			}

			So(counts[RelationStaleRelativeTo], ShouldEqual, 0)
			So(counts[RelationIncomparableWith], ShouldEqual, 0)
			So(counts[RelationSupports], ShouldEqual, 1)
			So(len(graph.Edges), ShouldEqual, 1)
		})
	})

	Convey("Given a cognition winner with a measured lookahead path", t, func() {
		at := time.Unix(20, 0).UTC()
		symbol := types.NewSymbol("BTC/USD")
		symbol.Cognition.Push(types.Cognition{
			Source:     "cognition",
			Symbol:     "BTC/USD",
			At:         at,
			Winner:     "concept_1",
			Confidence: 0.8,
			Predictions: map[string]float64{
				"active_reversal": 0.7,
				"unsupported":     0,
			},
		})
		graph := NewGraph(at)
		solver := NewSolver(types.NewThesis(t.Context(), nil), nil, nil)
		cognition, found := solver.popCognition(symbol)
		So(found, ShouldBeTrue)
		solver.extractCognitionNodes(symbol, cognition, graph)

		err := solver.inferStructuralEdges(symbol, nil, cognition, graph)

		Convey("It should retain the prediction endpoints and temporal relations", func() {
			So(err, ShouldBeNil)
			So(graph.Nodes, ShouldHaveLength, 2)
			So(graph.Edges, ShouldHaveLength, 2)
			So(graph.Edges[0].Relation, ShouldEqual, RelationLeads)
			So(graph.Edges[1].Relation, ShouldEqual, RelationLags)
			So(graph.Edges[0].Weight, ShouldEqual, 0.7)
			So(graph.Edges[0].To,
				ShouldEqual, "cog:BTC/USD:prediction:active_reversal")
		})
	})
}

func TestExtractCausalNodes(t *testing.T) {
	Convey("Given causal outputs with channel confidence and sample precision", t, func() {
		at := time.Unix(1, 0).UTC()
		thesis := types.NewThesis(t.Context(), nil)
		thesis.At = at
		symbol := types.NewSymbol("BTC/USD")
		symbol.Causal.Push(map[string]any{
			"association":       0.1,
			"associationScore":  0.2,
			"doExpectation":     0.25,
			"interventionScore": 0.4,
			"precision":         0.5,
			"probabilities":     []float64{0.12, 0.58, 0.2, 0.1},
		})
		thesis.Symbols.Store("BTC/USD", symbol)
		graph := NewGraph(at)
		solver := NewSolver(thesis, nil, nil)

		err := solver.extractCausalNodes(symbol, graph)
		So(err, ShouldBeNil)

		Convey("It should attenuate channel confidence by estimator precision", func() {
			node, found := graph.Nodes["causal:BTC/USD:doExpectation"]
			association := graph.Nodes["causal:BTC/USD:association"]

			So(found, ShouldBeTrue)
			So(node.Confidence, ShouldAlmostEqual, 0.29)
			So(node.Strength, ShouldEqual, 0.4)
			So(node.At, ShouldEqual, at)
			So(node.Metadata["horizon"], ShouldEqual, 1)
			So(association.Confidence, ShouldAlmostEqual, 0.06)
			So(association.Confidence, ShouldNotEqual, node.Confidence)
		})
	})

	Convey("Given a causal value without its probability distribution", t, func() {
		thesis := types.NewThesis(t.Context(), nil)
		symbol := types.NewSymbol("BTC/USD")
		symbol.Causal.Push(map[string]any{"intervention": 1.0})
		thesis.Symbols.Store("BTC/USD", symbol)

		err := NewSolver(thesis, nil, nil).extractCausalNodes(
			symbol,
			NewGraph(time.Unix(1, 0).UTC()),
		)

		Convey("It should reject confidence-free causal nodes", func() {
			So(err, ShouldNotBeNil)
		})
	})
}

func TestExtractResonanceNodes(t *testing.T) {
	Convey("Given a confidence-supported multi-step resonance forecast", t, func() {
		at := time.Unix(1, 0).UTC()
		thesis := types.NewThesis(t.Context(), nil)
		thesis.At = at
		symbol := types.NewSymbol("BTC/USD")
		symbol.Resonance.Push(graphResonanceManifold())
		symbol.Resonance.Push(
			&types.ResonanceReturnForecast{
				Distribution: learning.RLSOutput{
					Value:            0.012,
					Scale:            0.002,
					DegreesOfFreedom: 8,
					Ready:            true,
				},
				Horizon: 3,
				Call:    1,
			},
		)
		thesis.Symbols.Store("BTC/USD", symbol)
		graph := NewGraph(at)
		solver := NewSolver(thesis, nil, nil)

		solver.extractResonanceNodes(symbol, graph)

		Convey("It should publish the direction call, not a priced return path", func() {
			node, found := graph.Nodes["res:BTC/USD:forecast"]

			So(found, ShouldBeTrue)
			So(node.Value, ShouldEqual, 1)
			So(node.Confidence, ShouldAlmostEqual, 1, 0.001)
			So(node.At, ShouldEqual, at)
			So(graph.ForecastHorizon, ShouldEqual, 3)
			So(graph.ForwardCurve, ShouldBeEmpty)
		})
	})
}

func TestExtractPredictiveDynamicsNodes(t *testing.T) {
	Convey("Given committed continuous predictive dynamics", t, func() {
		at := time.Unix(2, 0).UTC()
		symbol := types.NewSymbol("BTC/USD")
		symbol.Resonance.Push(graphResonanceManifold())
		dynamics := nmtypes.Frame{}
		dynamics.Put(learning.SymbolDynamicsReady, 1)
		dynamics.Put(learning.SymbolDynamicsSampleCount, 8)
		dynamics.Put(learning.SymbolDynamicsVelocity, 0.4)
		dynamics.Put(learning.SymbolDynamicsMemory, 0.3)
		dynamics.Put(learning.SymbolDynamicsPassivityResidue, -0.1)
		dynamics.Put(learning.SymbolDynamicsJumpVariance, 0.02)
		symbol.Resonance.Push(dynamics)
		graph := NewGraph(at)

		NewSolver(
			types.NewThesis(t.Context(), nil), nil, nil,
		).extractResonanceNodes(symbol, graph)

		Convey("It should expose motion and risk as inspectable graph evidence", func() {
			velocity, hasVelocity := graph.Nodes["res:BTC/USD:generalized_velocity"]
			residue, hasResidue := graph.Nodes["res:BTC/USD:passivity_residue"]
			jumpVariance, hasJumpVariance := graph.Nodes["res:BTC/USD:jump_variance"]

			So(hasVelocity, ShouldBeTrue)
			So(velocity.Value, ShouldEqual, 0.4)
			So(hasResidue, ShouldBeTrue)
			So(residue.Value, ShouldEqual, -0.1)
			So(hasJumpVariance, ShouldBeTrue)
			So(jumpVariance.Value, ShouldEqual, 0.02)
		})
	})
}

func TestExtractManifoldNodes(t *testing.T) {
	Convey("Given a ready universe phase alignment", t, func() {
		at := time.Unix(1, 0).UTC()
		thesis := types.NewThesis(t.Context(), nil)
		thesis.StorePhase(types.PhaseReading{
			At:    at,
			Ready: true,
			Responses: []types.PhaseResponse{{
				Angle: 0.75, Similarity: 0.6, ObservedAt: at.Add(-time.Second).Format(time.RFC3339),
				Outcome: types.PhaseOutcome{
					Direction: "up", Return: 0.01, Horizon: 2,
				},
			}},
		})
		graph := NewGraph(at)

		NewSolver(thesis, nil, nil).extractManifoldNodes(thesis, graph)

		Convey("It should retain the measured phase and directional inference", func() {
			node := graph.Nodes["man:universe:phase_direction"]
			So(node, ShouldNotBeNil)
			So(node.Kind, ShouldEqual, KindManifold)
			So(node.Value, ShouldEqual, 1.0)
			So(node.Confidence, ShouldEqual, 1.0)
			So(node.Metadata["support"], ShouldEqual, 0.6)
			So(node.Metadata["responses"], ShouldEqual, 1)
		})
	})
}

func TestMagnitudeWeight(t *testing.T) {
	Convey("Given the large finite causal magnitude observed on the live graph", t, func() {
		weight, err := magnitudeWeight(2301.376798)

		Convey("It should remain strong without becoming certain", func() {
			So(err, ShouldBeNil)
			So(weight, ShouldBeGreaterThan, 0)
			So(weight, ShouldBeLessThan, 1)
		})
	})

	Convey("Given the largest representable finite strength", t, func() {
		weight, err := magnitudeWeight(math.MaxFloat64)

		Convey("It should preserve the open probability bound", func() {
			So(err, ShouldBeNil)
			So(weight, ShouldBeLessThan, 1)
		})
	})
}

func TestAgreementWeight(t *testing.T) {
	Convey("Given two large finite readings on unrelated scales", t, func() {
		weight, err := agreementWeight(2301.376798, -9172.4)

		Convey("It should preserve strong agreement without saturating", func() {
			So(err, ShouldBeNil)
			So(weight, ShouldBeGreaterThan, 0)
			So(weight, ShouldBeLessThan, 1)
		})
	})
}

func BenchmarkInferStructuralEdges(b *testing.B) {
	at := time.Unix(1, 0).UTC()
	nodes := make(map[string]*Node, 260)

	for index := range 256 {
		nodeID := "cat:BTC/USD:inactive:" + strconv.Itoa(index)
		nodes[nodeID] = &Node{
			ID:         nodeID,
			Symbol:     "BTC/USD",
			Kind:       KindCategory,
			Confidence: 1,
			At:         at,
		}
	}

	for _, symbol := range []string{"BTC/USD", "ETH/USD"} {
		interventionID := "causal:" + symbol + ":intervention"
		nodes[interventionID] = &Node{
			ID:         interventionID,
			Symbol:     symbol,
			Kind:       KindCausal,
			Value:      0.5,
			Strength:   0.5,
			Confidence: 1,
			At:         at,
		}

		expectationID := "causal:" + symbol + ":doExpectation"
		nodes[expectationID] = &Node{
			ID:         expectationID,
			Symbol:     symbol,
			Kind:       KindCausal,
			Confidence: 1,
			At:         at,
		}
	}

	solver := NewSolver(types.NewThesis(b.Context(), nil), nil, nil)
	symbol := types.NewSymbol("BTC/USD")

	b.ReportAllocs()

	for b.Loop() {
		graph := NewGraph(at)
		graph.Nodes = nodes
		if err := solver.inferStructuralEdges(
			symbol, nil, types.Cognition{}, graph,
		); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkConnectLongOpportunity(b *testing.B) {
	at := time.Unix(1, 0).UTC()
	symbol := types.NewSymbol("BENCH/USD")
	categoryTypes := []types.CategoryType{
		types.AggressiveDrive,
		types.MechanicalCollapse,
		types.DenseNeutrality,
	}
	nodes := make([]*Node, 0, 64)

	for index := range 64 {
		categoryType := categoryTypes[index%len(categoryTypes)]
		nodes = append(nodes, &Node{
			ID:         fmt.Sprintf("cat:BENCH/USD:%02d", index),
			Symbol:     symbol.Symbol,
			Kind:       KindCategory,
			Value:      float64(index+1) / float64(index%7+2),
			Strength:   float64(index+1) / float64(index%7+2),
			Confidence: float64((index*17)%63+1) / 64,
			At:         at,
			Metadata: map[string]any{
				"type": string(categoryType),
			},
		})
	}

	solver := &Solver{}
	b.ReportAllocs()

	for b.Loop() {
		graph := NewGraph(at)

		for _, node := range nodes {
			graph.AddNode(node)
		}

		if err := solver.connectLongOpportunity(symbol, graph); err != nil {
			b.Fatal(err)
		}

		if !graph.OpportunitySummary().Ready {
			b.Fatal("decision summary is not ready")
		}
	}
}

func BenchmarkUpdate(b *testing.B) {
	at := time.Unix(1, 0).UTC()
	thesis := types.NewThesis(b.Context(), nil)
	thesis.At = at
	forecast := graphForecast(0.01)

	for index := range 256 {
		symbol := "SIM" + strconv.Itoa(index) + "/USD"
		symbolState := types.NewSymbol(symbol)
		separation := 0.8
		surge := 0.5
		sentiment := newTestMeasurement(
			"sentiment-"+symbol,
			types.SourceSentiment,
			symbol,
			at,
		)
		putTestMetric(sentiment, types.MetricSurgeScore, surge, &surge, nmtypes.UnitDimensionless)
		putTestMetric(sentiment, types.MetricHypothesisSeparation, separation, &separation, nmtypes.UnitDimensionless)
		symbolState.AppendMeasurement(sentiment)
		thesis.Symbols.Store(symbol, symbolState)

		symbolState.Categories.Push([]types.Category{
			{
				Symbol:     symbol,
				Type:       types.CategoryRiskOnSurge,
				Confidence: 0.8,
				Strength:   0.5,
				Supporting: []string{"sentiment:surge_score"},
			},
		})
		coder := learning.NewResonanceManifold([]int{1, 2, 1}, 1, 0.1)

		for sample := range 8 {
			input := []float64{float64(sample+1) / 10}
			_, _ = coder.SettleFromBatchOptions(input, []float64{forecast.Value}, true, true)
		}

		symbolState.Resonance.Push(coder)
		symbolState.Causal.Push(map[string]any{
			"association":       0.1,
			"associationScore":  0.1,
			"doExpectation":     0.1,
			"intervention":      0.1,
			"interventionScore": 0.1,
			"precision":         0.8,
			"probabilities":     []float64{0.3, 0.4, 0.2, 0.1},
			"uplift":            0.1,
			"upliftScore":       0.1,
		})
		symbolState.Cognition.Push(types.Cognition{
			Symbol:     symbol,
			Source:     "cognition",
			At:         at,
			Winner:     string(types.CategoryRiskOnSurge),
			Confidence: 0.8,
		})
	}

	consumer := transport.NewConsumer[*types.UIFrame](
		graphTestConsumer, func() {},
	)
	ui := transport.NewMapReduce[*types.UIFrame](
		[]*transport.Consumer[*types.UIFrame]{consumer}, nil, nil,
	)
	solver := NewSolver(thesis, ui, nil)
	previousFocus := types.Focus()
	types.SetFocus("SIM0/USD")
	defer types.SetFocus(previousFocus)

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		thesis.Symbols.Range(func(_, value any) bool {
			symbolState, _ := value.(*types.Symbol)

			if symbolState != nil {
				solver.buildGraph(symbolState.Symbol, symbolState)
			}

			_, _ = ui.Pop(consumer)

			return true
		})
	}
}

func allSymbols(thesis *types.Thesis) []*types.Symbol {
	var symbols []*types.Symbol

	thesis.Symbols.Range(func(_, value any) bool {
		if symbolState, ok := value.(*types.Symbol); ok && symbolState != nil {
			symbols = append(symbols, symbolState)
		}

		return true
	})

	return symbols
}

func TestHeldCognitionGraphEvidence(t *testing.T) {
	Convey("Given a hysteretically held cognition reading", t, func() {
		at := time.Unix(1, 0).UTC()
		symbol := types.NewSymbol("BTC/USD")
		symbol.Cognition.Push(types.Cognition{
			Source:           "cognition",
			Symbol:           symbol.Symbol,
			At:               at,
			Winner:           "trend",
			CandidateWinner:  "drive",
			Confidence:       0.2,
			StateHeld:        true,
			PredictionsHeld:  true,
			SwitchConfidence: 0.8,
			SwitchThreshold:  0.95,
			Predictions: map[string]float64{
				"aggressive_drive": 0.8,
			},
		})
		graph := NewGraph(at)
		solver := NewSolver(types.NewThesis(t.Context(), nil), nil, nil)
		cognition, found := solver.popCognition(symbol)
		So(found, ShouldBeTrue)

		solver.extractCognitionNodes(symbol, cognition, graph)

		Convey("It should retain audit state without exposing a root or lookahead", func() {
			winner, found := graph.Nodes["cog:BTC/USD:winner_regime"]
			So(found, ShouldBeTrue)
			So(winner.Metadata["held"], ShouldEqual, true)
			So(graph.Nodes, ShouldHaveLength, 1)
			So(graph.Roots(), ShouldBeEmpty)
		})
	})
}

const graphTestConsumer = "graph-test"
