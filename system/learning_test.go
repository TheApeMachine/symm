package system

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestNewLearning(t *testing.T) {
	Convey("Setup", t, func() {
		Convey("When NewLearning is called", func() {
			learning := NewLearning()

			Convey("resonance gate defaults land from viper", func() {
				So(learning.MinSupportedHorizon, ShouldEqual, 2)
				So(learning.MinTaskRelativePrecision, ShouldEqual, 0.5)
				So(learning.MinSurprise, ShouldEqual, 0.5)
				So(learning.ResonanceBypass, ShouldBeFalse)
				So(learning.MinEnterZ, ShouldEqual, 0.35)
				So(learning.MaxPathLength, ShouldEqual, 8)
				So(learning.MinPathLength, ShouldEqual, 2)
			})
		})
	})
}

func TestApplyResonance(t *testing.T) {
	Convey("Setup", t, func() {
		learning := NewLearning()

		Convey("When ApplyResonance receives a gate-lab patch", func() {
			learning.ApplyResonance(map[string]any{
				"type":                        "resonance",
				"bypass":                      true,
				"min_task_relative_precision": 0.1,
				"min_surprise":                0.2,
				"min_supported_horizon":       1.0,
				"min_enter_z":                 0.0,
			})

			Convey("floors and bypass update", func() {
				So(learning.ResonanceBypass, ShouldBeTrue)
				So(learning.MinTaskRelativePrecision, ShouldEqual, 0.1)
				So(learning.MinSurprise, ShouldEqual, 0.2)
				So(learning.MinSupportedHorizon, ShouldEqual, 1)
				So(learning.MinEnterZ, ShouldEqual, 0.0)
			})
		})

		Convey("When ApplyResonance receives an empty patch", func() {
			before := learning.MinEnterZ
			learning.ApplyResonance(nil)

			Convey("fields are unchanged", func() {
				So(learning.MinEnterZ, ShouldEqual, before)
			})
		})
	})
}

func BenchmarkNewLearning(b *testing.B) {
	b.ReportAllocs()

	for b.Loop() {
		_ = NewLearning()
	}
}

func BenchmarkApplyResonance(b *testing.B) {
	learning := NewLearning()
	patch := map[string]any{
		"type":                        "resonance",
		"bypass":                      true,
		"min_task_relative_precision": 0.1,
		"min_surprise":                0.2,
		"min_supported_horizon":       1.0,
		"min_enter_z":                 0.0,
	}

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		learning.ApplyResonance(patch)
	}
}
