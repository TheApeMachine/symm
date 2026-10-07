package strategy

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/hindsight/tables"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/store"
)

const gridLatestKey = "grid/latest"

/*
impulse is the Impulse Map of TRAINING.md: one grid of metric cells, the
per-symbol streams whose deformations light it, and the grid's checkpoint in
object storage. It develops the grid from live tape until the regions settle,
then only reads region tokens from the frozen grid.

The live streams belong to the training pipeline stage, which steps one
measurement at a time; a replayed fragment gets its own stream (tokens), so
historical tokens never depend on what the live tape last showed.
*/
type impulse struct {
	grid    *store.Grid
	catalog *tables.Catalog
	live    map[string]*store.Stream

	// restore runs the grid/latest lookup once per process: a missing
	// checkpoint must not cost an object-storage round trip on every Step.
	restore    sync.Once
	restoreErr error
}

func newImpulse(catalog *tables.Catalog) *impulse {
	return &impulse{
		grid:    store.NewGrid(),
		catalog: catalog,
		live:    make(map[string]*store.Stream),
	}
}

/*
observe folds one live step of symbol into its stream and answers the
lighting pass of every sensory channel. While the grid develops, the step
also trains it; a settled grid is frozen and no longer pays the pair update.
*/
func (impulse *impulse) observe(prior *data.Measurement) map[string]store.Excitation {
	stream, ok := impulse.live[prior.Label]

	if !ok {
		stream = store.NewStream()
		impulse.live[prior.Label] = stream
	}

	observed := channelsFrom(sensoryMeasurements(prior)...)
	deformations := stream.Deform(observed.raw)

	if !impulse.grid.IsSettled() {
		impulse.grid.Update(prior.Tick, deformations)
	}

	return observed.excite(deformations)
}

/*
develop advances grid development by one step and reports whether the grid
is settled and checkpointed. A previously checkpointed grid (grid/latest) is
restored first, so a restart does not re-discover regions that already froze.
Spectral partitioning is O(n^3) in cells, so it runs single-flight in the
background and the pipeline stage never waits for it.
*/
func (impulse *impulse) develop(ctx context.Context, epoch, seqIdx int64) (bool, error) {
	if !impulse.grid.IsSettled() {
		restored, err := impulse.restored(ctx)

		if err != nil {
			return false, errnie.Error(err)
		}

		if !restored {
			if seqIdx%32 == 0 {
				impulse.grid.PartitionAsync()
			}

			if !impulse.grid.Converged() {
				return false, nil
			}

			impulse.grid.Settle()
		}
	}

	return impulse.checkpoint(ctx, epoch, seqIdx)
}

/*
restored loads grid/latest when object storage is configured and a
checkpoint exists. Unconfigured storage or a missing checkpoint leaves the
live develop path (false, nil). A configured store that fails to read, or a
checkpoint that fails to restore, is an error: silently re-developing would
then overwrite grid/latest with a fresh grid and discard the trained one.
*/
func (impulse *impulse) restored(ctx context.Context) (bool, error) {
	if impulse.catalog == nil {
		return false, nil
	}

	impulse.restore.Do(func() {
		encoded, err := impulse.catalog.GetBlob(ctx, gridLatestKey)

		if errors.Is(err, tables.ErrBlobMissing) ||
			errors.Is(err, tables.ErrBlobStorageUnconfigured) {
			return
		}

		if err != nil {
			impulse.restoreErr = errnie.Err(errnie.IO, "[impulse] unable to read grid/latest", err)
			return
		}

		if err = impulse.grid.RestoreSnapshot(encoded); err != nil {
			impulse.restoreErr = errnie.Err(errnie.IO, "[impulse] unable to restore grid/latest", err)
		}
	})

	if impulse.restoreErr != nil {
		return false, impulse.restoreErr
	}

	return impulse.grid.IsSettled(), nil
}

/*
checkpoint snapshots the settled grid to grid/{epoch}/{seqIdx} and
grid/latest. The upload runs off the pipeline; a failed upload only loses the
restart checkpoint and is logged. A snapshot that cannot be taken is an error.
*/
func (impulse *impulse) checkpoint(ctx context.Context, epoch, seqIdx int64) (bool, error) {
	encoded, err := impulse.grid.Snapshot()

	if err != nil {
		return false, errnie.Error(errnie.Err(
			errnie.IO,
			fmt.Sprintf("[impulse] unable to snapshot grid/%d/%d", epoch, seqIdx),
			err,
		))
	}

	if impulse.catalog == nil {
		return true, nil
	}

	keys := []string{fmt.Sprintf("grid/%d/%d", epoch, seqIdx), gridLatestKey}

	go func() {
		for _, key := range keys {
			if err := impulse.catalog.PutBlob(ctx, key, encoded); err != nil {
				errnie.Error(errnie.Err(errnie.IO, "[impulse] unable to checkpoint "+key, err))
			}
		}
	}()

	return true, nil
}

