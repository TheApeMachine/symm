package tests

import (
	"github.com/theapemachine/symm/nomagique/core"
	"math"
	"testing"
)

// CheckQuality implements the source's metadata precedence independently at the
// assertion boundary, including repeated transitions back to missing evidence.
func CheckQuality(t *testing.T, quality, authority core.Primitive) {
	t.Helper()
	cases := []map[string]any{
		{}, {"support": 10.0, "divergence": 4.0, "noise_variance": 2.0},
		{"support": 10.0, "divergence": 4.0}, {"support": 10.0, "divergence": 4.0, "noise_variance": 4.0},
		{"support": 20.0, "mahalanobis_snr": 5.5}, {"support": 1.0, "mahalanobis_snr": 5.5},
		{"mahalanobis_snr": 5.5}, {"divergence": 2.0, "noise_variance": 1.0}, {"support": 0.0},
		{"support": 4.0, "divergence": 0.0, "noise_variance": 1.0},
		{},
	}
	for _, input := range cases {
		f := Fields(t, Drain(t, quality, Values(Record(input)))[0])
		Sound(t, quality)
		support, hasSupport := input["support"].(float64)
		d, hasD := input["divergence"].(float64)
		noise, hasN := input["noise_variance"].(float64)
		mah, hasM := input["mahalanobis_snr"].(float64)
		snr := 0.0
		defined := false
		maturity := 1.0
		if hasD && hasN && noise > 0 {
			snr = d * d / noise
			defined = true
		}
		if hasSupport {
			maturity = 0
			if support > 1 {
				maturity = 1 - 1/support
				if hasM && mah >= 0 {
					snr = mah
					defined = true
				}
			}
		}
		estimated := hasSupport || hasD || hasM
		EqualNumber(t, Number(t, f, "maturity"), maturity)
		EqualNumber(t, Number(t, f, "snr"), snr)
		if core.To[bool](f["snr_defined"]) != defined || core.To[bool](f["estimated"]) != estimated {
			t.Fatal("quality flags")
		}
		factor := 1.0
		if estimated {
			factor = 0.5
			if defined {
				factor = 0.1
				if snr > 0 {
					factor = snr / (1 + snr)
				}
			}
		}
		result := Drain(t, authority, Values(core.From(f)))
		Sound(t, authority)
		EqualNumber(t, result[0], math.Min(1, math.Max(0, maturity*factor)))
	}
}
