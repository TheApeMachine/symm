package causal

import (
	"context"
	"math"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/types"
)

func TestObserve(t *testing.T) {
	Convey("Given one pending causal forecast", t, func() {
		solver := NewSolver(types.NewThesis(context.Background(), nil), nil, nil, nil)
		at := time.Unix(1, 0)
		features := [3]float64{0.25, 0.5, 0.01}
		_, _, resolved, err := solver.observe(
			"BTC/USD", features, 100, at, true,
		)
		So(err, ShouldBeNil)
		So(resolved, ShouldBeFalse)

		Convey("It should not resolve against the same timestamp", func() {
			_, _, resolved, err = solver.observe(
				"BTC/USD", [3]float64{1, 1, 1}, 110, at, true,
			)

			So(err, ShouldBeNil)
			So(resolved, ShouldBeFalse)
		})

		Convey("It should resolve against a strictly later midpoint", func() {
			row, rows, resolved, err := solver.observe(
				"BTC/USD", [3]float64{1, 1, 1}, 110,
				at.Add(time.Second), true,
			)

			So(err, ShouldBeNil)
			So(resolved, ShouldBeTrue)
			So(row[:3], ShouldResemble, features[:])
			So(row[3], ShouldAlmostEqual, math.Log(110.0/100.0), 1e-12)
			So(rows, ShouldHaveLength, 1)
		})

		Convey("It should not carry a target across a missing forecast", func() {
			_, rows, resolved, err := solver.observe(
				"BTC/USD", [3]float64{}, 110,
				at.Add(time.Second), false,
			)
			So(err, ShouldBeNil)
			So(resolved, ShouldBeTrue)
			So(rows, ShouldHaveLength, 1)

			nextFeatures := [3]float64{0.75, 0.25, 0.02}
			_, _, resolved, err = solver.observe(
				"BTC/USD", nextFeatures, 120,
				at.Add(2*time.Second), true,
			)
			So(err, ShouldBeNil)
			So(resolved, ShouldBeFalse)

			row, rows, resolved, err := solver.observe(
				"BTC/USD", [3]float64{}, 132,
				at.Add(3*time.Second), false,
			)
			So(err, ShouldBeNil)
			So(resolved, ShouldBeTrue)
			So(row[:3], ShouldResemble, nextFeatures[:])
			So(row[3], ShouldAlmostEqual, math.Log(132.0/120.0), 1e-12)
			So(rows, ShouldHaveLength, 2)
		})
	})
}

func BenchmarkObserve(b *testing.B) {
	solver := NewSolver(types.NewThesis(context.Background(), nil), nil, nil, nil)
	at := time.Unix(1, 0)
	midpoint := 100.0
	features := [3]float64{0.25, 0.5, 0.01}

	if _, _, _, err := solver.observe(
		"BTC/USD", features, midpoint, at, true,
	); err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		at = at.Add(time.Nanosecond)
		midpoint += 0.01

		if _, _, _, err := solver.observe(
			"BTC/USD", features, midpoint, at, true,
		); err != nil {
			b.Fatal(err)
		}
	}
}
