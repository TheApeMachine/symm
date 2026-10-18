package audit

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/data"
)

type policyWarmup struct {
	Rule     string
	FastRate float64
	SlowRate float64
	Ticks    int
}

func (warmup policyWarmup) String() string {
	if warmup.Rule == "ticks" {
		return fmt.Sprintf("after %d ticks (legacy)", warmup.Ticks)
	}

	return fmt.Sprintf("%s: error average at %g no longer below its average at %g", warmup.Rule, warmup.FastRate, warmup.SlowRate)
}

type policyGate struct {
	Epoch        uint64
	TickEpoch    uint64
	Enabled      bool
	SurpriseZ    float64
	Warmup       policyWarmup
	LearningRate float64
}

func (gate policyGate) String() string {
	state := "off"

	if gate.Enabled {
		state = "on"
	}

	return fmt.Sprintf("epoch %d on tick epoch %d (%s, %s)", gate.Epoch, gate.TickEpoch, state, gate.Warmup.String())
}

func readPolicyGate(value *data.Value) (policyGate, error) {
	var gate policyGate

	if value == nil || value.Origin != "policy" {
		return gate, errnie.Err(errnie.Validation, "audit: not a policy", nil)
	}

	reading := floatMetrics(value)
	enabled, ok := reading["enabled"]

	if !ok {
		return gate, errnie.Err(errnie.UnprocessableContent, "audit: policy "+value.Prefix+" has no gate", nil)
	}

	gate.Epoch = value.Epoch
	gate.Enabled = enabled != 0
	gate.SurpriseZ = reading["surprise_z"]
	gate.LearningRate = reading["learning_rate"]
	gate.Warmup.FastRate = reading["warmup_fast_rate"]
	gate.Warmup.SlowRate = reading["warmup_slow_rate"]

	if ticks, legacy := reading["warmup_ticks"]; legacy && reading["warmup_fast_rate"] == 0 {
		gate.Warmup.Rule = "ticks"
		gate.Warmup.Ticks = int(ticks)
	}

	for index := range data.Layer[[]float64](value, "gate") {
		if _, ticks, found := strings.Cut(index, "/ticks:"); found {
			ticks, _, _ = strings.Cut(ticks, "/")
			parsed, err := strconv.ParseUint(ticks, 10, 64)

			if err != nil {
				return gate, errnie.Err(errnie.UnprocessableContent, "audit: policy "+value.Prefix+" tick epoch "+ticks, err)
			}

			gate.TickEpoch = parsed
		}

		if _, rule, found := strings.Cut(index, "/warmup:"); found {
			rule, _, _ = strings.Cut(rule, "/")
			gate.Warmup.Rule = rule
		}
	}

	return gate, nil
}

func policyPath(value *data.Value) (string, error) {
	if value == nil {
		return "", errnie.Err(errnie.Validation, "audit: no policy path", nil)
	}

	for index := range data.Layer[[]float64](value) {
		_, path, found := strings.Cut(index, "/path:")

		if !found || path == "" {
			continue
		}

		path, _, _ = strings.Cut(path, "/")

		if path != "" {
			return path, nil
		}
	}

	return "", errnie.Err(errnie.UnprocessableContent, "audit: policy "+value.Prefix+" has no path", nil)
}

func policyPrefixes(epoch ...uint64) []string {
	prefixes := make([]string, 0, 2)

	for _, action := range []string{"enter", "exit"} {
		prefix := "policy/" + action + "/all/"

		if len(epoch) > 0 {
			prefix += fmt.Sprintf("%019d/", epoch[0])
		}

		prefixes = append(prefixes, prefix)
	}

	return prefixes
}

func excursionPrefixes(symbol string, epoch uint64) []string {
	prefixes := make([]string, 0, 4)

	for _, class := range []string{"up", "down", "chop", "flat"} {
		prefixes = append(prefixes, fmt.Sprintf("detections/%s/%s/%019d/", class, symbol, epoch))
	}

	return prefixes
}

func paperActionPrefixes() []string {
	return []string{"paper/enter/", "paper/exit/"}
}

type paperRecord struct {
	Action      string
	Symbol      string
	Path        string
	PolicyEpoch uint64
	Fields      map[string]float64
}

func paperFields(value *data.Value) (paperRecord, error) {
	var record paperRecord

	if value == nil || value.Origin != "paper" {
		return record, errnie.Err(errnie.Validation, "audit: not a paper record", nil)
	}

	keys := []string{
		"depth", "intended_price", "fill_price", "decided_at", "filled_at",
		"latency_seconds", "slippage", "qty", "entry_price", "exit_price",
		"entered_at", "exited_at", "fees", "pnl", "net_return",
	}
	record.Action = value.Role
	record.Symbol = value.Scope
	record.Fields = floatMetrics(value)

	for _, key := range keys {
		if _, ok := record.Fields[key]; !ok {
			return record, errnie.Err(errnie.UnprocessableContent, "audit: paper "+value.Prefix+" has no "+key, nil)
		}
	}

	for index := range data.Layer[[]float64](value) {
		for _, tag := range strings.Split(index, "/") {
			epoch, found := strings.CutPrefix(tag, "policy_epoch:")

			if !found {
				continue
			}

			parsed, err := strconv.ParseUint(epoch, 10, 64)

			if err != nil {
				return record, errnie.Err(errnie.UnprocessableContent, "audit: paper "+value.Prefix+" policy epoch "+epoch, err)
			}

			record.PolicyEpoch = parsed
		}

		if _, path, found := strings.Cut(index, "/path:"); found {
			path, _, _ = strings.Cut(path, "/policy_epoch:")
			record.Path = path
		}
	}

	if record.Path == "" || record.PolicyEpoch == 0 {
		return record, errnie.Err(errnie.UnprocessableContent, "audit: paper "+value.Prefix+" has no path or policy epoch", nil)
	}

	return record, nil
}
