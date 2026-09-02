package main

import (
	"go/ast"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"golang.org/x/tools/go/packages"
)

func TestSplitMetricIdentity(t *testing.T) {
	Convey("Given a side-qualified metric wire name", t, func() {
		identity := splitMetricIdentity("liquidity", "depth_ratio:bid")

		Convey("It resolves to the same metric and side identity as a producer", func() {
			So(identity, ShouldResemble, metricID{
				Source: "liquidity",
				Metric: "depth_ratio",
				Side:   "bid",
			})
		})
	})

	Convey("Given an unqualified metric wire name", t, func() {
		identity := splitMetricIdentity("hawkes", "arrival_rate")

		Convey("It retains an empty side", func() {
			So(identity, ShouldResemble, metricID{
				Source: "hawkes",
				Metric: "arrival_rate",
			})
		})
	})
}

func TestBuildReport(t *testing.T) {
	Convey("Given a producer and side-normalized named consumer", t, func() {
		identity := splitMetricIdentity("toxicity", "touch_fill_fraction:ask")
		report := buildReport(
			[]producer{{ID: identity}},
			[]consumerEdge{{
				ID:       identity,
				Kind:     "bound",
				Consumer: "position-risk:executable-liquidity",
			}},
			nil,
		)

		Convey("The producer is referenced rather than falsely reported dead", func() {
			So(report.Summary.TotalProducers, ShouldEqual, 1)
			So(report.Summary.ReferencedProducers, ShouldEqual, 1)
			So(report.Summary.DeadProducers, ShouldEqual, 0)
			So(report.Producers[0].Dead, ShouldBeFalse)
		})
	})
}

func TestScanCategoryConsumers(t *testing.T) {
	Convey("Given the production CategorySchemas declaration", t, func() {
		loaded, err := packages.Load(&packages.Config{
			Mode: packages.NeedName | packages.NeedFiles | packages.NeedSyntax |
				packages.NeedTypes | packages.NeedTypesInfo,
			Dir: "../..",
		}, "./types")
		So(err, ShouldBeNil)
		So(loaded, ShouldHaveLength, 1)

		var edges []consumerEdge

		for _, file := range loaded[0].Syntax {
			edges = append(edges, scanCategoryConsumers(loaded[0], file, "types/category.go")...)
		}

		Convey("Every declared row becomes a side-normalized named consumer", func() {
			So(len(edges), ShouldEqual, lenCategorySchemaRows(loaded[0].Syntax))
			So(edges, ShouldContainConsumerEdge, consumerEdge{
				ID:       metricID{Source: "toxicity", Metric: "fill_fraction_zscore", Side: "ask"},
				Kind:     "bound",
				Consumer: "category:LiquidityVacuum (github.com/theapemachine/symm/types)",
				Package:  "github.com/theapemachine/symm/types",
			})
		})
	})
}

func TestScanFineConsumers(t *testing.T) {
	Convey("Given Manifold's declared forcing selectors", t, func() {
		loaded, err := packages.Load(&packages.Config{
			Mode: packages.NeedName | packages.NeedFiles | packages.NeedSyntax |
				packages.NeedTypes | packages.NeedTypesInfo,
			Dir: "../..",
		}, "./logic/manifold")
		So(err, ShouldBeNil)
		So(loaded, ShouldHaveLength, 1)

		var edges []consumerEdge

		for _, file := range loaded[0].Syntax {
			edges = append(edges, scanFineConsumers(
				loaded[0], file, "logic/manifold/solver.go",
			)...)
		}

		Convey("Both side-specific Hawkes reads become named Manifold inputs", func() {
			consumer := "manifold.forcingInputs (github.com/theapemachine/symm/logic/manifold)"

			So(edges, ShouldHaveLength, 2)
			So(edges, ShouldContainConsumerEdge, consumerEdge{
				ID:       metricID{Source: "hawkes", Metric: "excitation_fraction", Side: "buy"},
				Kind:     "catalog",
				Consumer: consumer,
				Package:  "github.com/theapemachine/symm/logic/manifold",
			})
			So(edges, ShouldContainConsumerEdge, consumerEdge{
				ID:       metricID{Source: "hawkes", Metric: "excitation_fraction", Side: "sell"},
				Kind:     "catalog",
				Consumer: consumer,
				Package:  "github.com/theapemachine/symm/logic/manifold",
			})
		})
	})
}

func lenCategorySchemaRows(files []*ast.File) int {
	count := 0

	for _, file := range files {
		ast.Inspect(file, func(node ast.Node) bool {
			declaration, ok := node.(*ast.ValueSpec)

			if !ok || !namesIdentifier(declaration.Names, "CategorySchemas") {
				return true
			}

			for _, value := range declaration.Values {
				if table, ok := value.(*ast.CompositeLit); ok {
					count += len(table.Elts)
				}
			}

			return false
		})
	}

	return count
}

func shouldContainConsumerEdge(actual any, expected ...any) string {
	edges := actual.([]consumerEdge)
	target := expected[0].(consumerEdge)

	for _, edge := range edges {
		if edge.ID == target.ID && edge.Kind == target.Kind &&
			edge.Consumer == target.Consumer && edge.Package == target.Package {
			return ""
		}
	}

	return "expected consumer edge was not found"
}

var ShouldContainConsumerEdge = shouldContainConsumerEdge

func BenchmarkScanFineConsumers(b *testing.B) {
	loaded, err := packages.Load(&packages.Config{
		Mode: packages.NeedName | packages.NeedFiles | packages.NeedSyntax |
			packages.NeedTypes | packages.NeedTypesInfo,
		Dir: "../..",
	}, "./logic/manifold")
	if err != nil {
		b.Fatal(err)
	}

	if len(loaded) != 1 {
		b.Fatalf("expected one Manifold package, got %d", len(loaded))
	}

	b.ReportAllocs()
	b.ResetTimer()

	for iteration := 0; iteration < b.N; iteration++ {
		count := 0

		for _, file := range loaded[0].Syntax {
			count += len(scanFineConsumers(
				loaded[0], file, "logic/manifold/solver.go",
			))
		}

		if count != 2 {
			b.Fatalf("expected two Manifold forcing selectors, got %d", count)
		}
	}
}
