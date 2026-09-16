package impulse

import (
	"cmp"
	"hash/fnv"
	"slices"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/geometry"
	"github.com/theapemachine/symm/nomagique/learning/associative/grid"
	"github.com/theapemachine/symm/nomagique/statistic"
)

/* Market owns the topology and clock for one symbol, never a table of values. */
type Market struct {
	At              time.Time
	Symbol          string
	Sequence        int64
	Volume          *decimal.Decimal
	Cells           []*Cell
	Impulse         grid.Impulse
	owners          map[string]*owner
	addresses       map[[2]string]*Cell
	pairs           []statistic.Concordance
	edges           []geometry.Edge
	points          []*geometry.Point
	regions         []grid.Region
	keys            []string
	volumeIncrement float64
	valid           bool
}

func newMarket(symbol string) *Market {
	return &Market{Symbol: symbol, Volume: decimal.NewFromInt64(0),
		owners: make(map[string]*owner), addresses: make(map[[2]string]*Cell)}
}

/* Bind replaces an address's publication, without writing or copying its values. */
func (market *Market) Bind(measurement *data.Measurement[float64]) error {
	name := measurement.Provenance["owner"]

	if name == "" {
		return errnie.Error(errnie.Err(errnie.Validation, "impulse: measurement has no producer identity", nil))
	}

	held := market.owners[name]

	if held == nil {
		held = &owner{}
		market.owners[name] = held
	}

	if held.measurement != nil && held.sequence == measurement.SeqIdx {
		return errnie.Error(errnie.Err(errnie.Conflict, "impulse: duplicate producer in one sequence: "+name, nil))
	}

	if measurement.At.After(market.At) {
		market.At = measurement.At
	}
	held.measurement = measurement
	held.sequence = measurement.SeqIdx
	market.keys = market.keys[:0]

	for key := range measurement.Metrics {
		if market.addresses[[2]string{name, key}] == nil {
			market.keys = append(market.keys, key)
		}
	}

	slices.Sort(market.keys)

	for _, key := range market.keys {
		if err := market.add(name, key, held); err != nil {
			return err
		}
	}

	return market.advance(measurement)
}

func (market *Market) add(name, key string, held *owner) error {
	hash := fnv.New64a()
	// NUL separates identity components; the token representation permits 48 bits.
	for _, component := range []string{name, "\x00", key} {
		if _, err := hash.Write([]byte(component)); err != nil {
			return errnie.Error(err)
		}
	}
	identity := hash.Sum64() & ((1 << 48) - 1)

	if identity == 0 {
		return errnie.Error(errnie.Err(errnie.Conflict, "impulse: zero coordinate identity", nil))
	}

	for _, cell := range market.Cells {
		if cell.ID == identity {
			return errnie.Error(errnie.Err(errnie.Conflict, "impulse: coordinate identity collision", nil))
		}
	}

	cell := &Cell{ID: identity, Owner: name, Metric: key, owner: held}
	// Hash halves seed a deterministic unit square, independent of admission order.
	cell.Position.X = float64(identity&((1<<24)-1)) / (1 << 24)
	cell.Position.Y = float64(identity>>24) / (1 << 24)
	market.addresses[[2]string{name, key}] = cell
	market.Cells = append(market.Cells, cell)
	return nil
}

/* prepare rebuilds address-only scratch storage when the schema grows. */
func (market *Market) prepare() {
	if len(market.points) == len(market.Cells) {
		return
	}

	previous := make(map[[2]uint64]statistic.Concordance, len(market.pairs))

	for index, edge := range market.edges {
		previous[[2]uint64{market.Cells[edge.Left].ID, market.Cells[edge.Right].ID}] = market.pairs[index]
	}

	slices.SortFunc(market.Cells, func(left, right *Cell) int {
		if ordering := cmp.Compare(left.Owner, right.Owner); ordering != 0 {
			return ordering
		}
		return cmp.Compare(left.Metric, right.Metric)
	})
	count := len(market.Cells)
	market.points = make([]*geometry.Point, count)
	market.edges = make([]geometry.Edge, count*(count-1)/2)
	market.pairs = make([]statistic.Concordance, len(market.edges))
	market.regions = make([]grid.Region, count)

	for right, cell := range market.Cells {
		market.points[right] = &cell.Position
		cell.Position.Basin = right

		for left := 0; left < right; left++ {
			index := right*(right-1)/2 + left
			market.edges[index] = geometry.Edge{Left: left, Right: right}
			market.pairs[index] = previous[[2]uint64{market.Cells[left].ID, cell.ID}]
		}
	}
}

func (market *Market) advance(measurement *data.Measurement[float64]) error {
	if measurement.Err != nil {
		return nil
	}

	if measurement.Provenance["channel"] != "trade" || measurement.Metadata["venue"] != "true" || measurement.Metadata["volume-unit"] != "base" {
		return nil
	}

	quantity := measurement.Metrics["qty"].Exact

	if quantity == nil || quantity.Sign() <= 0 {
		return errnie.Error(errnie.Err(errnie.Validation, "impulse: trade requires its original positive decimal quantity", nil))
	}

	if quantity.GetScale() > market.Volume.GetScale() {
		market.Volume = market.Volume.SetScale(quantity.GetScale())
	}
	market.Volume = market.Volume.Add(quantity)
	market.volumeIncrement += measurement.Metrics["qty"].Raw
	return nil
}
