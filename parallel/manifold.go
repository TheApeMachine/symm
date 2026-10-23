package parallel

import (
	"math"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/logic/manifold"
	"github.com/theapemachine/symm/nomagique/physics/sensorium"
	nmstore "github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/system"
)

/*
Field wraps the sensorium manifold for region chords and vacuum null steps.
It stays inside the parallel package and never feeds the incumbent strategy.
*/
type Field struct {
	manifold    *sensorium.Manifold
	null        *sensorium.State
	nullReading sensorium.Reading
	tick        int64
	reading     sensorium.Reading
	deltaR      float64
	healthy     bool
	initErr     error
}

func NewField() *Field {
	cfg := system.Cfg.Manifold
	manifold := sensorium.NewManifold(cfg.Grid.X, cfg.Grid.Y, cfg.Grid.Z)
	if manifold == nil {
		return &Field{healthy: false, initErr: errnie.Err(errnie.Internal, "parallel: sensorium manifold unavailable (Metal init failed)", nil)}
	}

	field := &Field{
		manifold: manifold,
		healthy:  true,
	}

	if manifold.Tokenizer != nil {
		field.null = manifold.Tokenizer.MakeDarkBatch(0, 1e-4)
		field.bootstrap()
	}

	return field
}

func (field *Field) bootstrap() {
	if field == nil || field.manifold == nil || field.null == nil {
		return
	}

	field.manifold.AddBatch(field.null)

	if _, err := field.manifold.Step(field.null); err != nil {
		field.healthy = false
		field.initErr = err
		errnie.Error(errnie.Err(errnie.Internal, "parallel: vacuum bootstrap step failed", err))
		return
	}

	field.nullReading = field.manifold.Reading()
	field.reading = field.nullReading
}

func (field *Field) Step(regions []*nmstore.Region, seq int64, anomalyHeat float64) sensorium.Reading {
	if field == nil || !field.healthy || field.manifold == nil {
		return sensorium.Reading{}
	}

	field.tick = seq
	priorR := field.reading.KuramotoR

	batch := regionsToBatch(field.manifold, regions, seq, anomalyHeat)
	if batch == nil {
		return field.reading
	}

	if _, err := field.manifold.Step(batch); err != nil {
		field.healthy = false
		errnie.Error(errnie.Err(errnie.Internal, "parallel: live manifold step failed", err))
		return field.reading
	}

	field.reading = field.manifold.Reading()
	field.deltaR = field.reading.KuramotoR - priorR

	return field.reading
}

func (field *Field) Reading() sensorium.Reading {
	if field == nil {
		return sensorium.Reading{}
	}

	return field.reading
}

func (field *Field) DeltaR() float64 {
	if field == nil {
		return 0
	}

	return field.deltaR
}

func (field *Field) NullReading() sensorium.Reading {
	if field == nil {
		return sensorium.Reading{}
	}

	return field.nullReading
}

func (field *Field) Healthy() bool {
	return field != nil && field.healthy && field.reading.Health.IsFinite()
}

func (field *Field) InitError() error {
	if field == nil {
		return nil
	}

	return field.initErr
}

/*
ManifoldFrame snapshots the resident Metal domain for the fluid viewer.
*/
func (field *Field) ManifoldFrame() *manifold.State {
	if field == nil || !field.healthy || field.manifold == nil {
		return nil
	}

	version := uint64(field.tick)
	if version == 0 {
		version = 1
	}

	return manifold.SnapshotHost(field.manifold, version, "")
}

func (field *Field) SpectralHint() map[string]any {
	if field == nil || field.manifold == nil {
		return nil
	}

	omega, real, imag, linewidth := field.manifold.SpectralModes()
	if len(omega) == 0 {
		return nil
	}

	best := 0
	bestMag := float32(0)

	for index := range omega {
		mag := real[index]*real[index] + imag[index]*imag[index]
		if mag > bestMag {
			bestMag = mag
			best = index
		}
	}

	return map[string]any{
		"omega":     omega[best],
		"linewidth": linewidth[best],
		"magnitude": math.Sqrt(float64(bestMag)),
	}
}

func regionsToBatch(
	manifold *sensorium.Manifold,
	regions []*nmstore.Region,
	seq int64,
	anomalyHeat float64,
) *sensorium.State {
	if manifold == nil || manifold.Tokenizer == nil || len(regions) == 0 {
		return nil
	}

	lits := make([]sensorium.RegionLit, 0, len(regions))
	for _, region := range regions {
		if region == nil || region.Brightness <= 0 {
			continue
		}

		lits = append(lits, sensorium.RegionLit{
			Key:        region.Key,
			Brightness: region.Brightness,
		})
	}

	if len(lits) == 0 {
		return nil
	}

	state, err := manifold.Tokenizer.MakeRegionLitBatch(seq, lits, anomalyHeat)
	if err != nil || state == nil {
		return nil
	}

	return state
}

func globalPhysicsFeatures(reading sensorium.Reading, phase SymbolPhase) []float64 {
	guidanceSign := 0.0
	if phase.Guidance > 0 {
		guidanceSign = 1
	} else if phase.Guidance < 0 {
		guidanceSign = -1
	}

	speed := reading.GuidanceSpeed
	if math.IsNaN(speed) || math.IsInf(speed, 0) {
		speed = 0
	}

	return []float64{
		reading.KuramotoR,
		reading.KuramotoPsi,
		phase.R,
		phase.DeltaR,
		reading.CoherenceMag2,
		speed,
		guidanceSign,
		phase.Heat,
	}
}
