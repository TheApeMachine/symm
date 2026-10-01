package strategy

import (
	"context"
	"fmt"
	"time"

	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/hindsight/tables"
	"github.com/theapemachine/symm/nomagique/cognition"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/ui"
	"github.com/theapemachine/symm/workbench"
)

var _ ui.CognitionSource = (*Training)(nil)

/*
Training is the learning orchestrator.
Please refer to TRAINING.md for the complete overview of how training
on pre-cursors is performed.
*/
type Training struct {
	*runtime.System
	grid      *store.Grid
	engine    *cognition.Engine
	trader    *Trader
	catalog   *tables.Catalog
	warehouse *workbench.Warehouse
	tee       runtime.Tee
	pipeline  core.Primitive
	signature []byte
}

func NewTraining(
	ctx context.Context,
	price *broker.Price,
	trader *Trader,
	catalog *tables.Catalog,
	warehouse *workbench.Warehouse,
	tee runtime.Tee,
) *Training {
	grid := store.NewGrid()
	engine := cognition.NewEngine(cognition.Config{})

	training := &Training{
		System:    runtime.NewSystem(ctx, "training", price),
		grid:      grid,
		engine:    engine,
		trader:    trader,
		catalog:   catalog,
		warehouse: warehouse,
		tee:       tee,
	}

	// The pipeline for training consists of just the Grid in the primary stage.
	// The cognitive observations are handled manually using temporal signatures.
	training.pipeline = grid

	// Set the status to INIT to indicate we need to build the grid until it
	// has formed stable regions, and is frozen and checkpointed.
	training.Transition(runtime.INIT)
	return training
}

/*
Step is part of the LMAX Disruptor pipeline.
This means it is actively plugged in to the real-time market tape, and as such we
need to use it only for initial grid development, and later as the secondary training
stage, which uses the paper trading mechanism to validate the model developed in Run.

Also, since we have defined our peer-interest to "*" in our Register method, we
will receive all measurements from all peers. We need to use the grid to develop
stable regions over time, and when those regions are frozen, we can use them to
train the trie, and validate the model.
*/
func (training *Training) Step(
	measurement *data.Measurement[float64],
) *data.Measurement[float64] {
	if measurement == nil {
		return nil
	}

	if !training.grid.Settled {
		// During INIT, we feed real-time measurements to the pipeline to train the grid.
		for out := range training.pipeline.Next(data.NewValue(measurement)) {
			_ = out
		}
		// We still allow Transition(READY) to be called to notify anything waiting on the status.
		if training.grid.Settled && training.Status() != runtime.READY {
			training.Transition(runtime.READY)
		}
	} else {
		// Secondary stage: live prediction and paper trading against live tape!
		token := training.grid.LitRegions(measurement, 3)

		if len(token) > 0 {
			training.signature = append(training.signature, token...)
			training.signature = append(training.signature, 0)
			
			// Prevent signature from growing infinitely
			if len(training.signature) > 1024 {
				training.signature = training.signature[len(training.signature)-512:]
			}

			// In fully matured state, we ask engine to Evaluate sequence and pass to trader
			evalCmd := cognition.Command{
				Evaluate: &cognition.Question{
					Context: training.signature,
				},
			}

			for out := range training.engine.Next(data.NewValue(evalCmd)) {
				eval := (*cognition.Evaluation)(out)

				if eval != nil {
					measurement.EnsureMetadata()
					if eval.WinnerClass != "" {
						var prediction float64
						if eval.WinnerClass == string(cognition.ActionEnter) {
							prediction = 1
						} else if eval.WinnerClass == string(cognition.ActionExit) {
							prediction = 2
						}
						measurement.WriteMetric("frozen_prediction", prediction)
						measurement.SetMetadata("predicted_action", eval.WinnerClass)
						measurement.SetMetadata("predicted_confidence", fmt.Sprintf("%f", eval.Confidence))
					}

					if eval.WinnerClass == string(
						cognition.ActionEnter,
					) || eval.WinnerClass == string(
						cognition.ActionExit,
					) {
						training.trader.OnAction(
							measurement.Label,
							cognition.Action(eval.WinnerClass),
							eval.Confidence,
						)
					}
				}
			}
		}
		
		// Pass through the frozen grid to decorate metrics for the UI visualization
		for out := range training.pipeline.Next(data.NewValue(measurement)) {
			_ = out
		}
	}

	if training.tee != nil {
		measurement.EnsureMetadata()
		if !training.grid.Settled {
			measurement.WriteMetric("stage_code", 0.0) // MODEL DEVELOPMENT
		} else {
			measurement.WriteMetric("stage_code", 2.0) // FORWARD PAPER LEARNING
		}
		// In Step, the source should be real-time
		measurement.Source = "training:live"
		training.tee.Push(measurement)
	}

	return measurement
}

/*
CognitionTree satisfies ui.CognitionSource to broadcast the cognitive topology
and learning state to the dashboard.
*/
func (training *Training) CognitionTree() cognition.CognitionTreeExport {
	if training.engine == nil {
		return cognition.CognitionTreeExport{}
	}

	return training.engine.TreeExport()
}

