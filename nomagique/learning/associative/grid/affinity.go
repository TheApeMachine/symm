package grid

import "math"

/*
affinity is what one quantity's relationship to another is read from: three
channels measured over the bins the two were both observed in.

Directional says whether they move the same way, consistency says whether they
keep doing so, and magnitude says whether they move by comparable amounts. They
answer different questions and a single correlation conflates them: two
quantities can agree in direction every time while one barely moves, and two
can have a strong average product from a handful of violent disagreements.

Shared is how many bins the reading rests on. A relationship measured over one
bin is not the same claim as one measured over the whole window, and nothing
downstream may treat them alike.
*/
type affinity struct {
	directional float64
	consistency float64
	magnitude   float64
	shared      int
}

/*
strength is how strongly the two quantities are related, without saying in
which direction. The channels are fused as the diagram composes them, on their
own scales: directional and consistency are signed and enter by magnitude,
while magnitude is already unsigned.
*/
func (reading affinity) strength() float64 {
	return (math.Abs(reading.directional) +
		math.Abs(reading.consistency) + reading.magnitude) / 3
}

/*
stable reports that the two quantities keep the same relationship rather than
having averaged one out of disagreement.

Consistency is the mean agreement of their signs, so it is +1 when they always
move together, -1 when they always move oppositely, and 0 when the relationship
is noise. Both extremes are relationships — an inverse relationship is a
relationship — and only the middle is instability, which is why the test is on
its magnitude.
*/
func (reading affinity) stable() bool {
	return math.Abs(reading.consistency) > 0.5
}

/*
orientation says whether the relationship is direct or inverse, taken from the
channel that measures direction rather than from the fused strength.
*/
func (reading affinity) orientation() float64 {
	if reading.directional < 0 {
		return -1
	}

	return 1
}

/*
measure reads the three channels for one pair of quantities over the bins that
observed both.

Both quantities are already standardized against their own behaviour across the
window, so this is one pass over the bins they share. A pair with fewer than two
shared bins returns no reading at all: silence about a pair is not evidence that
they are unrelated, and neither is a single coincidence.
*/
func (retained *window) measure(left, right int) affinity {
	retained.refresh()

	if !retained.varies[left] || !retained.varies[right] {
		return affinity{shared: retained.shared(left, right)}
	}
	reading := affinity{}

	for bin := range retained.slots {
		if !retained.present[bin][left] || !retained.present[bin][right] {
			continue
		}
		standardLeft := retained.standard[bin][left]
		standardRight := retained.standard[bin][right]
		reading.directional += standardLeft * standardRight
		reading.consistency += sign(standardLeft) * sign(standardRight)
		reading.magnitude += math.Abs(standardLeft) * math.Abs(standardRight)
		reading.shared++
	}

	if reading.shared < 2 {
		return affinity{shared: reading.shared}
	}
	count := float64(reading.shared)
	reading.directional /= count
	reading.consistency /= count
	reading.magnitude /= count

	return reading
}

/* shared counts the bins that observed both quantities. */
func (retained *window) shared(left, right int) int {
	count := 0

	for bin := range retained.slots {
		if retained.present[bin][left] && retained.present[bin][right] {
			count++
		}
	}

	return count
}

/*
sign is the sign field the consistency channel is built from. An exact zero
carries no direction, and giving it one would invent agreement or disagreement
where the quantity simply sat still.
*/
func sign(value float64) float64 {
	switch {
	case value > 0:
		return 1
	case value < 0:
		return -1
	default:
		return 0
	}
}
