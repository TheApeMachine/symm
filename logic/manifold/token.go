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
const segmentLen = 256

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
	content uint8
	relSeq  int64
}

/*
Tokenizer maps one L3 book sample into Sensorium-shaped particles.
It stays aligned with universal.py: content→ω, sequence→phase/grid, cold heat,
unit oscillator energy, zero inject velocity, mass from within-batch multiplicity.
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
Sequence indices reset at the book boundary (one sample); relative wrap uses
segmentLen. Hawkes never enters heat — heat is cold on real inject.
*/
func (tokenizer Tokenizer) MakeBatch(
	orders []restingOrder,
	midPrice float64,
) Batch {
	if midPrice <= 0 || len(orders) == 0 {
		return Batch{}
	}

	counts := make(map[tokenKey]float32, len(orders))
	order := make([]tokenKey, 0, len(orders))

	for sequence, resting := range orders {
		content := orderContent(resting, midPrice)
		key := tokenKey{
			content: content,
			relSeq:  int64(sequence) % segmentLen,
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

	for _, key := range order {
		mass := counts[key]

		if mass <= 0 {
			continue
		}

		omega := tokenizer.config.OmegaMin +
			float32(key.content)/255.0*
				(tokenizer.config.OmegaMax-tokenizer.config.OmegaMin)

		batch.Particles = append(batch.Particles, pfluid.Particle{
			Position: sequencePosition(key.relSeq, grid, spacing),
			Velocity: pfluid.Vector{},
			Mass:     mass,
			// Cold inject: universal.py comments 1e-4 of oscillator energy; never
			// Hawkes-derived and never negative.
			Heat:   1e-4,
			Energy: 1,
			Phase:  positionPhase(key.relSeq),
			Omega:  omega,
		})
		batch.ContentIDs = append(batch.ContentIDs, uint32(key.content))
	}

	return batch
}

/*
orderContent discretizes side and log-price versus mid into one byte so content
alone drives ω, matching the universal byte→excitations split.
*/
func orderContent(order restingOrder, midPrice float64) uint8 {
	if midPrice <= 0 || order.price <= 0 {
		return 0
	}

	bin := uint8(math.Round((math.Tanh(math.Log(order.price/midPrice)) + 1) / 2 * 127))

	if order.side == book.Ask {
		return 128 + bin
	}

	return bin
}

/*
positionPhase encodes sequence structure into phase while leaving frequency to
content — the universal._position_phase split.
*/
func positionPhase(seq int64) float32 {
	rel := float64(seq%segmentLen) / float64(segmentLen)
	beat := math.Mod(float64(seq/segmentLen), 32.0) / 32.0
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
