package types

import (
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"

	"github.com/bytedance/sonic"
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
	Categories   map[string][]Category
	Resonance    []any
	Causal       []any
	Incomplete   bool
	Sequence     uint64
}

/*
NewImmutableCut builds a cut from the current thesis publish surface. Measurements
are a shallow pointer-slice snapshot: Publish replaces row pointers rather than
mutating published rows, so SnapshotMeasurements is the correct freeze. Deep-
cloning every Measurement struct here allocated hundreds of gigabytes per run.
*/
func NewImmutableCut(id CutID, tick int64, thesis *Thesis) *ImmutableCut {
	if thesis == nil {
		return &ImmutableCut{ID: id, Tick: tick}
	}

	return &ImmutableCut{
		ID:           id,
		Tick:         tick,
		At:           thesis.At,
		Measurements: thesis.SnapshotMeasurements(),
		Forecasts:    thesis.Forecasts,
		Decisions:    thesis.Decisions,
		Hypotheses:   thesis.Hypotheses,
		Categories:   thesis.Categories,
		Resonance:    append([]any(nil), thesis.Resonance...),
		Causal:       append([]any(nil), thesis.Causal...),
		Incomplete:   thesis.Incomplete(),
		Sequence:     uint64(tick),
	}
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
