package probability

import (
	"math/rand"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestSortDescending(t *testing.T) {
	t.Parallel()

	Convey("SortDescending orders by probability then token", t, func() {
		dist := []Ranked{
			{Token: "z", Probability: 0.2},
			{Token: "a", Probability: 0.2},
			{Token: "m", Probability: 0.5},
		}

		SortDescending(dist)

		So(dist[0].Token, ShouldEqual, "m")
		So(dist[1].Token, ShouldEqual, "a")
		So(dist[2].Token, ShouldEqual, "z")
	})
}

func TestSample(t *testing.T) {
	t.Parallel()

	Convey("Sample", t, func() {
		dist := []Ranked{
			{Token: "x", Probability: 0.4},
			{Token: "y", Probability: 0.6},
		}

		Convey("when distribution is empty, It should return empty string", func() {
			rng := rand.New(rand.NewSource(42))

			So(Sample(nil, 1, rng), ShouldEqual, "")
		})

		Convey("when temperature > 0, It should draw from CDF", func() {
			rng := rand.New(rand.NewSource(42))
			tok := Sample(dist, 1, rng)

			So(tok == "x" || tok == "y", ShouldBeTrue)
		})

		Convey("when temperature == 0, It should pick among maxima", func() {
			tied := []Ranked{
				{Token: "p", Probability: 0.5},
				{Token: "q", Probability: 0.5},
			}

			rng := rand.New(rand.NewSource(42))
			tok := Sample(tied, 0, rng)

			So(tok == "p" || tok == "q", ShouldBeTrue)
		})
	})
}

func BenchmarkSortDescending(b *testing.B) {
	dist := make([]Ranked, 64)

	for idx := range dist {
		dist[idx] = Ranked{Token: string(rune('a' + idx%26)), Probability: float64(idx%7) * 0.01}
	}

	rng := rand.New(rand.NewSource(1))

	b.ResetTimer()

	for b.Loop() {
		rng.Shuffle(len(dist), func(i, j int) {
			dist[i], dist[j] = dist[j], dist[i]
		})

		SortDescending(dist)
	}
}

func BenchmarkSample(b *testing.B) {
	dist := []Ranked{
		{Token: "a", Probability: 0.05},
		{Token: "b", Probability: 0.1},
		{Token: "c", Probability: 0.85},
	}

	rng := rand.New(rand.NewSource(7))

	b.ResetTimer()

	for b.Loop() {
		_ = Sample(dist, 1, rng)
	}
}
