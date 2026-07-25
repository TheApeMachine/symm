package manifold

import (
	"math"

	"github.com/krakenfx/api-go/v2/pkg/book"
	pfluid "github.com/theapemachine/nomagique/physics/fluid"
)

/*
segmentLen mirrors Sensorium universal.SEGMENT_LEN so relative sequence wrap
transfers structure across book samples the same way it transfers across text
samples.
*/
const (
	segmentLen = 256
	// injectHeatFraction is cold inject heat as a fraction of oscillator energy
	// (universal.py make_batch: 1e-4 of energy_osc).
	injectHeatFraction float32 = 1e-4
)

/*
restingOrder is the tokenize input retained after the book lease: side and price
only. Content ω needs nothing else from the SDK order.
*/
type restingOrder struct {
	side  book.BookDirection
	price float64
}

/*
tokenKey is one compressor identity inside a book sample: content at a relative
sequence index. Collision-is-compression aggregates multiplicity into mass.
*/
type tokenKey struct {
	content uint32
	relSeq  int64
	side    book.BookDirection
}

/*
Tokenizer maps one L3 book sample into Sensorium-shaped particles.
Each resting order is one site on a circle of size len(orders); that index
drives ω. Sequence drives phase/grid; Hawkes intensities set oscillator energy
by side; heat is injectHeatFraction of that energy.
*/
type Tokenizer struct {
	config pfluid.Config
}

/*
NewTokenizer binds the shared domain grid used for spatial sequence layout.
*/
func NewTokenizer(config pfluid.Config) Tokenizer {
	return Tokenizer{config: config}
}

/*
Batch is one tokenized book sample: Sensorium particles plus the content
identities inelastic merge uses (content_token_ids).
*/
type Batch struct {
	Particles  []pfluid.Particle
	ContentIDs []uint32
}

/*
MakeBatch converts one book's resting orders into an appendable particle batch.
The content circle has exactly len(orders) sites — one per resting order — and
ω sits at each site's arc center on [Ω_min, Ω_max]. Sequence indices reset at
the book boundary; relative wrap uses segmentLen. Bid particles take
buyIntensity as Energy, ask particles take sellIntensity; Heat is
injectHeatFraction of Energy.
*/
func (tokenizer Tokenizer) MakeBatch(
	orders []restingOrder,
	midPrice float64,
	buyIntensity float64,
	sellIntensity float64,
) Batch {
	if midPrice <= 0 || len(orders) == 0 {
		return Batch{}
	}

	if buyIntensity < 0 || sellIntensity < 0 ||
		math.IsNaN(buyIntensity) || math.IsNaN(sellIntensity) ||
		math.IsInf(buyIntensity, 0) || math.IsInf(sellIntensity, 0) {
		return Batch{}
	}

	universe := uint32(len(orders))
	counts := make(map[tokenKey]float32, len(orders))
	order := make([]tokenKey, 0, len(orders))

	for sequence, resting := range orders {
		key := tokenKey{
			content: uint32(sequence),
			relSeq:  int64(sequence) % segmentLen,
			side:    resting.side,
		}

		if _, seen := counts[key]; !seen {
			order = append(order, key)
		}

		counts[key]++
	}

	batch := Batch{
		Particles:  make([]pfluid.Particle, 0, len(order)),
		ContentIDs: make([]uint32, 0, len(order)),
	}
	grid := tokenizer.config.Grid
	extent := max(grid.X, grid.Y, grid.Z)
	spacing := float32(1.0 / float64(extent))
	span := tokenizer.config.OmegaMax - tokenizer.config.OmegaMin

	for _, key := range order {
		mass := counts[key]

		if mass <= 0 {
			continue
		}

		// N equal arcs on [Ω_min, Ω_max]; site centers — inclusive endpoints
		// leave the resident wave dark (exact Ω_min / Ω_max do not couple).
		omega := tokenizer.config.OmegaMin +
			(float32(key.content)+0.5)/float32(universe)*span
		energy := float32(buyIntensity)

		if key.side == book.Ask {
			energy = float32(sellIntensity)
		}

		batch.Particles = append(batch.Particles, pfluid.Particle{
			Position: sequencePosition(key.relSeq, grid, spacing),
			Velocity: pfluid.Vector{},
			Mass:     mass,
			Heat:     energy * injectHeatFraction,
			Energy:   energy,
			Phase:    positionPhase(key.relSeq, universe),
			Omega:    omega,
		})
		batch.ContentIDs = append(batch.ContentIDs, key.content)
	}

	return batch
}

/*
positionPhase encodes sequence structure into phase while leaving frequency to
content. The beat wraps over the book order count — the same circle that sizes
content→ω — not a fixed text-tokenizer period.
*/
func positionPhase(seq int64, universe uint32) float32 {
	if universe < 1 {
		universe = 1
	}

	rel := float64(seq%segmentLen) / float64(segmentLen)
	beat := math.Mod(float64(seq/segmentLen), float64(universe)) / float64(universe)
	phase := 2*math.Pi*rel + math.Pi*beat

	return float32(math.Mod(phase, 2*math.Pi))
}

/*
sequencePosition places a relative sequence index onto the normalized grid the
same way universal.py spatializes rel_seq.
*/
func sequencePosition(
	relSeq int64,
	grid pfluid.Grid,
	spacing float32,
) pfluid.Vector {
	gx := int64(grid.X)
	gy := int64(grid.Y)
	gz := int64(grid.Z)

	return pfluid.Vector{
		X: float32(relSeq%gx) * spacing,
		Y: float32((relSeq/gx)%gy) * spacing,
		Z: float32((relSeq/(gx*gy))%gz) * spacing,
	}
}
