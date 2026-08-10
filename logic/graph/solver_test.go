package graph

import (
	"math"
	"strconv"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/types"
)

func TestUpdate(t *testing.T) {
	Convey("Given completed upstream stages without a causal estimate", t, func() {
		thesis := types.NewThesis(t.Context(), nil)
		bitcoin := types.NewSymbol("BTC/USD", nil)
		thesis.Symbols.Store("BTC/USD", bitcoin)
		thesis.Symbols.Store("ETH/USD", types.NewSymbol("ETH/USD", nil))

		for _, source := range []types.SourceType{
			types.SourceCategory,
			types.SourceResonance,
			types.SourceManifold,
			types.SourceCausal,
			types.SourceCognition,
		} {
			thesis.Stamp("BTC/USD", source)
		}
		thesis.Stamp("ETH/USD", types.SourceCategory)
		thesis.Stamp("ETH/USD", types.SourceResonance)
		thesis.Stamp("ETH/USD", types.SourceManifold)
		thesis.Stamp("ETH/USD", types.SourceCausal)

		solver := NewSolver(nil, nil)

		err := solver.Update(thesis)

		Convey("It should compile and stamp the graph", func() {
			So(err, ShouldBeNil)
			So(thesis.Stamped("BTC/USD", types.SourceGraph), ShouldBeTrue)
			So(thesis.Stamped("ETH/USD", types.SourceGraph), ShouldBeFalse)
			_, found := bitcoin.Graphs.Load("market_graph")
			So(found, ShouldBeTrue)
		})
	})
}

func TestAddEdge(t *testing.T) {
	Convey("Given an edge whose target is not a registered graph node", t, func() {
		graph := NewGraph(time.Unix(1, 0).UTC())
		graph.AddNode(&Node{ID: "source"})
		edge := &Edge{From: "source", To: "missing"}

		graph.AddEdge(edge)

		Convey("It should not materialize a dangling relationship", func() {
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
		forecast, err := types.NewResonanceForecast(
			[]float64{0.01, 0.02}, []float64{1, 0.5}, 2, 0.4,
		)
		So(err, ShouldBeNil)

		thesis := types.NewThesis(t.Context(), nil)
		thesis.At = at
		symbol := types.NewSymbol("BTC/USD", nil)
		symbol.Resonance.Store("BTC/USD", types.ResonanceReading{
			Symbol:   "BTC/USD",
			At:       at,
			Surprise: 0.25,
			Forecast: forecast,
		})
		thesis.Symbols.Store("BTC/USD", symbol)
		graph := NewGraph(at)
		solver := NewSolver(nil, nil)

		solver.extractResonanceNodes(symbol, graph)

		Convey("It should publish the full-horizon return at measured confidence", func() {
			node, found := graph.Nodes["res:BTC/USD:forecast"]

			So(found, ShouldBeTrue)
			So(node.Value, ShouldEqual, forecast.ExpectedReturn)
			So(node.Value, ShouldNotEqual, forecast.Curve[0])
			So(node.Confidence, ShouldEqual, forecast.Confidence)
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
	forecast, err := types.NewResonanceForecast(
		[]float64{0.01},
		[]float64{1},
		1,
		0.8,
	)

	if err != nil {
		b.Fatal(err)
	}

	for index := range 256 {
		symbol := "SIM" + strconv.Itoa(index) + "/USD"
		symbolState := types.NewSymbol(symbol, nil)
		thesis.Symbols.Store(symbol, symbolState)

		for _, source := range []types.SourceType{
			types.SourceCategory,
			types.SourceResonance,
			types.SourceManifold,
			types.SourceCausal,
			types.SourceCognition,
		} {
			thesis.Stamp(symbol, source)
		}

		symbolState.Categories.Store(symbol, []types.Category{
			{
				Symbol:     symbol,
				Type:       types.CategoryForecastEdge,
				Confidence: 0.8,
				Strength:   0.5,
				Supporting: []string{"sentiment:" + symbol + ":change"},
			},
			{
				Symbol:     symbol,
				Type:       types.CategoryExhaustion,
				Confidence: 0.7,
				Strength:   0.4,
				Opposing:   []string{"sentiment:" + symbol + ":change"},
			},
		})
		symbolState.Resonance.Store(symbol, types.ResonanceReading{
			Symbol:   symbol,
			Surprise: 0.1,
			Forecast: forecast,
		})
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
			Winner:     string(types.CategoryForecastEdge),
			Confidence: 0.8,
		})
	}

	ui := make(chan []byte, 1)
	solver := NewSolver(ui, nil)
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		thesis.Symbols.Range(func(_, value any) bool {
			symbol := value.(*types.Symbol)
			symbol.Readiness.Reset()

			for _, source := range []types.SourceType{
				types.SourceCategory,
				types.SourceResonance,
				types.SourceManifold,
				types.SourceCausal,
				types.SourceCognition,
			} {
				symbol.Stamp(source)
			}

			return true
		})

		if err := solver.Update(thesis); err != nil {
			b.Fatal(err)
		}

		<-ui
	}
}
