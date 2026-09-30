package strategy

import (
	"context"
	"fmt"
	"time"

	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/hindsight/tables"
	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/cognition"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/workbench"
)

/*
Training is the learning orchestrator.
Please refer to TRAINING.md for the complete overview of how training
on pre-cursors is performed.
*/
type Training struct {
	*runtime.System
	grid     *store.Grid
	engine   *cognition.Engine
	trader   *Trader
	catalog   *tables.Catalog
	warehouse *workbench.Warehouse
	webrtc    runtime.Tee
	pipeline  core.Primitive
}

func NewTraining(
	ctx context.Context,
	price *broker.Price,
	trader *Trader,
	catalog *tables.Catalog,
	warehouse *workbench.Warehouse,
	webrtc runtime.Tee,
) *Training {
	grid := store.NewGrid()
	engine := cognition.NewEngine(cognition.Config{})

	training := &Training{
		System:   runtime.NewSystem(ctx, "training", price),
		grid:     grid,
		engine:    engine,
		trader:    trader,
		catalog:   catalog,
		warehouse: warehouse,
		webrtc:    webrtc,
	}
	
	// The pipeline for training consists of the Grid and an Adapter that 
	// transforms the Grid's LitRegions into cognitive observations.
	training.pipeline = nomagique.NewNumber(
		grid,
		data.NewAdapter(
			engine,
			func(m *data.Measurement[float64]) *cognition.Command {
				if m == nil || !grid.Settled {
					return nil
				}
				
				token := grid.LitRegions(m, 3)
				if len(token) == 0 {
					return nil
				}
				
				// For now we just observe the sequence of tokens.
				// In a full implementation, we'd buffer the signature and predict.
				return &cognition.Command{
					Observe: &cognition.Association{
						Context: token,
					},
				}
			},
			func(m *data.Measurement[float64], res *cognition.Result) {
			},
		),
	)

	// Set the status to INIT to indicate we need to build the grid until it
	// has formed stable regions, and is frozen and checkpointed.
	training.Transition(runtime.INIT)
	return training
}

func (training *Training) Register() *data.Measurement[float64] {
	measurement := data.NewMeasurement[float64]("training", nil)
	measurement.Metadata["peer-interest"] = "*"
	return measurement
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

	if training.Status() == runtime.INIT {
		// During INIT, we feed real-time measurements to the pipeline to train the grid.
		for out := range training.pipeline.Next(data.NewValue(measurement)) {
			_ = out
		}

		if training.grid.Settled {
			// Grid has frozen! Checkpoint and transition to READY.
			// The grid will no longer mutate on Update().
			training.Transition(runtime.READY)
		}
	} else if training.Status() == runtime.READY {
		// Secondary stage: live prediction and paper trading against live tape!
		token := training.grid.LitRegions(measurement, 3)
		if len(token) > 0 {
			// In fully matured state, we ask engine to Evaluate token and pass to trader
			// For now, we simulate sending the evaluate command:
			evalCmd := &cognition.Command{
				Evaluate: &cognition.Question{
					Context: token,
				},
			}
			
			for out := range training.engine.Next(data.NewValue(evalCmd)) {
				eval := (*cognition.Evaluation)(out)
				if eval != nil {
					// Predict action (Enter, Exit, Wait)
					// training.trader.Enter() or .Exit()
				}
			}
		}
	}

	return measurement
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

			if training.Status() != runtime.READY {
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
			// 1: Upward Profitable, 2: Upward Unprofitable, 3: Downward, 4: Choppy, 5: Flat
			extType := ((iteration - 1) % 5) + 1
			iteration++
			
			// Use the sophisticated DuckDB Excursions primitive to find a 
			// genuinely mature market excursion of the requested type
			excursions := NewExcursions(ctx, training.warehouse, extType)
			
			// Buffer the tape fragment to annotate the markers for the dashboard
			var fragment []*data.Measurement[float64]
			for ptr := range excursions.Next(nil) {
				if ptr != nil {
					fragment = append(fragment, (*data.Measurement[float64])(ptr))
				}
			}
			
			// The excursion primitive ensures we have a valid window of A -> B -> C.
			// It pads 50 ticks before A, and 200 ticks after B (for C).
			if len(fragment) > 100 {
				var maxPrice float64
				var minPrice float64 = 99999999999.0
				var peakTick int64
				var bottomTick int64
				
				// Find peak/bottom prices in the fragment
				for _, m := range fragment {
					if bid, ok := m.Metrics["best_bid"]; ok {
						if bid.Raw > maxPrice {
							maxPrice = bid.Raw
							peakTick = m.SeqIdx
						}
						if bid.Raw < minPrice {
							minPrice = bid.Raw
							bottomTick = m.SeqIdx
						}
					}
				}
				
				// B marker depends on the excursion type
				var ignitionTick int64
				if extType <= 2 {
					ignitionTick = peakTick
				} else if extType == 3 {
					ignitionTick = bottomTick
				} else {
					// For flat/choppy, just pick the highest point as the center of gravity
					ignitionTick = peakTick
				}
				
				// A is exactly 50 ticks before the fragment start (as padded by DuckDB)
				// Or safely just pick index 50
				startTick := fragment[0].SeqIdx
				if len(fragment) > 50 {
					startTick = fragment[50].SeqIdx
				}
				
				endTick := fragment[len(fragment)-1].SeqIdx
				
				// Replay the annotated fragment into the learning system and UI
				for _, m := range fragment {
					if m.Metadata == nil {
						m.Metadata = make(map[string]string)
					}
					
					// Annotate A, B, C markers for UI and training validation
					m.Metadata["excursion_start"] = fmt.Sprintf("%d", startTick)
					m.Metadata["excursion_ignition"] = fmt.Sprintf("%d", ignitionTick)
					m.Metadata["excursion_extremum_tick"] = fmt.Sprintf("%d", endTick)
					m.Metadata["excursion_type"] = fmt.Sprintf("%d", extType)
					m.Metadata["stage_code"] = "2" // FORWARD PAPER LEARNING
					
					// Rewrite the source so the UI Tee's `types.Filters` allows it through to the "learning" dashboard route
					m.Source = "training:historical"
					
					for out := range training.pipeline.Next(data.NewValue(m)) {
						// The Grid and Radix Trie evaluate the precursor here!
						_ = out
					}
					
					// Stream the trained historical fragment directly to the dashboard visualization!
					if training.webrtc != nil {
						training.webrtc.Push(m)
					}
				}
			}
			
			// Wait before scanning again to avoid busy-looping if no new excursions exist
			time.Sleep(5 * time.Second)
			
			// Sleep before pulling the next tape fragment.
			time.Sleep(1 * time.Second)
		}
	}()
}
