package optimizer

import (
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/theapemachine/nomagique/causal"
	"github.com/theapemachine/nomagique/mcts"
)

const (
	colAction = iota
	colConfidence
	colEdge
	colFriction
	colFill
	colReward
)

// Options controls one playbook optimization pass.
type Options struct {
	Iterations      int
	HoldoutFraction float64
	Exploration     float64
	CausalAlpha     float64
	MinRows         int
	LinearFit       bool
}

// Sample is one observed playbook outcome row.
type Sample struct {
	Symbol          string  `json:"symbol,omitempty"`
	Type            string  `json:"type,omitempty"`
	Source          string  `json:"source,omitempty"`
	Category        string  `json:"category,omitempty"`
	Verdict         string  `json:"verdict,omitempty"`
	Confidence      float64 `json:"confidence,omitempty"`
	Edge            float64 `json:"edge,omitempty"`
	Reward          float64 `json:"reward,omitempty"`
	Hurdle          float64 `json:"hurdle,omitempty"`
	Friction        float64 `json:"friction,omitempty"`
	FillProbability float64 `json:"fill_probability,omitempty"`
	EconomicPriced  bool    `json:"economic_priced,omitempty"`
	Filled          *bool   `json:"filled,omitempty"`
}

// Report is the optimizer output. It is intentionally descriptive, not a patch.
type Report struct {
	Samples         int              `json:"samples"`
	UsableSamples   int              `json:"usable_samples"`
	TrainSamples    int              `json:"train_samples"`
	HoldoutSamples  int              `json:"holdout_samples"`
	ActionCount     int              `json:"action_count"`
	Iterations      int              `json:"iterations"`
	MinRows         int              `json:"min_rows"`
	Best            Recommendation   `json:"best"`
	Recommendations []Recommendation `json:"recommendations"`
}

// Recommendation is an MCTS-ranked playbook action family.
type Recommendation struct {
	ID             float64 `json:"id"`
	Type           string  `json:"type,omitempty"`
	Source         string  `json:"source,omitempty"`
	Category       string  `json:"category,omitempty"`
	TrainSamples   int     `json:"train_samples"`
	TrainReward    float64 `json:"train_reward"`
	HoldoutSamples int     `json:"holdout_samples"`
	HoldoutReward  float64 `json:"holdout_reward"`
}

type actionKey struct {
	Type     string
	Source   string
	Category string
}

type catalogEntry struct {
	id  float64
	key actionKey
}

type actionStats struct {
	count      int
	confidence float64
	edge       float64
	friction   float64
	fill       float64
	reward     float64
}

// Optimize ranks observed playbook action families using nomagique's causal MCTS.
func Optimize(samples []Sample, options Options) (Report, error) {
	usable := usableSamples(samples)
	if len(usable) == 0 {
		return Report{Samples: len(samples)}, errors.New("playbook optimizer: no usable samples with explicit reward or edge and fill data")
	}

	catalog := buildCatalog(usable)
	if len(catalog) == 0 {
		return Report{Samples: len(samples), UsableSamples: len(usable)}, errors.New("playbook optimizer: no action families found")
	}

	options = normalizeOptions(options, len(usable), len(catalog))
	train, holdout := splitSamples(usable, options.HoldoutFraction)
	if len(train) == 0 {
		train = usable
		holdout = nil
	}

	stats := statsByAction(train, catalog)
	root := playbookState{
		actions: actionIDs(catalog),
		stats:   stats,
	}
	rows := rowsForSamples(train, catalog)

	search := mcts.NewCausalMCTS(
		causal.NewMCTSAdapter(),
		options.Exploration,
		options.CausalAlpha,
		options.MinRows,
		colAction,
		colReward,
		[]int{colConfidence, colFriction, colFill},
		[]int{colAction, colConfidence, colEdge, colFriction, colFill},
		options.LinearFit,
	)

	bestID, err := search.Search(root, options.Iterations, rows)
	if err != nil {
		return Report{}, fmt.Errorf("playbook optimizer: mcts search failed: %w", err)
	}

	report := Report{
		Samples:        len(samples),
		UsableSamples:  len(usable),
		TrainSamples:   len(train),
		HoldoutSamples: len(holdout),
		ActionCount:    len(catalog),
		Iterations:     options.Iterations,
		MinRows:        options.MinRows,
	}
	report.Recommendations = recommendations(catalog, train, holdout)
	sort.Slice(report.Recommendations, func(i, j int) bool {
		if report.Recommendations[i].ID == bestID {
			return true
		}
		if report.Recommendations[j].ID == bestID {
			return false
		}
		if report.Recommendations[i].TrainReward == report.Recommendations[j].TrainReward {
			return report.Recommendations[i].ID < report.Recommendations[j].ID
		}
		return report.Recommendations[i].TrainReward > report.Recommendations[j].TrainReward
	})
	for _, rec := range report.Recommendations {
		if rec.ID == bestID {
			report.Best = rec
			break
		}
	}
	return report, nil
}

