package types

import (
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"

	"github.com/bytedance/sonic"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
)

/*
CutID identifies one market cut from ingress through execution.
*/
type CutID uint64

/*
SignalStatus reports whether a signal contributed evidence or explicitly skipped.
*/
type SignalStatus uint8

const (
	SignalReady SignalStatus = iota
	SignalSkip
)

/*
SignalResult is one signal's contribution to an exact CutID.
*/
type SignalResult struct {
	CutID        CutID
	Source       SourceType
	Measurements []*Measurement
	Status       SignalStatus
}

/*
ImmutableCut is a finalized, deep-copied evidence surface for analyzer/planner.
Durable Thesis may consume completed cuts; it is not mutated as the pipeline
object through signals.
*/
type ImmutableCut struct {
	ID           CutID
	Tick         int64
	At           time.Time
	Measurements []*Measurement
	Forecasts    []Forecasts
	Decisions    []Decision
	Hypotheses   []Hypothesis
	Categories   []Category
	Resonance    []any
	Causal       []any
	Incomplete   bool
	Sequence     uint64
}

/*
CloneMeasurements deep-copies measurement rows. Prefer SnapshotMeasurements for
cuts: Publish replaces row pointers rather than mutating published rows, so a
shallow slice copy is the correct freeze.
*/
func CloneMeasurements(rows []*Measurement) []*Measurement {
	if len(rows) == 0 {
		return nil
	}

	out := make([]*Measurement, 0, len(rows))

	for _, row := range rows {
		if row == nil {
			continue
		}

		copyRow := *row

		if row.Uncertainty != nil {
			uncertainty := *row.Uncertainty
			copyRow.Uncertainty = &uncertainty
		}

		if len(row.Metrics) > 0 {
			copyRow.Metrics = make(map[string]MetricSample, len(row.Metrics))

			for key, sample := range row.Metrics {
				copied := sample

				if sample.Normalized != nil {
					normalized := *sample.Normalized
					copied.Normalized = &normalized
				}

				copyRow.Metrics[key] = copied
			}
		}

		out = append(out, &copyRow)
	}

	return out
}

/*
NewImmutableCut builds a cut from the current thesis publish surface. Measurement
rows are deep-copied so later Publish pointer replacements and any in-place
enrichment cannot mutate a historical cut.
*/
func NewImmutableCut(id CutID, tick int64, thesis *Thesis) *ImmutableCut {
	if thesis == nil {
		return &ImmutableCut{ID: id, Tick: tick}
	}

	return &ImmutableCut{
		ID:           id,
		Tick:         tick,
		At:           thesis.At,
		Measurements: CloneMeasurements(thesis.SnapshotMeasurements()),
		Forecasts:    cloneForecasts(thesis.Forecasts),
		Decisions:    cloneDecisions(thesis.Decisions),
		Hypotheses:   cloneHypotheses(thesis.Hypotheses),
		Categories:   cloneCategories(thesis.Categories),
		Resonance:    append([]any(nil), thesis.Resonance...),
		Causal:       append([]any(nil), thesis.Causal...),
		Incomplete:   thesis.Incomplete(),
		Sequence:     uint64(tick),
	}
}

func cloneDecisions(rows []Decision) []Decision {
	if len(rows) == 0 {
		return nil
	}

	out := make([]Decision, len(rows))

	for index, row := range rows {
		out[index] = row

		if row.Alternatives != nil {
			alternatives := make(map[string]float64, len(row.Alternatives))

			for key, value := range row.Alternatives {
				alternatives[key] = value
			}

			out[index].Alternatives = alternatives
		}

		out[index].ProposedNotional = copyDecimal(row.ProposedNotional)
		out[index].ProposedQuantity = copyDecimal(row.ProposedQuantity)
		out[index].ReferencePrice = copyDecimal(row.ReferencePrice)
		out[index].ExpectedReturn = copyDecimal(row.ExpectedReturn)
		out[index].ExpectedFees = copyDecimal(row.ExpectedFees)
		out[index].ExpectedSpread = copyDecimal(row.ExpectedSpread)
		out[index].ExpectedImpact = copyDecimal(row.ExpectedImpact)
		out[index].AvailableCapital = copyDecimal(row.AvailableCapital)
		out[index].DisplacedQuantity = copyDecimal(row.DisplacedQuantity)
		out[index].DisplacedPrice = copyDecimal(row.DisplacedPrice)
	}

	return out
}