type tapeStats struct {
	uniqueTicks int
	maxScore    float64
}

/*
tokens encodes one replayed tape (tick-ordered sensory rows of one symbol)
into one region token per tick on the frozen grid, through a stream of its
own. Ticks that light no region carry no frame.
*/
func (impulse *impulse) tokens(rows []*data.Measurement) ([]int64, [][]byte, tapeStats, error) {
	if !impulse.grid.IsSettled() {
		return nil, nil, tapeStats{}, errnie.Error(errnie.Err(
			errnie.Validation,
			"[impulse] region tokens are only comparable on a settled grid",
			nil,
		))
	}

	var (
		stream   = store.NewStream()
		ticks    []int64
		frames   [][]byte
		group    []*data.Measurement
		stats    tapeStats
		maxScore float64
	)

	flush := func() {
		if len(group) == 0 {
			return
		}

		tick := group[0].Tick
		stats.uniqueTicks++
		observed := channelsFrom(group...)
		deforms := stream.Deform(observed.raw)
		excited := observed.excite(deforms)
		scores := impulse.grid.RegionScores(excited)

		if len(scores) > 0 && scores[0].Score > maxScore {
			maxScore = scores[0].Score
		}

		tok := impulse.grid.LitRegion(excited)
		group = group[:0]

		if len(tok) == 0 {
			return
		}

		ticks = append(ticks, tick)
		frames = append(frames, tok)
	}

	for _, row := range rows {
		if len(group) > 0 && row.Tick != group[0].Tick {
			flush()
		}

		group = append(group, row)
	}

	flush()
	stats.maxScore = maxScore

	return ticks, frames, stats, nil
}

/*
channels is one pass of grid cells: the raw value of every cell and the
confidence of the Measurements that observed it (data.Measurement.Confidence).
Confidence belongs to the Measurement, not to its Metrics, so every cell a
Measurement feeds carries the same one.
*/
type channels struct {
	raw        map[string]float64
	confidence map[string]float64
}

/*
channelsFrom extracts one raw value per grid cell from measurements. Several
observations of one cell in a pass (the same label from two producers, or one
pair fact "<fact>@<peer>" across every peer symbol) are reduced to their
mean, and so is the confidence of the Measurements behind them, so the cell
does not depend on peer order.
*/
func channelsFrom(measurements ...*data.Measurement) channels {
	raw := make(map[string]float64)
	confidence := make(map[string]float64)
	counts := make(map[string]int)

	for _, measurement := range measurements {
		if measurement == nil || measurement.Error() != nil {
			continue
		}

		trust := measurement.Confidence()

		for entry := range measurement.Read() {
			if entry == nil || entry.Err != nil || entry.Metric == nil {
				continue
			}

			key := store.CellKey(entry.Metric.Label)
			raw[key] += entry.Metric.Raw
			confidence[key] += trust
			counts[key]++
		}
	}

	for key, count := range counts {
		raw[key] /= float64(count)
		confidence[key] /= float64(count)
	}

	return channels{raw: raw, confidence: confidence}
}

/*
excite pairs every deformation of the pass with the confidence of its cell.
*/
func (channels channels) excite(deformations map[string]float64) map[string]store.Excitation {
	pass := make(map[string]store.Excitation, len(deformations))

	for key, deformation := range deformations {
		pass[key] = store.Excitation{Deformation: deformation, Confidence: channels.confidence[key]}
	}

	return pass
}

/*
sensoryMeasurements answers the Stage 0 sensory producers of a measurement
and its peers. Resonance and manifold are execution vetoes (see
paper.authorized), not grid cells.
*/
func sensoryMeasurements(prior *data.Measurement) []*data.Measurement {
	if prior.Source != "runtime:join" {
		if isSolver(prior) {
			return nil
		}

		return []*data.Measurement{prior}
	}

	var sensory []*data.Measurement

	for _, peer := range prior.Peers() {
		if peer == nil || isSolver(peer) {
			continue
		}

		sensory = append(sensory, peer)
	}

	return sensory
}

func isSolver(measurement *data.Measurement) bool {
	return measurement.Source == "resonance" || measurement.Source == "manifold"
}