func normalizeOptions(options Options, samples, actions int) Options {
	if options.Iterations <= 0 {
		options.Iterations = samples * actions
	}
	if options.Iterations < actions {
		options.Iterations = actions
	}
	if options.Exploration <= 0 {
		options.Exploration = 1
	}
	if options.CausalAlpha <= 0 {
		options.CausalAlpha = 1
	}
	if options.MinRows <= 0 {
		requiredCausalRows := actions * actions * 2
		if samples < requiredCausalRows {
			options.MinRows = samples + options.Iterations + 1
		} else {
			options.MinRows = requiredCausalRows
		}
	}
	if options.HoldoutFraction < 0 {
		options.HoldoutFraction = 0
	}
	if options.HoldoutFraction >= 1 {
		options.HoldoutFraction = 0
	}
	return options
}

func usableSamples(samples []Sample) []Sample {
	out := make([]Sample, 0, len(samples))
	for _, sample := range samples {
		if _, ok := sample.netReward(); ok {
			out = append(out, sample)
		}
	}
	return out
}

func splitSamples(samples []Sample, holdoutFraction float64) ([]Sample, []Sample) {
	if holdoutFraction <= 0 || len(samples) < 2 {
		return samples, nil
	}
	holdout := int(math.Round(float64(len(samples)) * holdoutFraction))
	if holdout <= 0 {
		return samples, nil
	}
	if holdout >= len(samples) {
		holdout = len(samples) - 1
	}
	cut := len(samples) - holdout
	return samples[:cut], samples[cut:]
}

