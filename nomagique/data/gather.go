package data

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"time"

	capnp "capnproto.org/go/capnp/v3"
	"github.com/bytedance/sonic"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/types"
)

/* SignalFamily names a consecutive range of required metric coordinates. */
type SignalFamily struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

/*
GatherServer owns the latest observation of every wired metric. It emits a
complete cut only after every coordinate has initialized. A quiet producer's
value remains in the cut; missing initialization never becomes a zero reading.
*/
type GatherServer struct {
	values     []float64
	covered    []bool
	families   []SignalFamily
	declared   string
	width      int
	pending    bool
	epoch      int64
	sequence   int64
	scope      string
	epochs     []int64
	sequences  []int64
	identities []string
	provenance string
}

func NewGather() *GatherServer { return &GatherServer{} }

/* Write applies only the coordinates present in this observation. */
func (server *GatherServer) Write(ctx context.Context, call Gather_write) error {
	args := call.Args()
	families, err := args.Families()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "data.gather: read families", err))
	}

	if err := server.declare(families); err != nil {
		return err
	}

	identities, err := args.Identities()
	if err != nil {
		return errnie.Error(err)
	}
	if identities.Len() > 0 {
		if len(server.identities) == 0 {
			server.identities = make([]string, identities.Len())
		}
		if identities.Len() != len(server.identities) {
			return errnie.Error(errnie.Err(errnie.Validation, "data.gather: metric identities changed", nil))
		}
		for index := range identities.Len() {
			identity, err := identities.At(index)
			if err != nil {
				return errnie.Error(err)
			}
			if identity == "" || server.identities[index] != "" && server.identities[index] != identity {
				return errnie.Error(errnie.Err(errnie.Validation, "data.gather: metric identity is empty or changed", nil))
			}
			server.identities[index] = identity
		}
	}
	row, err := args.Row()
	if err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "data.gather: replay row", err))
	}
	if len(row) > 0 {
		if args.HasValues() || args.HasPresent() {
			return errnie.Error(errnie.Err(errnie.Validation, "data.gather: cannot mix replay and live coordinates", nil))
		}
		return server.restore(row)
	}

	values, err := args.Values()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "data.gather: read values", err))
	}

	present, err := args.Present()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "data.gather: read presence", err))
	}

	scope, err := args.Scope()
	if err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "data.gather: scope", err))
	}
	if args.Epoch() < 0 || args.Sequence() < 0 {
		return errnie.Error(errnie.Err(errnie.Validation, "data.gather: negative observation stamp", nil))
	}
	if server.epoch != args.Epoch() || server.scope != scope {
		clear(server.covered)
		clear(server.values)
		clear(server.epochs)
		clear(server.sequences)
		server.epoch, server.scope, server.sequence = args.Epoch(), scope, 0
	}
	if server.epoch > 0 && args.Sequence() < server.sequence {
		return errnie.Error(errnie.Err(errnie.Validation, "data.gather: observation sequence regressed", nil))
	}
	server.sequence = args.Sequence()
	observation, err := args.Observation()
	if err != nil {
		return errnie.Error(err)
	}
	server.provenance = ""
	var document struct {
		Capture json.RawMessage `json:"capture"`
	}
	if len(observation) > 0 {
		if err := json.Unmarshal(observation, &document); err != nil {
			return errnie.Error(err)
		}
	}
	provenance := document.Capture
	if len(provenance) > 0 {
		var origin struct {
			Session    string
			Sequence   *int64
			Record     *int64
			Endpoint   string
			ReceivedAt string
		}
		if err := json.Unmarshal(provenance, &origin); err != nil {
			return errnie.Error(err)
		}
		if origin.Session == "" || origin.Sequence == nil || *origin.Sequence < 0 || origin.Record == nil || *origin.Record < 0 || origin.Endpoint == "" {
			return errnie.Error(errnie.Err(errnie.Validation, "data.gather: incomplete source provenance", nil))
		}
		if _, err := time.Parse(time.RFC3339Nano, origin.ReceivedAt); err != nil {
			return errnie.Error(err)
		}
		server.provenance = string(provenance)
	}
	if args.RequireProvenance() && len(provenance) == 0 && values.Len() > 0 {
		return errnie.Error(errnie.Err(errnie.Validation, "data.gather: source provenance is required", nil))
	}

	server.pending = false

	if present.Len() != values.Len() {
		return errnie.Error(errnie.Err(errnie.Validation,
			fmt.Sprintf("data.gather: %d values with %d presence flags", values.Len(), present.Len()), nil))
	}

	if values.Len() == 0 {
		return nil
	}

	if len(server.identities) > 0 && len(server.identities) != values.Len() {
		return errnie.Error(errnie.Err(errnie.Validation, "data.gather: identities do not match coordinates", nil))
	}
	if server.width > 0 && values.Len() != server.width {
		return errnie.Error(errnie.Err(errnie.Validation, "data.gather: values do not match declared coordinate count", nil))
	}

	if len(server.values) == 0 {
		server.values = make([]float64, values.Len())
		server.covered = make([]bool, values.Len())
		server.epochs = make([]int64, values.Len())
		server.sequences = make([]int64, values.Len())
		server.width = values.Len()
	}

	for slot := range values.Len() {
		if !present.At(slot) {
			continue
		}

		server.values[slot] = values.At(slot)
		server.covered[slot] = true
		server.epochs[slot] = server.epoch
		server.sequences[slot] = server.sequence
		server.pending = true
	}

	return nil
}