func cloneForecasts(rows []Forecasts) []Forecasts {
	if len(rows) == 0 {
		return nil
	}

	out := make([]Forecasts, len(rows))

	for index, row := range rows {
		out[index] = row
		out[index].ReferencePrice = copyDecimal(row.ReferencePrice)
		out[index].BuyCapacity = copyDecimal(row.BuyCapacity)
		out[index].SellCapacity = copyDecimal(row.SellCapacity)
	}

	return out
}

func cloneHypotheses(rows []Hypothesis) []Hypothesis {
	if len(rows) == 0 {
		return nil
	}

	out := make([]Hypothesis, len(rows))

	for index, row := range rows {
		out[index] = row
		out[index].Controls = append([]string(nil), row.Controls...)
	}

	return out
}

func cloneCategories(rows []Category) []Category {
	if len(rows) == 0 {
		return nil
	}

	out := make([]Category, len(rows))

	for index, row := range rows {
		out[index] = row
		out[index].Supporting = append([]string(nil), row.Supporting...)
		out[index].Opposing = append([]string(nil), row.Opposing...)
		out[index].Missing = append([]string(nil), row.Missing...)
	}

	return out
}

func copyDecimal(value *decimal.Decimal) *decimal.Decimal {
	if value == nil {
		return nil
	}

	return value.Copy()
}

/*
Checkpoint atomically persists this cut under dir. It fsyncs the temp file,
renames into place, then fsyncs the directory so a crash cannot lose the entry.
*/
func (cut *ImmutableCut) Checkpoint(dir string) error {
	if cut == nil {
		return fmt.Errorf("cut: checkpoint requires ImmutableCut")
	}

	payload, err := sonic.Marshal(cut)

	if err != nil {
		return fmt.Errorf("cut: marshal: %w", err)
	}

	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("cut: mkdir: %w", err)
	}

	target := filepath.Join(dir, ThesisKey+".json")
	temporaryPath, err := cut.writeTemp(dir, payload)

	if err != nil {
		return err
	}

	return cut.syncAndRename(dir, temporaryPath, target)
}

func (cut *ImmutableCut) writeTemp(dir string, payload []byte) (string, error) {
	temporary, err := os.CreateTemp(dir, ThesisKey+"-*.tmp")

	if err != nil {
		return "", fmt.Errorf("cut: temp: %w", err)
	}

	temporaryPath := temporary.Name()

	cleanup := func() {
		_ = temporary.Close()
		_ = os.Remove(temporaryPath)
	}

	if _, err := temporary.Write(payload); err != nil {
		cleanup()
		return "", fmt.Errorf("cut: write: %w", err)
	}

	if err := temporary.Sync(); err != nil {
		cleanup()
		return "", fmt.Errorf("cut: sync file: %w", err)
	}

	if err := temporary.Close(); err != nil {
		_ = os.Remove(temporaryPath)
		return "", fmt.Errorf("cut: close temp: %w", err)
	}

	return temporaryPath, nil
}

func (cut *ImmutableCut) syncAndRename(dir, temporaryPath, target string) error {
	if err := os.Rename(temporaryPath, target); err != nil {
		_ = os.Remove(temporaryPath)
		return fmt.Errorf("cut: rename: %w", err)
	}

	directory, err := os.Open(dir)

	if err != nil {
		return fmt.Errorf("cut: open dir: %w", err)
	}

	defer directory.Close()

	if err := directory.Sync(); err != nil {
		return fmt.Errorf("cut: sync dir: %w", err)
	}

	return nil
}

/*
CutCounter allocates monotonic cut identifiers.
*/
type CutCounter struct {
	next atomic.Uint64
}

/*
Next returns the next CutID.
*/
func (counter *CutCounter) Next() CutID {
	return CutID(counter.next.Add(1))
}
