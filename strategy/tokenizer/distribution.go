package tokenizer

import "math"

/*
Distribution is a byte histogram for MDL segment costs and Shannon density.
*/
type Distribution struct {
	counts   [VocabSize]int
	total    int
	distinct int
}

func NewDistribution() *Distribution {
	return &Distribution{}
}

func (dist *Distribution) Add(b byte) {
	if dist.counts[b] == 0 {
		dist.distinct++
	}

	dist.counts[b]++
	dist.total++
}

func (dist *Distribution) Remove(b byte) {
	if dist.counts[b] <= 0 {
		return
	}

	dist.counts[b]--
	dist.total--

	if dist.counts[b] == 0 {
		dist.distinct--
	}
}

func (dist *Distribution) N() int {
	return dist.total
}

func (dist *Distribution) NumDistinct() int {
	return dist.distinct
}

func (dist *Distribution) Cost() float64 {
	if dist.total == 0 {
		return 0
	}

	cost := 0.0
	n := float64(dist.total)

	for _, count := range dist.counts {
		if count == 0 {
			continue
		}

		p := float64(count) / n
		cost -= float64(count) * math.Log(p)
	}

	return cost
}

func (dist *Distribution) ShannonDensity() float64 {
	if dist.total == 0 {
		return 0
	}

	entropy := 0.0
	n := float64(dist.total)

	for _, count := range dist.counts {
		if count == 0 {
			continue
		}

		p := float64(count) / n
		entropy -= p * math.Log(p)
	}

	ceiling := math.Log(VocabSize)

	if ceiling <= 0 {
		return 0
	}

	return entropy / ceiling
}

func (dist *Distribution) Clone() *Distribution {
	out := &Distribution{
		total:    dist.total,
		distinct: dist.distinct,
	}
	copy(out.counts[:], dist.counts[:])

	return out
}

func (dist *Distribution) AddFrom(other *Distribution) {
	if other == nil {
		return
	}

	for value, count := range other.counts {
		if count == 0 {
			continue
		}

		if dist.counts[value] == 0 {
			dist.distinct++
		}

		dist.counts[value] += count
		dist.total += count
	}
}
