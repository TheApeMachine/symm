package resonance

import (
	"math"
	"testing"

	"github.com/theapemachine/nomagique/learning"
	"github.com/theapemachine/symm/types"
)

func TestResonanceVerdict(t *testing.T) {
	ready := learning.PaceOutput{Ready: true, Alpha: 0.03}

	t.Run("withholds a reading until the error window has filled", func(t *testing.T) {
		verdict := resonanceVerdict(
			learning.PaceOutput{Ready: false},
			0.1,
			0.03, 0.005, 0.15,
			nil,
		)

		if verdict.Learning != "warming" || verdict.Tuning != "warming" {
			t.Fatalf("unready pace reported a verdict: %+v", verdict)
		}

		if verdict.LearningHealth != 0 || verdict.TuningHealth != 0 {
			t.Fatalf("unready pace claimed health: %+v", verdict)
		}
	})

	t.Run("separates a tracking model from one its regime has left", func(t *testing.T) {
		tracking := resonanceVerdict(ready, 0.3, 0.03, 0.005, 0.15, nil)
		drifting := resonanceVerdict(ready, 0.65, 0.03, 0.005, 0.15, nil)
		lost := resonanceVerdict(ready, 0.9, 0.03, 0.005, 0.15, nil)

		if tracking.Learning != "predicting" || tracking.LearningHealth != 1 {
			t.Fatalf("low error rank was not read as predicting: %+v", tracking)
		}

		if drifting.Learning != "drifting" || drifting.LearningHealth != 0 {
			t.Fatalf("middling error rank was not read as drifting: %+v", drifting)
		}

		if lost.Learning != "lost" || lost.LearningHealth != -1 {
			t.Fatalf("persistently high error rank was not read as lost: %+v", lost)
		}
	})

	/*
		The rail cases are the reason the band exists: alpha's own value cannot
		tell a settled controller from a clamped one.
	*/
	t.Run("reads a pinned pace as spent authority, not as a settled one", func(t *testing.T) {
		pinnedFast := resonanceVerdict(ready, 0.3, 0.15, 0.005, 0.15, nil)
		pinnedSlow := resonanceVerdict(ready, 0.3, 0.005, 0.005, 0.15, nil)
		adapting := resonanceVerdict(ready, 0.3, 0.03, 0.005, 0.15, nil)

		if pinnedFast.Tuning != "pinned fast" || pinnedFast.AlphaBand != 1 {
			t.Fatalf("ceiling pace not reported pinned: %+v", pinnedFast)
		}

		if pinnedSlow.Tuning != "pinned slow" || pinnedSlow.AlphaBand != 0 {
			t.Fatalf("floor pace not reported pinned: %+v", pinnedSlow)
		}

		if adapting.Tuning != "adapting" {
			t.Fatalf("mid-band pace not reported adapting: %+v", adapting)
		}
	})

	/*
		Log spacing is what makes the resting pace read as centered. On a linear
		band 0.03 sits under a fifth of the way up an interval it rests in the
		middle of, which would show a healthy controller as nearly pinned slow.
	*/
	t.Run("places the resting pace near the middle of its own band", func(t *testing.T) {
		band := paceBand(0.03, 0.005, 0.15)

		if math.Abs(band-0.5) > 0.06 {
			t.Fatalf("rest alpha placed at %v, not near mid-band", band)
		}
	})

	t.Run("keeps direction and conviction as separate facts", func(t *testing.T) {
		weakLong := resonanceVerdict(ready, 0.3, 0.03, 0.005, 0.15, &types.ResonanceForecast{
			ExpectedReturn: 0.004,
			Confidence:     0.1,
		})
		short := resonanceVerdict(ready, 0.3, 0.03, 0.005, 0.15, &types.ResonanceForecast{
			ExpectedReturn: -0.004,
			Confidence:     0.9,
		})

		if weakLong.Direction != 1 || weakLong.Conviction != 0.1 {
			t.Fatalf("weakly held long was folded into one number: %+v", weakLong)
		}

		if short.Direction != -1 || short.Conviction != 0.9 {
			t.Fatalf("short not reported: %+v", short)
		}
	})

	t.Run("reports no direction without a forecast", func(t *testing.T) {
		verdict := resonanceVerdict(ready, 0.3, 0.03, 0.005, 0.15, nil)

		if verdict.Direction != 0 || verdict.Conviction != 0 {
			t.Fatalf("absent forecast produced a call: %+v", verdict)
		}
	})
}
