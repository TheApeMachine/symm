package mcts

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestDefaultCausalEngineDoExpectation(t *testing.T) {
	Convey("Given linear causal history", t, func() {
		rows := [][]float64{
			{0, 1},
			{1, 3},
			{2, 5},
			{3, 7},
			{4, 9},
		}
		engine := DefaultCausalEngine{}

		expectation, err := engine.DoExpectation(rows, 1, len(rows), 0, 3, nil)

		Convey("It should evaluate the intervention through the causal package", func() {
			So(err, ShouldBeNil)
			So(expectation, ShouldAlmostEqual, 7, 1e-6)
		})
	})
}

func TestDefaultCausalEngineAbductiveCounterfactual(t *testing.T) {
	Convey("Given an observed row from linear causal history", t, func() {
		rows := [][]float64{
			{0, 1},
			{1, 3},
			{2, 5},
			{3, 7},
			{4, 9},
		}
		engine := DefaultCausalEngine{}

		counterfactual, noise, err := engine.AbductiveCounterfactual(
			rows, 1, len(rows), []int{0}, true, rows[2], 0, 4,
		)

		Convey("It should expose the counterfactual reward and abducted noise", func() {
			So(err, ShouldBeNil)
			So(counterfactual, ShouldAlmostEqual, 9, 1e-6)
			So(noise, ShouldAlmostEqual, 0, 1e-6)
		})
	})
}

func BenchmarkDefaultCausalEngineDoExpectation(b *testing.B) {
	rows := [][]float64{
		{0, 1},
		{1, 3},
		{2, 5},
		{3, 7},
		{4, 9},
	}
	engine := DefaultCausalEngine{}

	for b.Loop() {
		_, err := engine.DoExpectation(rows, 1, len(rows), 0, 3, nil)

		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkDefaultCausalEngineAbductiveCounterfactual(b *testing.B) {
	rows := [][]float64{
		{0, 1},
		{1, 3},
		{2, 5},
		{3, 7},
		{4, 9},
	}
	engine := DefaultCausalEngine{}

	for b.Loop() {
		_, _, err := engine.AbductiveCounterfactual(
			rows, 1, len(rows), []int{0}, true, rows[2], 0, 4,
		)

		if err != nil {
			b.Fatal(err)
		}
	}
}
