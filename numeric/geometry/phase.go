package geometry

import "math"

/*
Phase provides phase-space coupling and velocity computations for
surprisal dynamics. Two nodes whose surprisal velocities move in the
same direction are phase-coupled; the strength of that coupling
determines whether the field treats them as part of the same
emergent eigenmode.
*/
type Phase struct{}

/*
NewPhase constructs a Phase instance.
*/
func NewPhase() *Phase {
	return &Phase{}
}

/*
Coupling is directional alignment of two surprisal velocities, in [-1,+1].
With magnitudes large enough to clear magEps, (left*right)/(|left|*|right|)
collapses to sign(left)*sign(right): co-moving growth is +1, opposing signs
−1, and either side ~0 yields ~0. Field code treats this as a sign factor
in weights such as coupling * (1 + phaseCoupling): +1 boosts, −1 dampens.

magEps (0.01) gates out near-zero geometric mean so quiescent nodes do not
inject noisy coupling.
*/
func (phase *Phase) Coupling(leftGrowth float64, rightGrowth float64) float64 {
	const magEps = 0.01

	geometricMean := math.Sqrt(math.Abs(leftGrowth) * math.Abs(rightGrowth))

	if geometricMean < magEps {
		return 0
	}

	return (leftGrowth * rightGrowth) / (geometricMean * geometricMean)
}

/*
Velocity returns the phase velocity of a node given its current and
previous surprisal means. Positive velocity means surprisal is
increasing (the node is encountering more novel input); negative
means it is converging.
*/
func (phase *Phase) Velocity(surprisalMean float64, surprisalPrev float64) float64 {
	return surprisalMean - surprisalPrev
}
