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
	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/learning"
	nmtypes "github.com/theapemachine/symm/nomagique/types"
	"github.com/theapemachine/symm/types"
)

func graphForecast(value float64) *learning.RLSOutput {
	return &learning.RLSOutput{Value: value, Ready: true, Scale: 0.01, DegreesOfFreedom: 1}
}

func graphResonanceManifold() *learning.ResonanceManifold {
	coder := learning.NewResonanceManifold([]int{1, 2, 1}, 1, 0.1)

	for index := range 8 {
		input := []float64{float64(index+1) / 10}
		_, _ = coder.SettleFromBatchOptions(input, []float64{0.01}, true, true)
	}

	return coder
}

func TestUpdate(t *testing.T) {
	Convey("Given a reset lifecycle with only stale category state", t, func() {
		thesis := types.NewThesis(t.Context(), nil)
		thesis.At = time.Unix(10, 0).UTC()
		symbol := types.NewSymbol("BTC/USD", nil)
		symbol.Categories.Store("BTC/USD", []types.Category{{
			Symbol:     "BTC/USD",
			Type:       types.CategoryAggressiveDrive,
			Supporting: []string{"cvd:drive"},
		}})
		symbol.Graphs.Store("market_graph", NewGraph(thesis.At))
		thesis.Symbols.Store("BTC/USD", symbol)

		err := NewSolver(nil, nil).Update(thesis)

		Convey("It should wait for the first measurement of the new lifecycle", func() {
			So(err, ShouldBeNil)
			stored, found := symbol.Graphs.Load("market_graph")
			So(found, ShouldBeTrue)
			graph := stored.(*Graph)
			So(graph.Nodes, ShouldBeEmpty)
			So(graph.Edges, ShouldBeEmpty)
		})
	})

	Convey("Given a symbol with every category-bearing signal measurement", t, func() {
		thesis := types.NewThesis(t.Context(), nil)
		thesis.At = time.Unix(9, 0).UTC()
		symbol := types.NewSymbol("BTC/USD", nil)
		categories := map[types.CategoryType]struct{}{}
		references := map[string]struct{}{}

		for index, schema := range types.CategorySchemas {
			value := 1 / float64(index+2)
			metricKey := types.MetricKey(schema.Metric, schema.Side)

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
			putTestMetric(measurement, schema.Metric, value, &value, nmtypes.UnitDimensionless)
			symbol.AppendMeasurement(measurement)
			categories[schema.Category] = struct{}{}
		}
		thesis.Symbols.Store("BTC/USD", symbol)

		categoryErr := category.NewSolver(nil, nil, nil).Update(thesis)
		err := NewSolver(nil, nil).Update(thesis)

		Convey("It should compile a connected evidence graph rather than one winning edge", func() {
			So(categoryErr, ShouldBeNil)
			So(err, ShouldBeNil)
			stored, found := symbol.Graphs.Load("market_graph")
			So(found, ShouldBeTrue)
			graph := stored.(*Graph)
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
			storedCategories, categoriesFound := symbol.Categories.Load("BTC/USD")
			So(categoriesFound, ShouldBeTrue)

			sharedLinks := 0

			for _, category := range storedCategories.([]types.Category) {
				for _, peer := range storedCategories.([]types.Category) {
					if category.Type == peer.Type {
						continue
					}

					for _, evidence := range category.Supporting {
						if slices.Contains(peer.Supporting, evidence) {
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

			// Every measurement earns its own voice on the thesis and the
			// category classifier adds its supporting/contradicting/shared
			// enrichment on top of it; neither path replaces the other.
			So(thesisEdges, ShouldEqual, len(graph.Nodes)-1)
			So(graph.Nodes, ShouldHaveLength, len(references)+len(categories)+1)
		})
	})

	Convey("Given completed upstream stages without a causal estimate", t, func() {
		thesis := types.NewThesis(t.Context(), nil)
		thesis.At = time.Unix(1, 0).UTC()
		bitcoin := types.NewSymbol("BTC/USD", nil)
		drive := 0.8
		separation := 0.75
		bitcoinMeasurement := newTestMeasurement("cvd-measurement", types.SourceCVD, "BTC/USD", thesis.At)
		putTestMetric(bitcoinMeasurement, types.MetricDrive, drive, &drive, nmtypes.UnitDimensionless)
		putTestMetric(bitcoinMeasurement, types.MetricHypothesisSeparation, separation, &separation, nmtypes.UnitDimensionless)
		bitcoin.AppendMeasurement(bitcoinMeasurement)
		bitcoin.Categories.Store("BTC/USD", []types.Category{{
			Symbol:     "BTC/USD",
			Type:       types.CategoryAggressiveDrive,
			Confidence: 0.6,
			Strength:   drive,
			Supporting: []string{"cvd:drive"},
		}})
		thesis.Symbols.Store("BTC/USD", bitcoin)
		thesis.Symbols.Store("ETH/USD", types.NewSymbol("ETH/USD", nil))

		solver := NewSolver(nil, nil)

		err := solver.Update(thesis)

		Convey("It should compile the measurement, its category, the hypothesis, and both enrichment layers", func() {
			So(err, ShouldBeNil)
			stored, found := bitcoin.Graphs.Load("market_graph")
			So(found, ShouldBeTrue)
			graph := stored.(*Graph)
			So(graph.Nodes, ShouldHaveLength, 4)
			So(graph.Edges, ShouldHaveLength, 4)
			So(graph.Edges[0].Relation, ShouldEqual, RelationSupports)
			So(graph.Edges[0].Evidence,
				ShouldResemble, []string{"cvd-measurement", "cvd:drive"})
			So(graph.Edges[0].Quality, ShouldBeNil)
		})
	})

	Convey("Given completed graphs for the focused and unfocused pairs", t, func() {
		previousFocus := types.Focus()
		types.SetFocus("BTC/USD")
		Reset(func() { types.SetFocus(previousFocus) })
		thesis := types.NewThesis(t.Context(), nil)
		thesis.At = time.Unix(2, 0).UTC()

		for _, symbolName := range []string{"BTC/USD", "ETH/USD"} {
			symbol := types.NewSymbol(symbolName, nil)
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
			symbol.Categories.Store(symbolName, []types.Category{{
				Symbol: symbolName, Type: types.CategoryAggressiveDrive,
				Strength: drive, Confidence: 0.8,
				Supporting: []string{"cvd:drive"},
			}})

			thesis.Symbols.Store(symbolName, symbol)
		}

		ui := make(chan []byte, 2)
		err := NewSolver(ui, nil).Update(thesis)

		Convey("It should publish only the graph selected by the UI focus", func() {
			So(err, ShouldBeNil)
			So(ui, ShouldHaveLength, 1)
			payload := string(<-ui)
			So(payload, ShouldContainSubstring, "BTC/USD-measurement")
			So(payload, ShouldNotContainSubstring, "ETH/USD-measurement")
			So(payload, ShouldContainSubstring, `"relation":"supports"`)
		})
	})
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
			Weight: 0.8, Confidence: 0.9,
		})
		graph.AddEdge(&Edge{
			From: "forecast", To: "causal", Relation: RelationContradicts,
			Weight: 0.6, Confidence: 0.7,
		})

		Convey("It should replace the obsolete claim without duplicating the target", func() {
			So(graph.Edges, ShouldHaveLength, 1)
			So(graph.Edges[0].Relation, ShouldEqual, RelationContradicts)
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
		symbol := types.NewSymbol("BTC/USD", nil)
		symbol.Categories.Store("BTC/USD", []types.Category{
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
		NewSolver(nil, nil).extractCategoryNodes(symbol, graph)

		err := NewSolver(nil, nil).inferStructuralEdges(symbol, graph)

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
		solver := NewSolver(nil, nil)

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

		err := solver.inferStructuralEdges(types.NewSymbol("BTC/USD", nil), graph)
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
		solver := NewSolver(nil, nil)

		for index := range 256 {
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

		err := solver.inferStructuralEdges(types.NewSymbol("BTC/USD", nil), graph)
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
		symbol := types.NewSymbol("BTC/USD", nil)
		symbol.Cognition.Store("BTC/USD", types.Cognition{
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
		solver := NewSolver(nil, nil)
		solver.extractCognitionNodes(symbol, graph)

		err := solver.inferStructuralEdges(symbol, graph)

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
		symbol := types.NewSymbol("BTC/USD", nil)
		symbol.Causal.Store("BTC/USD", map[string]any{
			"association":       0.1,
			"associationScore":  0.2,
			"doExpectation":     0.25,
			"interventionScore": 0.4,
			"precision":         0.5,
			"probabilities":     []float64{0.12, 0.58, 0.2, 0.1},
		})
		thesis.Symbols.Store("BTC/USD", symbol)
		graph := NewGraph(at)
		solver := NewSolver(nil, nil)

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
		symbol := types.NewSymbol("BTC/USD", nil)
		symbol.Causal.Store("BTC/USD", map[string]any{"intervention": 1.0})
		thesis.Symbols.Store("BTC/USD", symbol)

		err := NewSolver(nil, nil).extractCausalNodes(
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
		symbol := types.NewSymbol("BTC/USD", nil)
		symbol.Resonance.Store("BTC/USD", graphResonanceManifold())
		symbol.Resonance.Store(
			types.ResonanceReturnForecastKey,
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
		solver := NewSolver(nil, nil)

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
		symbol := types.NewSymbol("BTC/USD", nil)
		symbol.Resonance.Store("BTC/USD", graphResonanceManifold())
		dynamics := nomagique.Frame{}
		dynamics.Put(learning.SymbolDynamicsReady, 1)
		dynamics.Put(learning.SymbolDynamicsSampleCount, 8)
		dynamics.Put(learning.SymbolDynamicsVelocity, 0.4)
		dynamics.Put(learning.SymbolDynamicsMemory, 0.3)
		dynamics.Put(learning.SymbolDynamicsPassivityResidue, -0.1)
		dynamics.Put(learning.SymbolDynamicsJumpVariance, 0.02)
		symbol.Resonance.Store(learning.PredictiveDynamicsKey, dynamics)
		graph := NewGraph(at)

		NewSolver(nil, nil).extractResonanceNodes(symbol, graph)

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

		NewSolver(nil, nil).extractManifoldNodes(thesis, graph)

		Convey("It should retain the measured phase and directional inference", func() {
			node := graph.Nodes["man:universe:phase_direction"]
			So(node, ShouldNotBeNil)
			So(node.Kind, ShouldEqual, KindManifold)
			So(node.Value, ShouldEqual, 1.0)
			So(node.Strength, ShouldEqual, 1.0)
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

	solver := NewSolver(nil, nil)
	symbol := types.NewSymbol("BTC/USD", nil)

	b.ReportAllocs()

	for b.Loop() {
		graph := NewGraph(at)
		graph.Nodes = nodes
		if err := solver.inferStructuralEdges(symbol, graph); err != nil {
			b.Fatal(err)
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
		symbolState := types.NewSymbol(symbol, nil)
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

		symbolState.Categories.Store(symbol, []types.Category{
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

		symbolState.Resonance.Store(symbol, coder)
		symbolState.Causal.Store(symbol, map[string]any{
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
		symbolState.Cognition.Store(symbol, types.Cognition{
			Symbol:     symbol,
			Source:     "cognition",
			At:         at,
			Winner:     string(types.CategoryRiskOnSurge),
			Confidence: 0.8,
		})
	}

	ui := make(chan []byte, 1)
	solver := NewSolver(ui, nil)
	previousFocus := types.Focus()
	types.SetFocus("SIM0/USD")
	defer types.SetFocus(previousFocus)
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		thesis.Symbols.Range(func(_, value any) bool {
			return true
		})

		if err := solver.Update(thesis); err != nil {
			b.Fatal(err)
		}

		<-ui
	}
}

func TestHeldCognitionGraphEvidence(t *testing.T) {
	Convey("Given a hysteretically held cognition reading", t, func() {
		at := time.Unix(1, 0).UTC()
		symbol := types.NewSymbol("BTC/USD", nil)
		symbol.Cognition.Store(symbol.Symbol, types.Cognition{
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
		solver := NewSolver(nil, nil)

		solver.extractCognitionNodes(symbol, graph)

		Convey("It should retain audit state without exposing a root or lookahead", func() {
			winner, found := graph.Nodes["cog:BTC/USD:winner_regime"]
			So(found, ShouldBeTrue)
			So(winner.Metadata["held"], ShouldEqual, true)
			So(graph.Nodes, ShouldHaveLength, 1)
			So(graph.Roots(), ShouldBeEmpty)
		})
	})
}
