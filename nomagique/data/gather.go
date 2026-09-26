package data

import (
	"context"
	"fmt"
	"slices"

	"github.com/bytedance/sonic"
	"github.com/theapemachine/errnie"
)

/*
SignalFamily defines the name and coordinate count of one contributing family.
*/
type SignalFamily struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

/*
GatherServer holds, as one list, the numbers its producers delivered on this
evaluation. When families are declared, it maintains retained readiness coverage
across all signal families, reporting warming phase until all families have contributed.
*/
type GatherServer struct {
	values             []float64
	covered            []bool
	families           []SignalFamily
	familyRanges       [][2]int
	familyContributing []bool
	ready              bool
	phase              string
	lastBatchPresent   []bool
}

func NewGather() *GatherServer {
	return &GatherServer{
		phase: "WARMING_SIGNALS",
	}
}

func (server *GatherServer) Write(ctx context.Context, call Gather_write) error {
	args := call.Args()
	values, err := args.Values()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "data.gather: failed to read values", err))
	}

	present, err := args.Present()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "data.gather: failed to read present", err))
	}

	familiesText, _ := args.Families()

	if len(server.families) == 0 && len(familiesText) > 0 {
		var parsedFamilies []SignalFamily
		parseErr := sonic.Unmarshal([]byte(familiesText), &parsedFamilies)

		if parseErr == nil && len(parsedFamilies) > 0 {
			server.families = parsedFamilies
			server.familyRanges = make([][2]int, len(parsedFamilies))
			server.familyContributing = make([]bool, len(parsedFamilies))

			currentOffset := 0

			for familyIndex, family := range parsedFamilies {
				server.familyRanges[familyIndex] = [2]int{currentOffset, currentOffset + family.Count}
				currentOffset += family.Count
			}
		}
	}

	if values.Len() == 0 {
		server.lastBatchPresent = nil
		return nil
	}

	if present.Len() != values.Len() {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			fmt.Sprintf("data.gather: %d values with %d presence flags", values.Len(), present.Len()),
			nil,
		))
	}

	if len(server.values) != values.Len() {
		server.values = make([]float64, values.Len())
		server.covered = make([]bool, values.Len())
		server.ready = false
		server.phase = "WARMING_SIGNALS"

		if len(server.families) > 0 {
			server.familyContributing = make([]bool, len(server.families))
		}
	}

	server.lastBatchPresent = make([]bool, values.Len())

	for slot := range values.Len() {
		isPres := present.At(slot)
		server.lastBatchPresent[slot] = isPres

		if !isPres {
			continue
		}

		val := values.At(slot)
		server.values[slot] = val
		server.covered[slot] = true

		for familyIndex, bounds := range server.familyRanges {
			if slot >= bounds[0] && slot < bounds[1] {
				server.familyContributing[familyIndex] = true
				break
			}
		}
	}

	return nil
}

func (server *GatherServer) Done(ctx context.Context, call Gather_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "data.gather: failed to allocate results", err))
	}

	contributingCount := 0
	var missingNames []string

	for familyIndex, family := range server.families {
		if server.familyContributing[familyIndex] {
			contributingCount++
		}

		if !server.familyContributing[familyIndex] {
			missingNames = append(missingNames, family.Name)
		}
	}

	if len(server.families) > 0 && contributingCount < len(server.families) {
		server.ready = false
		server.phase = "WARMING_SIGNALS"
	}

	if len(server.families) > 0 && contributingCount >= len(server.families) {
		server.ready = true
		server.phase = "FORMING_MAP"
	}

	if len(server.families) == 0 {
		server.ready = true
		server.phase = "FORMING_MAP"
	}

	readinessMap := map[string]any{
		"phase":        server.phase,
		"ready":        server.ready,
		"contributing": contributingCount,
		"total":        len(server.families),
		"missing":      missingNames,
		"input_count":  len(server.values),
	}

	readinessBytes, err := sonic.Marshal(readinessMap)

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "data.gather: failed to marshal readiness", err))
	}

	if err := results.SetReadiness(readinessBytes); err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "data.gather: failed to set readiness", err))
	}

	if err := results.SetPhase(server.phase); err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "data.gather: failed to set phase", err))
	}

	if !server.ready || !slices.Contains(server.lastBatchPresent, true) {
		results.SetIdle()
		return nil
	}

	results.SetGathered()
	gathered := results.Gathered()
	values, err := gathered.NewValues(int32(len(server.values)))

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "data.gather: failed to allocate values", err))
	}

	present, err := gathered.NewPresent(int32(len(server.values)))

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "data.gather: failed to allocate present", err))
	}

	for slot, value := range server.values {
		values.Set(slot, value)
		present.Set(slot, server.lastBatchPresent[slot])
	}

	return nil
}