/* declare validates the metric universe before any coordinate can contribute. */
func (server *GatherServer) declare(declaration string) error {
	if declaration == "" || declaration == server.declared {
		return nil
	}

	if server.declared != "" || len(server.values) > 0 {
		return errnie.Error(errnie.Err(errnie.Validation, "data.gather: cannot change the initialized metric universe", nil))
	}

	var families []SignalFamily

	if err := sonic.Unmarshal([]byte(declaration), &families); err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "data.gather: invalid families", err))
	}

	if len(families) == 0 {
		return errnie.Error(errnie.Err(errnie.Validation, "data.gather: declared families are empty", nil))
	}

	width := 0
	names := make(map[string]bool, len(families))

	for _, family := range families {
		if family.Name == "" || family.Count <= 0 || names[family.Name] {
			return errnie.Error(errnie.Err(errnie.Validation, "data.gather: families require distinct names and positive counts", nil))
		}

		names[family.Name] = true
		width += family.Count
	}

	server.families, server.declared, server.width = families, declaration, width
	return nil
}

/* Done emits each completed cut once, retaining causal values for the next cut. */
func (server *GatherServer) Done(ctx context.Context, call Gather_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "data.gather: allocate results", err))
	}

	ready := len(server.covered) > 0 && !slices.Contains(server.covered, false)
	phase := "WARMING_SIGNALS"

	if ready {
		phase = "FORMING_MAP"
	}

	if err := server.readiness(results, phase, ready); err != nil {
		return err
	}

	pending := server.pending
	server.pending = false

	if pending && len(server.identities) > 0 {
		row, err := results.NewRow()
		if err != nil {
			return err
		}
		if err := server.row(row, ready); err != nil {
			return errnie.Error(err)
		}
	}
	if !ready || !pending {
		results.SetIdle()
		return nil
	}

	results.SetEpoch(server.epoch)
	results.SetSequence(server.sequence)
	if err := results.SetScope(server.scope); err != nil {
		return errnie.Error(err)
	}
	epochs, err := results.NewEpochs(int32(len(server.values)))
	if err != nil {
		return errnie.Error(err)
	}
	sequences, err := results.NewSequences(int32(len(server.values)))
	if err != nil {
		return errnie.Error(err)
	}
	for index := range server.values {
		epochs.Set(index, server.epochs[index])
		sequences.Set(index, server.sequences[index])
	}
	results.SetGathered()
	gathered := results.Gathered()
	values, err := gathered.NewValues(int32(len(server.values)))

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "data.gather: allocate values", err))
	}

	present, err := gathered.NewPresent(int32(len(server.values)))

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "data.gather: allocate presence", err))
	}

	for slot, value := range server.values {
		values.Set(slot, value)
		present.Set(slot, true)
	}

	return nil
}

