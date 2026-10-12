package core

import (
	"math"
	"math/rand/v2"
	"slices"
	"sort"
)

/*
Ranks summarizes one stream's observations for empirical-rank queries: how
much of the stream lies below, and how much equals, a value. Its error is
measured, not assumed, and held to Tolerance of the empirical distribution's
own sampling error, so the summary is never the reason a rank is uncertain.

Level h holds items of weight 2^h. Level 0 is the unsorted arrival buffer;
every higher level is sorted. Compacting a level sorts it, keeps every other
item from a random offset at double weight in the level above, and leaves an
odd item behind, so the total weight always equals the observation count.
For any query, one compaction of level h changes the weighted count below it
by 0 or +-2^h with mean zero, so its error variance is at most 4^h; Variance
sums that bound over every compaction.

The empirical distribution of n observations counts the values below the
median with binomial standard deviation sqrt(n/4). A compaction is admitted
only while Variance stays within (Tolerance * sqrt(n/4))^2, so a stream too
short to afford the error is summarized exactly.

The random offsets come from a seeded generator: the same observations under
the same seed always produce the same summary, so a replay reproduces the
live ranks.
*/
type Ranks struct {
	levels   [][]float64
	count    float64
	variance float64
	seed     uint64
	random   *rand.Rand
}

/*
NewRanks returns an empty summary whose compaction offsets derive from seed.
*/
func NewRanks(seed uint64) Ranks {
	return Ranks{seed: seed}
}

/*
Count is the number of observations the summary holds.
*/
func (ranks *Ranks) Count() float64 {
	return ranks.count
}

/*
Variance is the upper bound on the error variance of every weighted count
Rank returns.
*/
func (ranks *Ranks) Variance() float64 {
	return ranks.variance
}

/*
Rank returns the weighted counts of the held observations below value and
equal to it.
*/
func (ranks *Ranks) Rank(value float64) (below float64, equal float64) {
	for height, level := range ranks.levels {
		weight := math.Ldexp(Unit, height)

		if height == 0 {
			for _, item := range level {
				if item < value {
					below += weight
				}

				if item == value {
					equal += weight
				}
			}

			continue
		}

		lower := sort.SearchFloat64s(level, value)
		upper := sort.Search(len(level), func(index int) bool {
			return level[index] > value
		})

		below += weight * float64(lower)
		equal += weight * float64(upper-lower)
	}

	return below, equal
}

/*
Add incorporates value, then compacts while the error budget admits it.

Holding level h at length L_h, compacting it each time it fills gives an
error-variance rate proportional to 2^h / L_h; the lengths that minimize the
summary's size for a given total rate grow as sqrt(2^h). The level furthest
past that shape, the largest L_h / sqrt(2^h), is therefore the one compacted,
and when its cost does not fit the budget the summary waits for the budget to
grow rather than spending it on a less efficient level.
*/
func (ranks *Ranks) Add(value float64) {
	if len(ranks.levels) == 0 {
		ranks.levels = append(ranks.levels, nil)
	}

	ranks.levels[0] = append(ranks.levels[0], value)
	ranks.count++

	budget := Tolerance * Tolerance * ranks.count / 4

	for {
		height := ranks.fullest()

		if height < 0 {
			return
		}

		cost := math.Ldexp(Unit, 2*height)

		if ranks.variance+cost > budget {
			return
		}

		ranks.compact(height)
		ranks.variance += cost
	}
}

/*
fullest returns the level furthest past the size-minimizing shape, or -1 when
no level holds a pair to compact.
*/
func (ranks *Ranks) fullest() int {
	fullest := -1
	excess := 0.0

	for height, level := range ranks.levels {
		if len(level) < 2 {
			continue
		}

		score := float64(len(level)) / math.Sqrt(math.Ldexp(Unit, height))

		if score > excess {
			fullest, excess = height, score
		}
	}

	return fullest
}

/*
compact promotes every other item of level height, from a random offset, to
the level above at double weight, leaving an odd item behind.
*/
func (ranks *Ranks) compact(height int) {
	if ranks.random == nil {
		ranks.random = rand.New(rand.NewPCG(ranks.seed, ranks.seed))
	}

	level := ranks.levels[height]

	if height == 0 {
		slices.Sort(level)
	}

	var left []float64

	if len(level)%2 == 1 {
		left = append(left, level[len(level)-1])
		level = level[:len(level)-1]
	}

	promoted := make([]float64, 0, len(level)/2)

	for index := ranks.random.IntN(2); index < len(level); index += 2 {
		promoted = append(promoted, level[index])
	}

	if height+1 == len(ranks.levels) {
		ranks.levels = append(ranks.levels, nil)
	}

	ranks.levels[height] = left
	ranks.levels[height+1] = merge(ranks.levels[height+1], promoted)
}

/*
merge returns the sorted union of two sorted slices.
*/
func merge(left []float64, right []float64) []float64 {
	merged := make([]float64, 0, len(left)+len(right))
	leftIndex, rightIndex := 0, 0

	for leftIndex < len(left) && rightIndex < len(right) {
		if left[leftIndex] <= right[rightIndex] {
			merged = append(merged, left[leftIndex])
			leftIndex++

			continue
		}

		merged = append(merged, right[rightIndex])
		rightIndex++
	}

	merged = append(merged, left[leftIndex:]...)

	return append(merged, right[rightIndex:]...)
}
