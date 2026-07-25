package manifold

import (
	"math"
	"sort"

	"github.com/krakenfx/api-go/v2/pkg/book"
	pfluid "github.com/theapemachine/nomagique/physics/fluid"
)

/*
segmentLen mirrors Sensorium universal.SEGMENT_LEN so relative sequence wrap
transfers structure across book samples the same way it transfers across text
samples. The content pack stores sequence in the top byte, so identities repeat
every segmentLen orders and collision-is-compression can raise mass.
*/
const (
	segmentLen = 256
	// injectHeatFraction is cold inject heat as a fraction of oscillator energy
	// (universal.py make_batch: 1e-4 of energy_osc).
	injectHeatFraction float32 = 1e-4
	// symbolIndexMask leaves the LSB free for bid/ask inside the low 24 bits.
	symbolIndexMask uint32 = 0x7fffff
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
tokenKey is one compressor identity inside a book sample. Content already packs
sequence, side, and symbolIndex; sequence/side are retained for ω and energy.
*/
type tokenKey struct {
	content  uint32
	sequence int
	side     book.BookDirection
}

/*
Tokenizer maps one L3 book sample into Sensorium-shaped particles.
Each resting order is one site on a circle of size len(orders); that index
drives ω. Packed content carries sequence×side×symbol for merge; Hawkes
intensities set oscillator energy by side; heat is injectHeatFraction of energy.
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
packContent encodes (sequence << 24) | (symbolIndex << 1) | sideBit.
Sequence occupies the top byte; the low 24 bits hold the alphabetical universe
slot shifted up one bit with bid=0 / ask=1 in the LSB so Metal's content&0xFF
merge still distinguishes sides.
*/
func packContent(
	sequence int,
	symbolIndex uint32,
	side book.BookDirection,
) uint32 {
	sideBit := uint32(0)

	if side == book.Ask {
		sideBit = 1
	}

	return (uint32(sequence) << 24) |
		((symbolIndex & symbolIndexMask) << 1) |
		sideBit
}

/*
universeIndex returns the alphabetical index of symbol in names. names must
already be sorted.
*/
func universeIndex(names []string, symbol string) (uint32, bool) {
	index := sort.SearchStrings(names, symbol)

	if index == len(names) || names[index] != symbol {
		return 0, false
	}

	return uint32(index), true
}

/*
sortedUniverse copies and sorts symbol names into a stable index basis.
*/
func sortedUniverse(names []string) []string {
	universe := append([]string(nil), names...)
	sort.Strings(universe)
	return universe
}

/*
MakeBatch converts one book's resting orders into an appendable particle batch.
Content is (sequence<<24)|(symbolIndex<<1)|side so host counts and Metal merge
share one identity. The ω circle still has len(orders) sites — one per resting
order — with ω at each site's arc center on [Ω_min, Ω_max]. Bid particles take
buyIntensity as Energy, ask particles take sellIntensity; Heat is
injectHeatFraction of Energy.
*/
func (tokenizer Tokenizer) MakeBatch(
	orders []restingOrder,
	midPrice float64,
	buyIntensity float64,
	sellIntensity float64,
	symbolIndex uint32,
) Batch {
	if midPrice <= 0 || len(orders) == 0 {
		return Batch{}
	}

	if buyIntensity < 0 || sellIntensity < 0 ||
		math.IsNaN(buyIntensity) || math.IsNaN(sellIntensity) ||
		math.IsInf(buyIntensity, 0) || math.IsInf(sellIntensity, 0) {
		return Batch{}
	}

	circle := uint32(len(orders))
	counts := make(map[uint32]float32, len(orders))
	order := make([]tokenKey, 0, len(orders))

	for sequence, resting := range orders {
		content := packContent(sequence, symbolIndex, resting.side)

		if _, seen := counts[content]; !seen {
			order = append(order, tokenKey{
				content:  content,
				sequence: sequence,
				side:     resting.side,
			})
		}

		counts[content]++
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
		mass := counts[key.content]

		if mass <= 0 {
			continue
		}

		// N equal arcs on [Ω_min, Ω_max]; site centers — inclusive endpoints
		// leave the resident wave dark (exact Ω_min / Ω_max do not couple).
		omega := tokenizer.config.OmegaMin +
			(float32(key.sequence)+0.5)/float32(circle)*span
		energy := float32(buyIntensity)

		if key.side == book.Ask {
			energy = float32(sellIntensity)
		}

		relSeq := int64(key.sequence) % segmentLen
		batch.Particles = append(batch.Particles, pfluid.Particle{
			Position: sequencePosition(relSeq, grid, spacing),
			Velocity: pfluid.Vector{},
			Mass:     mass,
			Heat:     energy * injectHeatFraction,
			Energy:   energy,
			Phase:    positionPhase(relSeq, circle),
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
func positionPhase(seq int64, circle uint32) float32 {
	if circle < 1 {
		circle = 1
	}

	rel := float64(seq%segmentLen) / float64(segmentLen)
	beat := math.Mod(float64(seq/segmentLen), float64(circle)) / float64(circle)
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
