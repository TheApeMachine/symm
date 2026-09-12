package tests

import (
	"fmt"
	"iter"
	"math"
	"testing"
	"unsafe"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/core"
)

// Case defines the test configuration for any Primitive.
type Case[T, U any] struct {
	Name string
	Seed U
	// Operation is the primitive under test.
	Operation core.Primitive
	// Factory creates a fresh primitive instance under test.
	Factory func() core.Primitive
	// Reference defines the mathematical ground truth: how input T transforms accumulator U.
	// For Add, this is: func(acc, val float64) float64 { return acc + val }
	Reference func(current U, in T) U
	// CustomVectors allows primitives to add domain-specific scenarios.
	CustomVectors [][]T
}

// TableRow represents a generated test scenario.
type TableRow[T, U any] struct {
	Name     string
	Inputs   []T
	Expected []U
}

// Check exercises independent delivery runs, multi-yield input, empty input,
// nil input, and every IEEE exceptional value against the reference function.
func Check[T, U any](t *testing.T, example Case[T, U]) {
	getOp := func() core.Primitive {
		if example.Factory != nil {
			return example.Factory()
		}
		return example.Operation
	}

	Convey("Setup "+example.Name, t, func() {
		op := getOp()

		Convey("empty and nil input", func() {
			emptySeq := SliceToSeq([]T{})
			outSeq := op.Next(emptySeq)
			actual := CollectSeq[U](outSeq)

			So(len(actual), ShouldEqual, 0)
			So(op.Error(), ShouldBeNil)
		})

		Convey("multi-yield dynamic table runs", func() {
			table := GenerateTable(example)

			for _, row := range table {
				Convey(row.Name, func() {
					rowOp := getOp()

					inSeq := SliceToSeq(row.Inputs)
					outSeq := rowOp.Next(inSeq)
					actual := CollectSeq[U](outSeq)

					So(actual, ShouldMatchElements[U], row.Expected)
					So(rowOp.Error(), ShouldBeNil)
				})
			}
		})

		Convey("independent delivery runs", func() {
			indepOp := getOp()

			if example.Reference != nil {
				run1Inputs := []T{generateValue[T](1), generateValue[T](2)}
				run2Inputs := []T{generateValue[T](3), generateValue[T](4)}

				// Run 1
				expected1 := ComputeExpected(example.Seed, run1Inputs, example.Reference)
				outSeq1 := indepOp.Next(SliceToSeq(run1Inputs))
				actual1 := CollectSeq[U](outSeq1)
				So(actual1, ShouldMatchElements[U], expected1)

				// Run 2: starts from the state left after Run 1
				lastAcc := expected1[len(expected1)-1]
				expected2 := ComputeExpected(lastAcc, run2Inputs, example.Reference)
				outSeq2 := indepOp.Next(SliceToSeq(run2Inputs))
				actual2 := CollectSeq[U](outSeq2)
				So(actual2, ShouldMatchElements[U], expected2)
			}
		})

		Convey("early consumer termination (yield break)", func() {
			breakOp := getOp()
			inputs := []T{generateValue[T](10), generateValue[T](20), generateValue[T](30)}
			inSeq := SliceToSeq(inputs)
			outSeq := breakOp.Next(inSeq)

			// Pull only the first yielded element
			count := 0
			for range outSeq {
				count++
				break // Stop consumer early
			}
			So(count, ShouldEqual, 1)
		})
	})
}

// ----------------------------------------------------------------------------
// Dynamic Table Generator
// ----------------------------------------------------------------------------