func buildCatalog(samples []Sample) []catalogEntry {
	seen := map[string]actionKey{}
	for _, sample := range samples {
		key := sample.actionKey()
		seen[key.String()] = key
	}
	keys := make([]string, 0, len(seen))
	for key := range seen {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	catalog := make([]catalogEntry, 0, len(keys))
	for idx, key := range keys {
		catalog = append(catalog, catalogEntry{
			id:  float64(idx + 1),
			key: seen[key],
		})
	}
	return catalog
}

func rowsForSamples(samples []Sample, catalog []catalogEntry) [][]float64 {
	rows := make([][]float64, 0, len(samples))
	for _, sample := range samples {
		reward, ok := sample.netReward()
		if !ok {
			continue
		}
		fill, _ := sample.fillRate()
		rows = append(rows, []float64{
			actionID(sample, catalog),
			sample.Confidence,
			sample.Edge,
			sample.friction(),
			fill,
			reward,
		})
	}
	return rows
}

func statsByAction(samples []Sample, catalog []catalogEntry) map[float64]actionStats {
	stats := make(map[float64]actionStats, len(catalog))
	for _, entry := range catalog {
		stats[entry.id] = actionStats{}
	}
	for _, sample := range samples {
		reward, ok := sample.netReward()
		if !ok {
			continue
		}
		fill, _ := sample.fillRate()
		id := actionID(sample, catalog)
		stat := stats[id]
		stat.count++
		stat.confidence += sample.Confidence
		stat.edge += sample.Edge
		stat.friction += sample.friction()
		stat.fill += fill
		stat.reward += reward
		stats[id] = stat
	}
	return stats
}

func actionIDs(catalog []catalogEntry) []float64 {
	ids := make([]float64, 0, len(catalog))
	for _, entry := range catalog {
		ids = append(ids, entry.id)
	}
	sort.Float64s(ids)
	return ids
}

func recommendations(catalog []catalogEntry, train, holdout []Sample) []Recommendation {
	recs := make([]Recommendation, 0, len(catalog))
	for _, entry := range catalog {
		trainReward, trainCount := averageReward(train, entry.id, catalog)
		holdoutReward, holdoutCount := averageReward(holdout, entry.id, catalog)
		recs = append(recs, Recommendation{
			ID:             entry.id,
			Type:           entry.key.Type,
			Source:         entry.key.Source,
			Category:       entry.key.Category,
			TrainSamples:   trainCount,
			TrainReward:    trainReward,
			HoldoutSamples: holdoutCount,
			HoldoutReward:  holdoutReward,
		})
	}
	return recs
}

func averageReward(samples []Sample, id float64, catalog []catalogEntry) (float64, int) {
	var total float64
	var count int
	for _, sample := range samples {
		if actionID(sample, catalog) != id {
			continue
		}
		reward, ok := sample.netReward()
		if !ok {
			continue
		}
		total += reward
		count++
	}
	if count == 0 {
		return 0, 0
	}
	return total / float64(count), count
}

func actionID(sample Sample, catalog []catalogEntry) float64 {
	key := sample.actionKey().String()
	for _, entry := range catalog {
		if entry.key.String() == key {
			return entry.id
		}
	}
	return 0
}

func (sample Sample) actionKey() actionKey {
	return actionKey{
		Type:     normalizeKey(sample.Type),
		Source:   normalizeKey(sample.Source),
		Category: normalizeKey(sample.Category),
	}
}

func (sample Sample) netReward() (float64, bool) {
	raw, ok := sample.rawReward()
	if !ok {
		return 0, false
	}
	fill, ok := sample.fillRate()
	if !ok {
		return 0, false
	}
	return raw*fill - sample.friction(), true
}

func (sample Sample) rawReward() (float64, bool) {
	if finiteNonZero(sample.Reward) {
		return sample.Reward, true
	}
	if finiteNonZero(sample.Edge) {
		return sample.Edge, true
	}
	return 0, false
}

func (sample Sample) fillRate() (float64, bool) {
	if sample.FillProbability > 0 {
		return clamp(sample.FillProbability, 0, 1), true
	}
	if sample.Filled != nil {
		if *sample.Filled {
			return 1, true
		}
		return 0, true
	}
	if strings.EqualFold(sample.Type, "market") {
		return 1, true
	}
	if sample.EconomicPriced {
		return 1, true
	}
	return 0, false
}

func (sample Sample) friction() float64 {
	if math.IsNaN(sample.Friction) || math.IsInf(sample.Friction, 0) {
		return 0
	}
	if sample.Friction != 0 {
		return math.Abs(sample.Friction)
	}
	if math.IsNaN(sample.Hurdle) || math.IsInf(sample.Hurdle, 0) {
		return 0
	}
	return math.Abs(sample.Hurdle)
}

func (key actionKey) String() string {
	return key.Type + "\x00" + key.Source + "\x00" + key.Category
}

func normalizeKey(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func finiteNonZero(value float64) bool {
	return value != 0 && !math.IsNaN(value) && !math.IsInf(value, 0)
}

func clamp(value, lo, hi float64) float64 {
	if value < lo {
		return lo
	}
	if value > hi {
		return hi
	}
	return value
}

type playbookState struct {
	terminal bool
	action   float64
	reward   float64
	vector   []float64
	actions  []float64
	stats    map[float64]actionStats
}

func (state playbookState) IsTerminal() bool {
	return state.terminal
}

func (state playbookState) GetReward() float64 {
	return state.reward
}

func (state playbookState) GetPossibleActions() []float64 {
	if state.terminal {
		return nil
	}
	actions := append([]float64(nil), state.actions...)
	return actions
}

func (state playbookState) ApplyAction(action float64) mcts.State {
	stat := state.stats[action]
	vector := stat.meanVector(action)
	return playbookState{
		terminal: true,
		action:   action,
		reward:   vector[colReward],
		vector:   vector,
		actions:  state.actions,
		stats:    state.stats,
	}
}

func (state playbookState) ToVector() []float64 {
	if len(state.vector) == 0 {
		return []float64{state.action, 0, 0, 0, 0, state.reward}
	}
	return append([]float64(nil), state.vector...)
}

func (stat actionStats) meanVector(action float64) []float64 {
	if stat.count == 0 {
		return []float64{action, 0, 0, 0, 0, 0}
	}
	denom := float64(stat.count)
	return []float64{
		action,
		stat.confidence / denom,
		stat.edge / denom,
		stat.friction / denom,
		stat.fill / denom,
		stat.reward / denom,
	}
}
