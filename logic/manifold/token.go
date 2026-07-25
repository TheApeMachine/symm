package manifold

import (
	"math"
	"sort"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/book"
	pfluid "github.com/theapemachine/nomagique/physics/fluid"
)

/*
segmentLen bounds the top byte of packed content so identities wrap and
collision-is-compression can raise mass. It is not a spatial stride.
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
restingOrder is the tokenize input retained after the book lease.
*/
type restingOrder struct {
	side     book.BookDirection
	price    float64
	quantity float64
	at       time.Time
}

/*
tokenKey is one compressor identity inside a book sample. Content packs
sequence×side×symbol; geometry fields place the particle on the market axes.
*/
type tokenKey struct {
	content  uint32
	sequence int
	side     book.BookDirection
	price    float64
	quantity float64
	at       time.Time
	ageRank  int
}

/*
Tokenizer maps one L3 book sample into Sensorium-shaped particles.
Position is market geometry (X relative log price, Y log size, Z age rank),
not a text-token raster. Content packs sequence×side×symbol for merge; ω sits
on the book-order circle; Hawkes intensities set Energy by side.
*/
type Tokenizer struct {
	config pfluid.Config
}

/*
NewTokenizer binds the shared domain grid used for spatial layout.
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
Each order is placed by relative log price (X), log size (Y), and empirical
age rank (Z) over this sample — one book tick, one geometric step into the
shared field. Content is (sequence<<24)|(symbolIndex<<1)|side for merge.
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

	logPriceMin, logPriceMax, logSizeMin, logSizeMax, ok := geometryBounds(
		orders, midPrice,
	)

	if !ok {
		return Batch{}
	}

	ranks := orderAgeRanks(orders)
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
				price:    resting.price,
				quantity: resting.quantity,
				at:       resting.at,
				ageRank:  ranks[sequence],
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

		omega := tokenizer.config.OmegaMin +
			(float32(key.sequence)+0.5)/float32(circle)*span
		energy := float32(buyIntensity)

		if key.side == book.Ask {
			energy = float32(sellIntensity)
		}

		batch.Particles = append(batch.Particles, pfluid.Particle{
			Position: marketPosition(
				key.price, midPrice, key.quantity, key.ageRank, len(orders),
				logPriceMin, logPriceMax, logSizeMin, logSizeMax,
				grid, spacing,
			),
			Velocity: pfluid.Vector{},
			Mass:     mass,
			Heat:     energy * injectHeatFraction,
			Energy:   energy,
			Phase:    positionPhase(int64(key.sequence)%segmentLen, circle),
			Omega:    omega,
		})
		batch.ContentIDs = append(batch.ContentIDs, key.content)
	}

	return batch
}

/*
geometryBounds derives this sample's log-price and log-size extents so axis
mapping stays relative to the book, not a static window.
*/
func geometryBounds(
	orders []restingOrder,
	midPrice float64,
) (logPriceMin, logPriceMax, logSizeMin, logSizeMax float64, ok bool) {
	logPriceMin = math.Inf(1)
	logPriceMax = math.Inf(-1)
	logSizeMin = math.Inf(1)
	logSizeMax = math.Inf(-1)

	for _, resting := range orders {
		if resting.price <= 0 || resting.quantity <= 0 {
			return 0, 0, 0, 0, false
		}

		logPrice := math.Log(resting.price / midPrice)
		logSize := math.Log(resting.quantity)
		logPriceMin = min(logPriceMin, logPrice)
		logPriceMax = max(logPriceMax, logPrice)
		logSizeMin = min(logSizeMin, logSize)
		logSizeMax = max(logSizeMax, logSize)
	}

	return logPriceMin, logPriceMax, logSizeMin, logSizeMax, true
}

/*
orderAgeRanks assigns 0..n-1 by ascending timestamp (oldest = 0). Equal times
keep walk order so Z still spans the sample.
*/
func orderAgeRanks(orders []restingOrder) []int {
	indices := make([]int, len(orders))

	for index := range indices {
		indices[index] = index
	}

	sort.SliceStable(indices, func(left, right int) bool {
		return orders[indices[left]].at.Before(orders[indices[right]].at)
	})

	ranks := make([]int, len(orders))

	for rank, index := range indices {
		ranks[index] = rank
	}

	return ranks
}

/*
unitInRange maps value onto [0,1] across an observed sample span. A degenerate
span sits at the midline so a single observation does not pin an axis edge.
*/
func unitInRange(value, minimum, maximum float64) float64 {
	if maximum <= minimum {
		return 0.5
	}

	return (value - minimum) / (maximum - minimum)
}

/*
marketPosition places one resting order on the pilot-wave axes advertised by
the dashboard: X relative log price, Y log size, Z empirical order-age rank.
*/
func marketPosition(
	price, midPrice, quantity float64,
	ageRank, ageCount int,
	logPriceMin, logPriceMax, logSizeMin, logSizeMax float64,
	grid pfluid.Grid,
	spacing float32,
) pfluid.Vector {
	xUnit := unitInRange(math.Log(price/midPrice), logPriceMin, logPriceMax)
	yUnit := unitInRange(math.Log(quantity), logSizeMin, logSizeMax)
	zUnit := 0.5

	if ageCount > 1 {
		zUnit = float64(ageRank) / float64(ageCount-1)
	}

	return pfluid.Vector{
		X: float32(xUnit) * float32(max(grid.X-1, 0)) * spacing,
		Y: float32(yUnit) * float32(max(grid.Y-1, 0)) * spacing,
		Z: float32(zUnit) * float32(max(grid.Z-1, 0)) * spacing,
	}
}

/*
positionPhase encodes walk-sequence structure into phase while frequency stays
on content ω.
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
