package probability

import (
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestTemperatureShape(t *testing.T) {
	t.Parallel()

	Convey("TemperatureShape", t, func() {
		base := []Ranked{
			{Token: "a", Probability: 0.5},
			{Token: "b", Probability: 0.5},
		}

		Convey("when distribution is empty, It should return nil", func() {
			So(TemperatureShape(nil, 1), ShouldBeNil)
		})

		Convey("when temperature <= 0, It should split probability among argmax ties", func() {
			out := TemperatureShape(base, 0)

			So(len(out), ShouldEqual, 2)
			So(out[0].Probability+out[1].Probability, ShouldAlmostEqual, 1, 1e-9)
			So(out[0].Probability, ShouldAlmostEqual, 0.5, 1e-9)
		})

		Convey("when temperature > 0, It should renormalize power-scaled masses", func() {
			out := TemperatureShape(base, 2)
			total := 0.0

			for _, entry := range out {
				total += entry.Probability
			}

			So(total, ShouldAlmostEqual, 1, 1e-9)
		})

		Convey("when all masses zero after scaling, It should return nil", func() {
			zero := []Ranked{{Token: "z", Probability: 0}}

			So(TemperatureShape(zero, 1), ShouldBeNil)
		})
	})
}

func TestNormalizeMap(t *testing.T) {
	t.Parallel()

	Convey("NormalizeMap rescales in place", t, func() {
		m := map[string]float64{"a": 1, "b": 3}
		NormalizeMap(m)

		So(m["a"], ShouldAlmostEqual, 0.25, 1e-9)
		So(m["b"], ShouldAlmostEqual, 0.75, 1e-9)
	})

	Convey("when total is zero, It should leave map unchanged", t, func() {
		m := map[string]float64{"a": 0}
		NormalizeMap(m)

		So(m["a"], ShouldEqual, 0)
	})
}

func TestAdditiveSmoothing(t *testing.T) {
	t.Parallel()

	Convey("AdditiveSmoothing", t, func() {
		p := AdditiveSmoothing(1, 10, 100, 0.5)

		So(p, ShouldAlmostEqual, (1+0.5)/(10+50), 1e-9)

		// Negative smoothing: AdditiveSmoothing rejects it (see temperature.go) so the
		// (count+α)/(total+α|V|) form is never applied with an invalid α.
		So(math.IsNaN(AdditiveSmoothing(0, 0, 1, -1)), ShouldBeTrue)

		// Negative vocabulary size: same guard — |V| must be non-negative for the denominator.
		So(math.IsNaN(AdditiveSmoothing(0, 0, -3, 1)), ShouldBeTrue)
	})
}

func TestRepetitionPenalty(t *testing.T) {
	t.Parallel()

	Convey("RepetitionPenalty", t, func() {
		base := []Ranked{
			{Token: "a", Probability: 0.7},
			{Token: "b", Probability: 0.3},
		}

		Convey("when inputs are empty, It should return the original slice", func() {
			So(RepetitionPenalty(nil, []string{"a"}, 0.5), ShouldBeNil)

			out := RepetitionPenalty(base, nil, 0.5)

			So(out, ShouldResemble, base)
		})

		Convey("when recent contains a token, It should down-weight and renormalize", func() {
			out := RepetitionPenalty(base, []string{"a"}, 0.25)
			total := 0.0

			for _, entry := range out {
				total += entry.Probability
			}

			So(total, ShouldAlmostEqual, 1, 1e-9)
			So(out[1].Probability, ShouldBeGreaterThan, out[0].Probability)
		})
	})
}

func TestSurprisal(t *testing.T) {
	t.Parallel()

	Convey("Surprisal uses floor when p <= 0", t, func() {
		So(Surprisal(0, 0.25), ShouldAlmostEqual, 2, 1e-9)
	})

	Convey("Surprisal returns -log2(p) for valid p", t, func() {
		So(Surprisal(0.5, 0), ShouldAlmostEqual, 1, 1e-9)
	})
}

func BenchmarkTemperatureShape(b *testing.B) {
	dist := []Ranked{
		{Token: "a", Probability: 0.1},
		{Token: "b", Probability: 0.2},
		{Token: "c", Probability: 0.7},
	}

	b.ResetTimer()

	for b.Loop() {
		_ = TemperatureShape(dist, 0.8)
	}
}

func BenchmarkNormalizeMap(b *testing.B) {
	template := map[string]float64{
		"a": 1, "b": 2, "c": 3, "d": 4, "e": 5,
	}

	dup := make(map[string]float64, len(template))

	b.ResetTimer()

	for b.Loop() {
		clear(dup)

		for key, val := range template {
			dup[key] = val
		}

		NormalizeMap(dup)
	}
}