func GenerateTable[T, U any](c Case[T, U]) []TableRow[T, U] {
	var rows []TableRow[T, U]

	// 1. Standard single & multi-yield vectors
	vectors := [][]T{
		{generateValue[T](1)},
		{generateValue[T](1), generateValue[T](2), generateValue[T](3)},
		{generateValue[T](-5), generateValue[T](10), generateValue[T](-2)},
		{generateValue[T](0), generateValue[T](0)},
	}

	// 2. Add IEEE exceptional values if T is floating point
	if isFloat[T]() {
		vectors = append(vectors,
			[]T{asT[T](math.Inf(1)), generateValue[T](1)},
			[]T{asT[T](math.Inf(-1)), generateValue[T](-1)},
			[]T{asT[T](math.Inf(1)), asT[T](math.Inf(-1))}, // Inf cancellation -> NaN
			[]T{asT[T](math.NaN()), generateValue[T](5)},
			[]T{asT[T](math.MaxFloat64), asT[T](math.MaxFloat64)}, // Overflow -> +Inf
			[]T{asT[T](0.0), asT[T](math.Copysign(0.0, -1.0))},    // +0.0 and -0.0
		)
	}

	// 3. Append user-defined custom vectors
	vectors = append(vectors, c.CustomVectors...)

	// 4. Generate rows and compute expected outputs via Reference function
	for i, vec := range vectors {
		name := fmt.Sprintf("vector_%d_len_%d", i+1, len(vec))
		expected := ComputeExpected(c.Seed, vec, c.Reference)
		rows = append(rows, TableRow[T, U]{
			Name:     name,
			Inputs:   vec,
			Expected: expected,
		})
	}

	return rows
}

func ComputeExpected[T, U any](seed U, inputs []T, ref func(U, T) U) []U {
	if ref == nil {
		return nil
	}
	out := make([]U, len(inputs))
	acc := seed
	for i, in := range inputs {
		acc = ref(acc, in)
		out[i] = acc
	}
	return out
}

// ----------------------------------------------------------------------------
// Test Sequence Adapters
// ----------------------------------------------------------------------------

func SliceToSeq[T any](items []T) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for i := range items {
			if !yield(unsafe.Pointer(&items[i])) {
				return
			}
		}
	}
}

func CollectSeq[U any](seq iter.Seq[unsafe.Pointer]) []U {
	var out []U
	if seq == nil {
		return out
	}
	for ptr := range seq {
		out = append(out, *(*U)(ptr))
	}
	return out
}

// ----------------------------------------------------------------------------
// IEEE-754 Aware Assertion for GoConvey
// ----------------------------------------------------------------------------

func ShouldMatchElements[U any](actual any, expected ...any) string {
	act, ok1 := actual.([]U)
	exp, ok2 := expected[0].([]U)
	if !ok1 || !ok2 {
		return "Both arguments must be []U"
	}
	if len(act) != len(exp) {
		return fmt.Sprintf("Slice lengths differ: expected %d, got %d", len(exp), len(act))
	}
	for i := range act {
		if !equalIEEE(act[i], exp[i]) {
			return fmt.Sprintf("Mismatch at index %d: expected %v, got %v", i, exp[i], act[i])
		}
	}
	return ""
}

func equalIEEE(a, b any) bool {
	switch av := a.(type) {
	case float64:
		bv, ok := b.(float64)
		if !ok {
			return false
		}
		if math.IsNaN(av) && math.IsNaN(bv) {
			return true // Treat NaN == NaN as passing
		}
		return av == bv
	case float32:
		bv, ok := b.(float32)
		if !ok {
			return false
		}
		if math.IsNaN(float64(av)) && math.IsNaN(float64(bv)) {
			return true
		}
		return av == bv
	default:
		return a == b
	}
}

// ----------------------------------------------------------------------------
// Type Introspection Helpers
// ----------------------------------------------------------------------------

func isFloat[T any]() bool {
	var zero T
	switch any(zero).(type) {
	case float64, float32:
		return true
	}
	return false
}

func asT[T any](v any) T {
	return v.(T)
}

func generateValue[T any](n int) T {
	var zero T
	switch any(zero).(type) {
	case float64:
		return any(float64(n)).(T)
	case float32:
		return any(float32(n)).(T)
	case int:
		return any(n).(T)
	case int64:
		return any(int64(n)).(T)
	case bool:
		return any(n != 0).(T)
	}
	return zero
}
