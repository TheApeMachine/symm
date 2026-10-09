package strategy

import (
	"bytes"
	"context"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/hindsight/tables"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/nomagique/store"
	"golang.org/x/sync/errgroup"
)

/*
Training orchestrates TRAINING.md. Step is the pipeline stage: it develops
the Impulse Map until its regions settle, then (once Train has opened the
skill gate) paper trades the live market with the Model. Train rehearses the
stored excursions of past runs on the frozen grid in the background.

Training only wires its owners; each one owns its own state:

  - impulse: the grid, its checkpoint, and the per-symbol streams;
  - Model: the predictive trie;
  - Rehearsal: historical fragment learning and its UI fragments;
  - paper: live episodes, Desk actions, and realized refinement;
  - Reporter: the learning telemetry and its UI tee.
*/
type Training struct {
	*runtime.System
	detector *Detector
	catalog  *tables.Catalog
	storeTee runtime.Tee
	grid     *store.Grid
}

func NewTraining(
	ctx context.Context,
	price *broker.Price,
	desk *broker.Desk,
	catalog *tables.Catalog,
	storeTee runtime.Tee,
) *Training {
	system := runtime.NewSystem(ctx, "training", price)

	training := &Training{
		System:   system,
		detector: NewDetector(ctx, storeTee, price),
		catalog:  catalog,
		storeTee: storeTee,
		grid:     store.NewGrid(),
	}

	// The detector labels every excursion against friction. Without it the
	// training system halts here instead of learning from mislabeled tape.
	if err := training.detector.Error(); err != nil {
		training.Error(errnie.Err(
			errnie.Internal, "[training] detector is not usable", err,
		))

		return training
	}

	training.Train()
	training.Transition(runtime.INIT)
	return training
}

/*
Step receives the live market Measurement.
*/
func (training *Training) Step(prior *data.Measurement) *data.Measurement {
	training.grid.Observe(prior)

	return prior.Next(training.Name(), map[string]float64{
		"win_rate": 0.0,
		"edge":     0.0,
	})
}

/*
Train rehearses the stored excursions of past runs on the frozen grid in the background.
It selects profitable friction-clearing excursions ("up"), retrieves their signal measurements,
assembles them as peers in a training frame, and runs them through the grid.
*/
func (training *Training) Train() error {
	if training == nil || training.catalog == nil {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"[training] catalog is required",
			nil,
		))
	}

	go func() {
		for excursion := range training.catalog.Excursions(training.Context()) {
			if excursion.Meta("type") != excursionUp {
				continue
			}

			group, ctx := errgroup.WithContext(training.Context())

			group.Go(func() error {
				select {
				case <-ctx.Done():
					return ctx.Err()
				default:
				}

				tokens := [2][][]byte{}

				for idx, fragment := range [][]string{{"start_tick", "b_tick"}, {"b_tick", "c_tick"}} {
					ticks := make(map[int64][]*data.Measurement)

					for tape, err := range training.catalog.ExcursionTape(
						training.Context(),
						excursion.Epoch,
						excursion.Label,
						int64(data.Pull(excursion.Read(fragment[0])).Metric.Raw),
						int64(data.Pull(excursion.Read(fragment[1])).Metric.Raw)-2,
					) {
						if err != nil {
							training.Error(errnie.Err(
								errnie.IO,
								"[training] tape read error",
								err,
							))

							continue
						}

						if tape != nil {
							ticks[tape.Tick] = append(ticks[tape.Tick], tape)
						}
					}

					for tick := int64(data.Pull(excursion.Read(
						fragment[0],
					)).Metric.Raw); tick <= int64(data.Pull(excursion.Read(
						fragment[1],
					)).Metric.Raw); tick++ {
						signals := ticks[tick]

						if len(signals) == 0 {
							continue
						}

						train := data.NewMeasurement(
							signals[0].Epoch,
							excursion.Label,
							"training",
							int64(data.Pull(excursion.Read("start_idx")).Metric.Raw),
							tick,
						)

						train.Peers(signals...)
						train.Write()

						token := training.grid.Observe(train)

						if len(tokens[idx]) > 0 && bytes.Equal(
							token, tokens[idx][len(tokens[idx])-1],
						) {
							continue
						}

						tokens[idx] = append(tokens[idx], token)
					}
				}

				for idx, action := range [][]byte{[]byte("enter.json"), []byte("exit.json")} {
					if len(tokens[idx]) == 0 {
						continue
					}

					tokens[idx] = append(tokens[idx], action)

					training.catalog.PutBlob(
						training.Context(),
						string(bytes.Join(tokens[idx], []byte("/"))),
						[]byte("{}"),
					)
				}

				return nil
			})

			if err := group.Wait(); err != nil {
				training.Error(errnie.Err(
					errnie.Internal,
					"[training] tape read error",
					err,
				))
			}
		}
	}()

	return nil
}
