package strategy

import (
	"context"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/hindsight"
	"github.com/theapemachine/symm/store"
	"gocloud.dev/blob"
	"gocloud.dev/gcerrors"
)

// CheckpointKey names the single consolidated model, independent of a process run.
const CheckpointKey = "models/agent.json"

// LoadCheckpoint restores the saved model before observations start arriving.
// A missing object is a new learner; corrupt or inaccessible objects are errors.
func (agent *Agent) LoadCheckpoint(ctx context.Context, bucket *blob.Bucket) error {
	checkpoint, err := store.Read[Checkpoint](ctx, bucket, CheckpointKey)
	if gcerrors.Code(err) == gcerrors.NotFound {
		errnie.Info("agent: no saved model; starting learning")
		return nil
	}
	if err != nil {
		return err
	}
	return agent.Restore(checkpoint)
}

// Persist takes checkpoints through the workspace owner and stores that original
// checkpoint struct. A failed periodic write is logged; later ticks retry.
func (agent *Agent) Persist(ctx context.Context, bucket *blob.Bucket, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			checkpoint, err := agent.SnapshotCheckpoint(ctx)
			if err != nil {
				errnie.Error(err)
				return
			}
			if err := store.Write(ctx, bucket, CheckpointKey, checkpoint); err != nil {
				errnie.Error(err)
			}
		}
	}
}

// Run evaluates completed tape outcomes on the persistence polling cadence.
// Market episode maturity remains determined by the captured observations.
func (reviewer *PolicyReview) Run(ctx context.Context, bucket *blob.Bucket, run hindsight.RunID, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := reviewer.readTape(ctx, bucket, run); err != nil {
				errnie.Error(err)
			}
		}
	}
}

func (reviewer *PolicyReview) readTape(ctx context.Context, bucket *blob.Bucket, run hindsight.RunID) error {
	observations, err := hindsight.ReadObservations(ctx, bucket, run)
	if err != nil {
		return err
	}
	spot := observations[:0]
	for _, observation := range observations {
		if observation.Domain == "spot" {
			spot = append(spot, observation)
		}
	}
	index := hindsight.NewRunIndex(run, spot)
	policy := hindsight.DefaultDiscoveryPolicy()
	confirmed := []hindsight.Episode{}
	for _, summary := range index.Summaries(policy) {
		for _, episode := range index.Discover(summary.Symbol, policy).Episodes {
			if episode.Confirmed && TrainableEpisode(episode.Kind) {
				confirmed = append(confirmed, episode)
			}
		}
	}
	return reviewer.Review(ctx, confirmed)
}
