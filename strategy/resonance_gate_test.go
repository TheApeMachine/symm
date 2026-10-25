package strategy

import "testing"

func TestForwardCurveAligned(t *testing.T) {
	cases := []struct {
		name    string
		forward []float64
		want    bool
	}{
		{name: "empty", forward: nil, want: false},
		{name: "bullish", forward: []float64{0.4, 0.2}, want: true},
		{name: "bearish", forward: []float64{-0.4, -0.2}, want: true},
		{name: "mixed sign", forward: []float64{0.4, -0.2}, want: false},
		{name: "zero horizon", forward: []float64{0.4, 0}, want: false},
		{name: "bearish one horizon", forward: []float64{-0.3}, want: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := forwardCurveAligned(coderReading{forward: tc.forward})
			if got != tc.want {
				t.Fatalf("forwardCurveAligned(%v) = %v, want %v", tc.forward, got, tc.want)
			}
		})
	}
}
