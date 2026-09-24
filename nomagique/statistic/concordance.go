package statistic

import (
	"context"
	"fmt"
	"math"

	"github.com/theapemachine/errnie"
)

/*
concordanceWidth is the layout of one pair's retained state: support, summed
alignment, mean alignment, its second moment and summed magnitude agreement.
*/
const concordanceWidth = 5

/*
ConcordanceServer reads how pairs of elements move together.
*/
type ConcordanceServer struct {
	from        []int64
	to          []int64
	strength    []float64
	orientation []float64
	index       []int64
	state       []float64
}

func NewConcordance() *ConcordanceServer {
	return &ConcordanceServer{}
}

func (server *ConcordanceServer) Write(ctx context.Context, call Concordance_write) error {
	args := call.Args()
	value, err := args.Value()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "statistic.concordance: failed to read value", err))
	}

	from, err := args.FromNodes()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "statistic.concordance: failed to read fromNodes", err))
	}

	to, err := args.ToNodes()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "statistic.concordance: failed to read toNodes", err))
	}

	pairs, err := args.Pairs()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "statistic.concordance: failed to read pairs", err))
	}

	prior, err := args.Prior()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "statistic.concordance: failed to read prior", err))
	}

	known, err := args.Known()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "statistic.concordance: failed to read known", err))
	}

	count := pairs.Len()

	if from.Len() != count || to.Len() != count || known.Len() != count || prior.Len() != count*concordanceWidth {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			fmt.Sprintf(
				"statistic.concordance: %d pairs need as many ends and flags and %d prior numbers; got %d, %d, %d, %d",
				count, count*concordanceWidth, from.Len(), to.Len(), known.Len(), prior.Len(),
			),
			nil,
		))
	}

	server.from, server.to = server.from[:0], server.to[:0]
	server.strength, server.orientation = server.strength[:0], server.orientation[:0]
	server.index, server.state = server.index[:0], server.state[:0]
	record := make([]float64, concordanceWidth)

	for pair := range count {
		left, right := int(from.At(pair)), int(to.At(pair))

		if left < 0 || right < 0 || left >= value.Len() || right >= value.Len() {
			return errnie.Error(errnie.Err(
				errnie.Validation,
				fmt.Sprintf("statistic.concordance: pair %d-%d is outside %d values", left, right, value.Len()),
				nil,
			))
		}

		clear(record)

		if known.At(pair) {
			for offset := range concordanceWidth {
				record[offset] = prior.At(pair*concordanceWidth + offset)
			}
		}

		strength, orientation, active := concord(record, value.At(left), value.At(right))

		if active {
			server.index = append(server.index, pairs.At(pair))
			server.state = append(server.state, record...)
		}

		if record[0] == 0 {
			continue
		}

		server.from = append(server.from, int64(left))
		server.to = append(server.to, int64(right))
		server.strength = append(server.strength, strength)
		server.orientation = append(server.orientation, orientation)
	}

	return nil
}

/*
concord adds one paired movement to a pair's record and reads the pair.
*/
func concord(record []float64, left, right float64) (float64, float64, bool) {
	prior := record[1]
	alignment := 0.0
	active := left != 0 || right != 0

	if active {
		if left != 0 && right != 0 {
			alignment = math.Copysign(1, left) * math.Copysign(1, right)
		}

		record[0]++
		record[1] += alignment
		delta := alignment - record[2]
		record[2] += delta / record[0]
		record[3] += delta * (alignment - record[2])
		record[4] += 1 - math.Abs(math.Abs(left)-math.Abs(right))/(math.Abs(left)+math.Abs(right))
	}

	if record[0] == 0 {
		return 0, 0, active
	}

	mean := record[1] / record[0]
	consistency := math.Abs(mean)
	uncertainty := math.Sqrt(record[3] / record[0] / record[0])
	strength := consistency - uncertainty + consistency*record[4]/record[0]

	if active && prior != 0 && alignment != math.Copysign(1, prior) {
		strength = -math.Abs(strength)
	}

	return strength, math.Copysign(1, mean), active
}

func (server *ConcordanceServer) Done(ctx context.Context, call Concordance_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "statistic.concordance: failed to allocate results", err))
	}

	integers := []struct {
		values []int64
		alloc  func(int32) (indexSetter, error)
	}{
		{server.from, func(size int32) (indexSetter, error) { return results.NewFromNodes(size) }},
		{server.to, func(size int32) (indexSetter, error) { return results.NewToNodes(size) }},
		{server.index, func(size int32) (indexSetter, error) { return results.NewIndex(size) }},
	}

	for _, list := range integers {
		allocated, err := list.alloc(int32(len(list.values)))

		if err != nil {
			return errnie.Error(errnie.Err(errnie.Internal, "statistic.concordance: failed to allocate a list", err))
		}

		for position, number := range list.values {
			allocated.Set(position, number)
		}
	}

	numbers := []struct {
		values []float64
		alloc  func(int32) (listSetter, error)
	}{
		{server.strength, func(size int32) (listSetter, error) { return results.NewStrength(size) }},
		{server.orientation, func(size int32) (listSetter, error) { return results.NewOrientation(size) }},
		{server.state, func(size int32) (listSetter, error) { return results.NewState(size) }},
	}

	for _, list := range numbers {
		allocated, err := list.alloc(int32(len(list.values)))

		if err != nil {
			return errnie.Error(errnie.Err(errnie.Internal, "statistic.concordance: failed to allocate a list", err))
		}

		for position, number := range list.values {
			allocated.Set(position, number)
		}
	}

	server.from, server.to = server.from[:0], server.to[:0]
	server.strength, server.orientation = server.strength[:0], server.orientation[:0]
	server.index, server.state = server.index[:0], server.state[:0]
	return nil
}
