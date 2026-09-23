package statistic

import (
	"context"
	"encoding/json"
	"math"
	"sort"

	"github.com/theapemachine/errnie"
)

/* TallyServer applies one multiset insertion to explicit input counts. */
type TallyServer struct {
	counts map[string]uint64
	total  uint64
}

func NewTally() *TallyServer { return &TallyServer{} }

/* Write validates integer multiplicities and optionally adds one observation. */
func (server *TallyServer) Write(ctx context.Context, call Tally_write) error {
	payload, err := call.Args().Counts()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "tally: counts", err))
	}

	var counts map[string]*uint64

	if err := json.Unmarshal(payload, &counts); err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "tally: counts must be an explicit integer object", err))
	}

	if counts == nil {
		return errnie.Error(errnie.Err(errnie.Validation, "tally: counts object is required", nil))
	}

	server.counts = make(map[string]uint64, len(counts))
	server.total = 0

	for category, count := range counts {
		if category == "" || count == nil || *count == 0 {
			return errnie.Error(errnie.Err(errnie.Validation, "tally: categories require positive integer multiplicities", nil))
		}

		if math.MaxUint64-server.total < *count {
			return errnie.Error(errnie.Err(errnie.Validation, "tally: total exceeds UInt64", nil))
		}

		server.counts[category] = *count
		server.total += *count
	}

	if call.Args().Query() {
		return nil
	}

	category, err := call.Args().Category()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "tally: category", err))
	}

	if category == "" || server.total == math.MaxUint64 {
		return errnie.Error(errnie.Err(errnie.Validation, "tally: nonempty category and count capacity required", nil))
	}

	server.counts[category]++
	server.total++
	return nil
}

/* Done emits the tally in deterministic category order and releases its state. */
func (server *TallyServer) Done(ctx context.Context, call Tally_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "tally: allocate result", err))
	}

	encoded, err := json.Marshal(server.counts)

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "tally: encode counts", err))
	}

	if err := results.SetCounts(encoded); err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "tally: emit counts", err))
	}

	categories := make([]string, 0, len(server.counts))

	for category := range server.counts {
		categories = append(categories, category)
	}

	sort.Strings(categories)
	names, err := results.NewCategories(int32(len(categories)))

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "tally: allocate categories", err))
	}

	weights, err := results.NewWeights(int32(len(categories)))

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "tally: allocate weights", err))
	}

	for index, category := range categories {
		if err := names.Set(index, []byte(category)); err != nil {
			return errnie.Error(errnie.Err(errnie.Internal, "tally: emit category", err))
		}

		weights.Set(index, server.counts[category])
	}

	results.SetTotal(server.total)
	server.counts = nil
	server.total = 0
	return nil
}
