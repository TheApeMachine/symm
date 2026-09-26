package store

import (
	"context"
	"math"
	"sort"

	"github.com/theapemachine/symm/nomagique/internal/dependence"

	"github.com/theapemachine/errnie"
)

/* RelationsServer exclusively owns causally stamped adaptive market histories. */
type RelationsServer struct {
	paths           map[string]*priceHistory
	keys            []string
	key             string
	epoch, sequence int64
}

/* NewRelations creates an empty explicit price store. */
func NewRelations() *RelationsServer {
	return &RelationsServer{paths: make(map[string]*priceHistory)}
}

/* Write admits one spot price; epochs clear histories and timestamps never regress. */
func (server *RelationsServer) Write(ctx context.Context, call Relations_write) error {
	args := call.Args()
	key, err := args.Key()

	if err != nil {
		return errnie.Error(err)
	}

	if key == "" || args.Price() <= 0 || args.Timestamp() < 0 || args.Epoch() < 0 || args.Sequence() < 0 {
		return errnie.Error(errnie.Err(errnie.Validation, "relations: market, positive price and nonnegative stamps required", nil))
	}

	if args.Epoch() < server.epoch || args.Epoch() == server.epoch && args.Sequence() < server.sequence {
		return errnie.Error(errnie.Err(errnie.Validation, "relations: causal observation regressed", nil))
	}

	if args.Epoch() != server.epoch {
		clear(server.paths)
		server.keys = nil
	}
	path := server.paths[key]

	if path == nil {
		path = &priceHistory{}
		server.paths[key] = path
		position := sort.SearchStrings(server.keys, key)
		server.keys = append(server.keys, "")
		copy(server.keys[position+1:], server.keys[position:])
		server.keys[position] = key
	}

	if err := path.append(int64(args.Timestamp()), args.Price()); err != nil {
		return err
	}
	path.measured.Measure()
	path.epoch, path.sequence = args.Epoch(), args.Sequence()
	server.key, server.epoch, server.sequence = key, args.Epoch(), args.Sequence()
	return nil
}

/* priceHistory applies the LEGACY Path/Window retention policy to one market. */
type priceHistory struct {
	measured               dependence.Path
	all, recent            pathMoments
	observations, capacity float64
	epoch, sequence        int64
}

/* append replaces equal timestamps and sheds only after a measured mean shift. */
func (path *priceHistory) append(at int64, value float64) error {
	count := len(path.measured.Points)

	if count > 0 && at < path.measured.Points[count-1].At {
		return errnie.Error(errnie.Err(errnie.Validation, "relations: market timestamp regressed", nil))
	}

	if count > 0 && at == path.measured.Points[count-1].At {
		path.measured.Points[count-1].Value = value
	}

	if count == 0 || at > path.measured.Points[count-1].At {
		path.measured.Points = append(path.measured.Points, dependence.Point{At: at, Value: value})
	}
	ratio := path.retain(value)

	if ratio < 1 && len(path.measured.Points) > 2 {
		retained := max(2, int(math.Floor(float64(len(path.measured.Points))*ratio)))
		copy(path.measured.Points, path.measured.Points[len(path.measured.Points)-retained:])
		path.measured.Points = path.measured.Points[:retained]
	}
	return nil
}

/*
	retain uses the all/recent moment mean-shift bound from LEGACY adaptive.Window.

The halves are the two samples being compared, not a fixed market window.
*/
func (path *priceHistory) retain(value float64) float64 {
	path.observations++
	path.capacity++
	path.all.update(value)
	path.recent.update(value)

	if path.observations > 3 && path.recent.count > path.capacity/2 {
		path.recent.shed(.5)
	}
	prior := path.capacity - path.recent.count

	if path.observations <= 3 || path.recent.count <= 1 || prior <= 1 || path.all.m2 <= 0 {
		return 1
	}
	variance := path.all.m2 / (path.all.count - 1)
	bound := math.Sqrt(variance * math.Log(4*path.observations*path.observations) * (.5 * (1/path.recent.count + 1/prior)))

	if math.Abs(path.recent.mean-path.all.mean) <= bound {
		return 1
	}
	capacity := max(1, math.Floor(path.capacity/2))
	ratio := capacity / path.capacity
	path.capacity = capacity
	path.all.shed(ratio)
	path.recent = pathMoments{}
	return ratio
}

/* pathMoments stores the fixed-size sufficient statistics of the retention test. */
type pathMoments struct{ count, mean, m2 float64 }

func (moments *pathMoments) update(value float64) {
	moments.count++
	delta := value - moments.mean
	moments.mean += delta / moments.count
	moments.m2 += delta * (value - moments.mean)
}
func (moments *pathMoments) shed(ratio float64) {
	if moments.count <= 2 {
		return
	}
	count := max(2, moments.count*ratio)
	moments.m2 *= (count - 1) / (moments.count - 1)
	moments.count = count
}
