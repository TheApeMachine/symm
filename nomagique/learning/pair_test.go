package learning

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestParsePredictedActual(testingTB *testing.T) {
	cases := []struct {
		name            string
		primary         float64
		extras          []float64
		expectPredicted float64
		expectActual    float64
		expectError     error
	}{
		{
			name:            "primary plus one extra",
			primary:         10,
			extras:          []float64{12},
			expectPredicted: 10,
			expectActual:    12,
		},
		{
			name:            "two extras ignore primary",
			primary:         0,
			extras:          []float64{10, 15},
			expectPredicted: 10,
			expectActual:    15,
		},
		{
			name:        "zero predicted",
			primary:     0,
			extras:      []float64{10},
			expectError: ErrZeroPredicted,
		},
		{
			name:        "empty extras",
			primary:     10,
			extras:      nil,
			expectError: ErrEmptyInputs,
		},
	}

	for _, testCase := range cases {
		testCase := testCase

		Convey("Given "+testCase.name, testingTB, func() {
			predicted, actual, err := parsePredictedActual(
				testCase.primary, testCase.extras,
			)

			if testCase.expectError != nil {
				Convey("It should return the expected error", func() {
					So(err, ShouldEqual, testCase.expectError)
				})

				return
			}

			Convey("It should parse predicted and actual", func() {
				So(err, ShouldBeNil)
				So(predicted, ShouldEqual, testCase.expectPredicted)
				So(actual, ShouldEqual, testCase.expectActual)
			})
		})
	}
}
