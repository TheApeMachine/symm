package equation

import (
	"time"

	"github.com/theapemachine/symm/nomagique/types"
)

type multivariateDivergenceState struct {
	joint    JointEstimator
	vel0     LocalRegression
	vel1     LocalRegression
	vel2     LocalRegression
	lastSec  float64
	lastNsec float64
	hasTime  bool
}

/*
MultivariateDivergence is a 3-channel joint log-space baseline and divergence
velocity estimator equation. It composes JointEstimator and LocalRegression
across 3 continuous channels.
*/
type MultivariateDivergence struct {
	Key func() string

	states map[string]*multivariateDivergenceState
	single multivariateDivergenceState
	active *multivariateDivergenceState

	values [3]float64
	at     time.Time
}

/*
NewMultivariateDivergence constructs a new MultivariateDivergence equation.
*/
func NewMultivariateDivergence(key func() string) *MultivariateDivergence {
	return &MultivariateDivergence{
		Key:    key,
		states: make(map[string]*multivariateDivergenceState),
	}
}

func (md *MultivariateDivergence) SetObservation(values [3]float64, at time.Time) {
	md.values = values
	md.at = at
}

func (md *MultivariateDivergence) Step(carrier types.Scalar) types.Scalar {
	state := md.resolveState()
	md.active = state

	sec := float64(md.at.Unix())
	nsec := float64(md.at.Nanosecond())

	if state.hasTime {
		if sec < state.lastSec || (sec == state.lastSec && nsec < state.lastNsec) {
			return carrier
		}
	}

	state.lastSec = sec
	state.lastNsec = nsec
	state.hasTime = true

	state.joint.Step(md.values, sec, nsec)

	nano := md.at.UnixNano()
	horizon := state.joint.Horizon()

	state.vel0.Step(state.joint.Residual(0), nano, horizon)
	state.vel1.Step(state.joint.Residual(1), nano, horizon)
	state.vel2.Step(state.joint.Residual(2), nano, horizon)

	return carrier
}

func (md *MultivariateDivergence) Joint() *JointEstimator {
	if md.active != nil {
		return &md.active.joint
	}

	return &md.single.joint
}

func (md *MultivariateDivergence) Vel(index int) *LocalRegression {
	state := md.active

	if state == nil {
		state = &md.single
	}

	switch index {
	case 0:
		return &state.vel0
	case 1:
		return &state.vel1
	case 2:
		return &state.vel2
	default:
		return nil
	}
}

func (md *MultivariateDivergence) Readings() []types.Reading {
	joint := md.Joint()
	readings := make([]types.Reading, 0, 22)

	if !joint.HasMean() {
		return readings
	}

	hasPrior := joint.count > 1

	add := func(label string, val float64, defined bool, unit, timescale string) {
		readings = append(readings, types.Reading{
			Label:     label,
			Value:     types.Scalar(val),
			Defined:   defined,
			Unit:      unit,
			Timescale: timescale,
		})
	}

	add("touch_notional_baseline:bid", joint.Baseline(0), hasPrior, "rate", "instantaneous")
	add("touch_notional_baseline:ask", joint.Baseline(1), hasPrior, "rate", "instantaneous")
	add("spread_baseline", joint.Baseline(2), hasPrior, "rate", "instantaneous")

	add("depth_ratio:bid", joint.Ratio(0), hasPrior, "dimensionless", "instantaneous")
	add("depth_ratio:ask", joint.Ratio(1), hasPrior, "dimensionless", "instantaneous")
	add("spread_ratio", joint.Ratio(2), hasPrior, "dimensionless", "instantaneous")

	add("depth_divergence:bid", joint.Residual(0), hasPrior, "dimensionless", "instantaneous")
	add("depth_divergence:ask", joint.Residual(1), hasPrior, "dimensionless", "instantaneous")
	add("spread_divergence", joint.Residual(2), hasPrior, "dimensionless", "instantaneous")

	noise0, hasNoise0 := joint.Noise(0)
	add("depth_noise_scale:bid", noise0, hasNoise0, "dimensionless", "instantaneous")
	noise1, hasNoise1 := joint.Noise(1)
	add("depth_noise_scale:ask", noise1, hasNoise1, "dimensionless", "instantaneous")
	noise2, hasNoise2 := joint.Noise(2)
	add("spread_noise_scale", noise2, hasNoise2, "dimensionless", "instantaneous")

	z0, hasZ0 := joint.ZScore(0)
	add("depth_zscore:bid", z0, hasZ0, "dimensionless", "instantaneous")
	z1, hasZ1 := joint.ZScore(1)
	add("depth_zscore:ask", z1, hasZ1, "dimensionless", "instantaneous")
	z2, hasZ2 := joint.ZScore(2)
	add("spread_zscore", z2, hasZ2, "dimensionless", "instantaneous")

	slope0, hasSlope0 := md.Vel(0).Slope()
	add("divergence_velocity:bid", slope0, hasSlope0, "per_second", "per_second")
	snr0, hasSNR0 := md.Vel(0).SNR()
	add("divergence_velocity_snr:bid", snr0, hasSNR0, "dimensionless", "instantaneous")

	slope1, hasSlope1 := md.Vel(1).Slope()
	add("divergence_velocity:ask", slope1, hasSlope1, "per_second", "per_second")
	snr1, hasSNR1 := md.Vel(1).SNR()
	add("divergence_velocity_snr:ask", snr1, hasSNR1, "dimensionless", "instantaneous")

	slope2, hasSlope2 := md.Vel(2).Slope()
	add("spread_divergence_velocity", slope2, hasSlope2, "per_second", "per_second")
	snr2, hasSNR2 := md.Vel(2).SNR()
	add("spread_divergence_velocity_snr", snr2, hasSNR2, "dimensionless", "instantaneous")

	return readings
}

func (md *MultivariateDivergence) Support() float64 {
	return md.Joint().NEff()
}

func (md *MultivariateDivergence) Divergence() types.Scalar {
	return types.Scalar(md.Joint().Residual(0))
}

func (md *MultivariateDivergence) NoiseVariance() types.Scalar {
	noise, hasNoise := md.Joint().Noise(0)

	if !hasNoise || noise <= 0 {
		return 0
	}

	return types.Scalar(noise * noise)
}

func (md *MultivariateDivergence) resolveState() *multivariateDivergenceState {
	key := ""

	if md.Key != nil {
		key = md.Key()
	}

	if key != "" {
		if md.states == nil {
			md.states = make(map[string]*multivariateDivergenceState)
		}

		st, found := md.states[key]

		if !found {
			st = &multivariateDivergenceState{}
			md.states[key] = st
		}

		return st
	}

	return &md.single
}

var (
	_ types.Node     = (*MultivariateDivergence)(nil)
	_ types.Reporter = (*MultivariateDivergence)(nil)
	_ types.Evidence = (*MultivariateDivergence)(nil)
)