/* readiness reports a family complete only when all its metrics are initialized. */
func (server *GatherServer) readiness(results Gathered, phase string, ready bool) error {
	contributing, offset := 0, 0
	missing := make([]string, 0)

	for _, family := range server.families {
		end := offset + family.Count
		complete := end <= len(server.covered) && !slices.Contains(server.covered[offset:end], false)

		if complete {
			contributing++
		}

		if !complete {
			missing = append(missing, family.Name)
		}

		offset = end
	}

	metadata := struct {
		Phase        string   `json:"phase"`
		Ready        bool     `json:"ready"`
		Contributing int      `json:"contributing"`
		Total        int      `json:"total"`
		Missing      []string `json:"missing"`
		InputCount   int      `json:"input_count"`
	}{phase, ready, contributing, len(server.families), missing, server.width}
	encoded, err := sonic.Marshal(metadata)

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "data.gather: encode readiness", err))
	}

	if err := results.SetReadiness(encoded); err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "data.gather: set readiness", err))
	}

	if err := results.SetPhase(phase); err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "data.gather: set phase", err))
	}

	return nil
}

/* row preserves named metric evidence, including warming cuts, without replaying raw data. */
func (server *GatherServer) row(record types.Record, ready bool) error {
	cut, err := NewMetricCut(record.Segment())
	if err != nil {
		return errnie.Error(err)
	}
	cut.SetEpoch(server.epoch)
	cut.SetSequence(server.sequence)
	cut.SetComplete(ready)
	if err := cut.SetSymbol(server.scope); err != nil {
		return errnie.Error(err)
	}
	if err := cut.SetProvenance(server.provenance); err != nil {
		return errnie.Error(err)
	}
	metrics, err := cut.NewMetrics(int32(len(server.values)))
	if err != nil {
		return errnie.Error(err)
	}
	for index, value := range server.values {
		metric := metrics.At(index)
		if err := metric.SetIdentity(server.identities[index]); err != nil {
			return errnie.Error(err)
		}
		metric.SetValue(value)
		metric.SetPresent(server.covered[index])
		metric.SetEpoch(server.epochs[index])
		metric.SetSequence(server.sequences[index])
	}
	record.SetTypeId(MetricCut_TypeID)
	return errnie.Error(record.SetValue(capnp.Struct(cut).ToPtr()))
}

/* persistedCut is the stored form of this node's complete causal state. */
type persistedCut struct {
	Provenance string            `json:"provenance,omitempty"`
	Epoch      int64             `json:"epoch"`
	Sequence   int64             `json:"sequence"`
	Symbol     string            `json:"symbol"`
	Complete   bool              `json:"complete"`
	Metrics    []persistedMetric `json:"metrics"`
}

type persistedMetric struct {
	Identity string  `json:"identity"`
	Value    float64 `json:"value"`
	Present  bool    `json:"present"`
	Epoch    int64   `json:"epoch"`
	Sequence int64   `json:"sequence"`
}

/* restore replays persisted metrics directly, retaining their original causal stamps. */
func (server *GatherServer) restore(encoded []byte) error {
	var cut persistedCut
	if err := sonic.Unmarshal(encoded, &cut); err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "data.gather: decode metric cut", err))
	}
	if cut.Epoch <= 0 || cut.Sequence < 0 || cut.Symbol == "" || len(cut.Metrics) == 0 || len(cut.Metrics) != len(server.identities) {
		return errnie.Error(errnie.Err(errnie.Validation, "data.gather: cut identity, stamp or width is invalid", nil))
	}
	complete := true
	for index, metric := range cut.Metrics {
		if metric.Identity != server.identities[index] {
			return errnie.Error(errnie.Err(errnie.Validation, "data.gather: replay coordinate identity mismatch", nil))
		}
		if metric.Present && (metric.Epoch != cut.Epoch || metric.Sequence < 0 || metric.Sequence > cut.Sequence) {
			return errnie.Error(errnie.Err(errnie.Validation, "data.gather: replay metric stamp lies outside its causal cut", nil))
		}
		complete = complete && metric.Present
	}
	if complete != cut.Complete {
		return errnie.Error(errnie.Err(errnie.Validation, "data.gather: replay completeness disagrees with metric presence", nil))
	}
	if len(server.values) != len(cut.Metrics) {
		server.values = make([]float64, len(cut.Metrics))
		server.covered = make([]bool, len(cut.Metrics))
		server.epochs = make([]int64, len(cut.Metrics))
		server.sequences = make([]int64, len(cut.Metrics))
	}
	for index, metric := range cut.Metrics {
		server.values[index] = metric.Value
		server.covered[index] = metric.Present
		server.epochs[index] = metric.Epoch
		server.sequences[index] = metric.Sequence
	}
	server.epoch, server.sequence, server.scope, server.pending = cut.Epoch, cut.Sequence, cut.Symbol, true
	server.provenance = cut.Provenance
	return nil
}