/*
Run is used for training on stored data. This is done by retrieving a stream
of data from the Iceberg Tables, and replaying the past. We then need to detect
the best excursions, which will for tape fragments we can train on.
*/
func (training *Training) Run() {
	go func() {
		iteration := 1
		for {
			select {
			case <-training.Context().Done():
				return
			default:
			}

			if !training.grid.Settled {
				// We wait for the Grid to settle before running historical training
				time.Sleep(100 * time.Millisecond)
				continue
			}

			if training.catalog == nil {
				// No catalog available, can't train on historical tape.
				time.Sleep(10 * time.Second)
				continue
			}

			// Run market tape fragments from Iceberg Tables using catalog.Scan
			// In symm, measurements are the sole source of truth in Iceberg.
			// E.g. we fetch measurements for the last N epochs.
			ctx := training.Context()

			// We scan the single Measurements table for tape fragments.
			// Pipeline execution:
			// Remove unused seq

			// Cycle through the 5 excursion types described in TRAINING.md
			// 1: Upward Profitable,
			// 2: Upward Unprofitable,
			// 3: Downward,
			// 4: Choppy,
			// 5: Flat
			extType := ((iteration - 1) % 5) + 1
			iteration++

			// Use the sophisticated DuckDB Excursions primitive to find a
			// genuinely mature market excursion of the requested type
			excursions := NewExcursions(ctx, training.warehouse, extType)

			// Buffer the tape fragment? No! We process the tape as a pure stream!
			// We stream directly from DuckDB to preserve the ground truth continuously.
			var signature []byte
			for ptr := range excursions.Next(nil) {
				if ptr == nil {
					continue
				}

				m := (*data.Measurement[float64])(ptr)

				// Rewrite the source so the UI Tee allows it through to the dashboard
				m.Source = "training:historical"

				m.EnsureMetadata()

				// Apply ground truth label based on objective tape bounds from DuckDB
				var gt string
				
				switch extType {
				case 1: // Upward Profitable (Enter and Exit)
					if m.SeqIdx >= excursions.IgnitionTick-20 && m.SeqIdx <= excursions.IgnitionTick {
						gt = string(cognition.ActionEnter)
					} else if m.SeqIdx >= excursions.EndTick-20 && m.SeqIdx <= excursions.EndTick {
						gt = string(cognition.ActionExit)
					}
				case 3: // Downward (Exit)
					if m.SeqIdx >= excursions.IgnitionTick-20 && m.SeqIdx <= excursions.IgnitionTick {
						gt = string(cognition.ActionExit)
					}
				}

				if gt == "" {
					gt = string(cognition.ActionWait)
				}

				m.SetMetadata("ground_truth", gt)
				
				// 1. Get the current evaluation (Pre-Outcome Prediction) BEFORE training on this frame
				token := training.grid.LitRegions(m, 3)
				if len(token) > 0 {
					signature = append(signature, token...)
					signature = append(signature, 0) // null byte delimiter
					
					// Keep sequence bounded to avoid infinite length arrays in engine
					if len(signature) > 1024 {
						signature = signature[len(signature)-512:]
					}
					
					evalCmd := cognition.Command{
						Evaluate: &cognition.Question{
							Context: signature,
						},
					}
					
					for out := range training.engine.Next(data.NewValue(evalCmd)) {
						eval := (*cognition.Evaluation)(out)
						if eval != nil && eval.WinnerClass != "" {
							m.SetMetadata("predicted_action", eval.WinnerClass)
							m.SetMetadata("predicted_confidence", fmt.Sprintf("%f", eval.Confidence))
						}
					}
				}

				// 2. Train the model by passing it into the engine
				if len(signature) > 0 {
					training.engine.Train(signature, []byte(gt), 1.0)
				}
				
				// 3. Pass through the frozen grid to decorate metrics for the UI visualization
				for out := range training.pipeline.Next(data.NewValue(m)) {
					_ = out
				}

				// Stream the trained historical fragment directly to the dashboard visualization
				if training.tee != nil {
					m.EnsureMetadata()

					// Inject the ground truth bounds (discovered by DuckDB) strictly
					// for the visualization AFTER the model evaluated it, preventing leakage.
					m.SetMetadata("excursion_start", fmt.Sprintf("%d", excursions.StartTick))
					m.SetMetadata("excursion_ignition", fmt.Sprintf("%d", excursions.IgnitionTick))
					m.SetMetadata("excursion_extremum_tick", fmt.Sprintf("%d", excursions.EndTick))
					m.WriteMetric("excursion_type", float64(extType))
					m.WriteMetric("stage_code", 1.0) // HISTORICAL VALIDATION

					training.tee.Push(m)
				}
			}

			// Wait before scanning again to avoid busy-looping if no new excursions exist
			time.Sleep(5 * time.Second)

			// Sleep before pulling the next tape fragment.
			time.Sleep(1 * time.Second)
		}
	}()
}
