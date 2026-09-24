package geometry

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"strconv"

	"github.com/theapemachine/errnie"
)

/*
peakWidth is the layout of one point's retained record: where it drains, the
peaks it reached on the previous two evaluations, and its peak in the last
partition that held.
*/
const peakWidth = 4

/*
PeakServer reads regions off an arranged landscape of points: the watershed
basins of their authority, and whether that partition has settled.
*/
type PeakServer struct {
	uphill   []int
	peak     []int
	previous []int
	regions  []int
}

func NewPeak() *PeakServer {
	return &PeakServer{}
}

func (server *PeakServer) Write(ctx context.Context, call Peak_write) error {
	args := call.Args()
	positions, err := args.Positions()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "geometry.peak: failed to read positions", err))
	}

	authority, err := args.Authority()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "geometry.peak: failed to read authority", err))
	}

	from, err := args.FromNodes()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "geometry.peak: failed to read fromNodes", err))
	}

	to, err := args.ToNodes()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "geometry.peak: failed to read toNodes", err))
	}

	strength, err := args.Strength()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "geometry.peak: failed to read strength", err))
	}

	prior, err := args.Prior()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "geometry.peak: failed to read prior", err))
	}

	known, err := args.Known()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "geometry.peak: failed to read known", err))
	}

	count := authority.Len()

	if positions.Len() != count*2 || prior.Len() != known.Len()*peakWidth ||
		to.Len() != from.Len() || strength.Len() != from.Len() {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			fmt.Sprintf(
				"geometry.peak: %d positions for %d points, %d prior numbers for %d records, %d edges with %d ends and %d strengths",
				positions.Len(), count, prior.Len(), known.Len(), from.Len(), to.Len(), strength.Len(),
			),
			nil,
		))
	}

	near := func(left, right int) bool {
		return math.Hypot(
			positions.At(right*2)-positions.At(left*2),
			positions.At(right*2+1)-positions.At(left*2+1),
		) < 1
	}

	server.uphill = make([]int, count)

	for point := range count {
		server.uphill[point] = -1

		if point < known.Len() && known.At(point) {
			server.uphill[point] = int(prior.At(point * peakWidth))
		}
	}

	for edge := range from.Len() {
		left, right := int(from.At(edge)), int(to.At(edge))

		if left < 0 || right < 0 || left >= count || right >= count {
			return errnie.Error(errnie.Err(
				errnie.Validation,
				fmt.Sprintf("geometry.peak: edge %d-%d is outside %d points", left, right, count),
				nil,
			))
		}

		sympathetic := strength.At(edge) > 0 && near(left, right)
		server.link(left, right, sympathetic, authority.At)
		server.link(right, left, sympathetic, authority.At)
	}

	// A retained link holds only while the arrangement keeps the two close
	// and the neighbour still stands higher. Checking that here is also what
	// keeps every chain strictly rising, so following one always ends.
	for point, neighbour := range server.uphill {
		if neighbour < 0 {
			continue
		}

		if neighbour >= count || !near(point, neighbour) || authority.At(neighbour) <= authority.At(point) {
			server.uphill[point] = -1
		}
	}

	server.peak = make([]int, count)
	server.previous = make([]int, count)
	held := count > 0 && known.Len() == count
	standing := held

	for point := range count {
		peak := point

		for server.uphill[peak] >= 0 {
			peak = server.uphill[peak]
		}

		server.peak[point] = peak
		server.previous[point] = -1

		if point >= known.Len() || !known.At(point) {
			held, standing = false, false
			continue
		}

		server.previous[point] = int(prior.At(point*peakWidth + 1))

		if prior.At(point*peakWidth+1) < 0 || prior.At(point*peakWidth+1) != prior.At(point*peakWidth+2) {
			held = false
		}

		if prior.At(point*peakWidth+3) < 0 {
			standing = false
		}
	}

	server.regions = nil

	if held {
		server.regions = server.previous
		return nil
	}

	if !standing {
		return nil
	}

	server.regions = make([]int, count)

	for point := range count {
		server.regions[point] = int(prior.At(point*peakWidth + 3))
	}

	return nil
}

/*
link updates where one point drains after reading its relationship with
another. An attraction the arrangement has acted on offers a way uphill; a
relationship that no longer is one withdraws it.
*/
func (server *PeakServer) link(point, neighbour int, sympathetic bool, height func(int) float64) {
	current := server.uphill[point]

	if !sympathetic {
		if current == neighbour {
			server.uphill[point] = -1
		}

		return
	}

	if height(neighbour) <= height(point) {
		return
	}

	if current >= 0 && current < len(server.uphill) && height(current) >= height(neighbour) {
		return
	}

	server.uphill[point] = neighbour
}

/*
standing is a point's peak in the partition that stands, or -1 with none.
*/
func standing(regions []int, point int) int {
	if regions == nil {
		return -1
	}

	return regions[point]
}

func (server *PeakServer) Done(ctx context.Context, call Peak_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "geometry.peak: failed to allocate results", err))
	}

	state, err := results.NewState(int32(len(server.peak) * peakWidth))

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "geometry.peak: failed to allocate state", err))
	}

	index, err := results.NewIndex(int32(len(server.peak)))

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "geometry.peak: failed to allocate index", err))
	}

	for point, summit := range server.peak {
		state.Set(point*peakWidth, float64(server.uphill[point]))
		state.Set(point*peakWidth+1, float64(summit))
		state.Set(point*peakWidth+2, float64(server.previous[point]))
		state.Set(point*peakWidth+3, float64(standing(server.regions, point)))
		index.Set(point, int64(point))
	}

	results.SetMoving()

	if server.regions != nil {
		results.SetSettled()
		settled := results.Settled()
		region, err := settled.NewRegions(int32(len(server.regions)))

		if err != nil {
			return errnie.Error(errnie.Err(errnie.Internal, "geometry.peak: failed to allocate regions", err))
		}

		hasher := sha256.New()

		for point, summit := range server.regions {
			name := strconv.Itoa(summit)

			if err := region.Set(point, name); err != nil {
				return errnie.Error(errnie.Err(errnie.Internal, "geometry.peak: failed to set a region", err))
			}

			hasher.Write([]byte(name + ","))
		}

		vocabulary, err := json.Marshal(hex.EncodeToString(hasher.Sum(nil)))

		if err != nil {
			return errnie.Error(errnie.Err(errnie.Internal, "geometry.peak: failed to encode vocabulary", err))
		}

		if err := settled.SetVocabulary(vocabulary); err != nil {
			return errnie.Error(errnie.Err(errnie.Internal, "geometry.peak: failed to set vocabulary", err))
		}
	}

	server.uphill, server.peak, server.previous, server.regions = nil, nil, nil, nil
	return nil
}
