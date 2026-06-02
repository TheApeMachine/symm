package optimizer

import (
	"context"
	"fmt"
	"runtime"
	"sync"
	"time"

	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/market/perspectives"
)

/*
Tuner is the tune-time stand-in for market.Story: it ingests measurements,
searches perspective branch trees with ScanSearch, and publishes actions.
*/
type Tuner struct {
	ctx         context.Context
	cancel      context.CancelFunc
	pool        *qpool.Q
	broadcasts  map[string]*qpool.BroadcastGroup
	subscribers map[string]*qpool.Subscriber
	profile     Profile
	tree        *perspectives.Tree
	branches    perspectives.BranchList
	ringWindow  []perspectives.Measurement
	trader      *Trader
	mu          sync.Mutex
	finished    bool
}

/*
NewTuner creates a new Tuner.
*/
func NewTuner(ctx context.Context, pool *qpool.Q) *Tuner {
	ctx, cancel := context.WithCancel(ctx)

	tuner := &Tuner{
		ctx:         ctx,
		cancel:      cancel,
		pool:        pool,
		broadcasts:  make(map[string]*qpool.BroadcastGroup),
		subscribers: make(map[string]*qpool.Subscriber),
		profile:     Profile{ctx: ctx},
	}

	for _, channel := range []string{"measurements", "actions"} {
		tuner.broadcasts[channel] = pool.CreateBroadcastGroup(channel, 10*time.Millisecond)
	}

	tuner.subscribers["measurements"] = tuner.broadcasts["measurements"].Subscribe(
		"optimizer:tuner", 128,
	)

	return tuner
}

/*
BindTrader wires the replay evaluator used to score candidate trees.
*/
func (tuner *Tuner) BindTrader(trader *Trader) {
	tuner.mu.Lock()
	defer tuner.mu.Unlock()

	tuner.trader = trader
}

func (tuner *Tuner) evaluator() func(perspectives.BranchList) float64 {
	return func(branches perspectives.BranchList) float64 {
		rows := tuner.profile.Rows()

		if tuner.trader != nil {
			return tuner.trader.Evaluate(branches, rows)
		}

		return NewReplaySimulation(tuner.ctx, branches, rows).Score()
	}
}

/*
Tick ingests measurements and walks the current best tree.
*/
func (tuner *Tuner) Tick() error {
	for {
		select {
		case <-tuner.ctx.Done():
			tuner.Finish()

			return tuner.ctx.Err()
		case row, ok := <-tuner.subscribers["measurements"].Incoming:
			if !ok {
				tuner.Finish()

				return nil
			}

			if row == nil {
				continue
			}

			measurement, ok := row.Value.(perspectives.Measurement)

			if !ok {
				continue
			}

			tuner.ingest(measurement)
		}
	}
}

func (tuner *Tuner) ingest(measurement perspectives.Measurement) {
	tuner.mu.Lock()
	defer tuner.mu.Unlock()

	tuner.profile.Add(measurement)

	if tuner.tree == nil {
		return
	}

	tuner.ringWindow = appendRingMeasurement(tuner.ringWindow, measurement)
	snapshots := ringSnapshot(tuner.ringWindow, measurement.Symbol)
	tuner.tree.ResetWalk()

	action := tuner.tree.Walk(snapshots, tuner.tree.Branches()...)

	if action == nil {
		return
	}

	tuner.publish(measurement, *action)
}

func (tuner *Tuner) publish(
	measurement perspectives.Measurement, actionType perspectives.ActionType,
) {
	tuner.broadcasts["actions"].Send(&qpool.QValue[any]{
		Value: perspectives.Action{
			Type:   actionType,
			Symbol: measurement.Symbol,
			Price:  measurement.Last,
		},
	})
}

/*
Finish runs hybrid beam+MCTS search once replay measurements are collected.
*/
func (tuner *Tuner) Finish() {
	tuner.mu.Lock()
	defer tuner.mu.Unlock()

	if tuner.finished {
		return
	}

	tuner.finished = true
	rows := tuner.profile.Rows()

	branches, _, err := RunHybridSearch(
		tuner.ctx,
		&tuner.profile,
		rows,
		HybridOptions{
			ScanOptions: ScanOptions{
				Workers:           runtime.NumCPU(),
				MaxThresholds:     DefaultScanMaxThresholds,
				BeamWidth:         DefaultScanBeamWidth,
				CandidateLimit:    DefaultScanCandidateLimit,
				MaxReasoningSteps: DefaultMaxReasoningSteps,
			},
			MCTSOptions: MCTSOptions{
				Iterations:        DefaultMCTSIterations,
				MaxReasoningSteps: DefaultMaxReasoningSteps,
			},
			SeedCount:    DefaultHybridSeedCount,
			ShallowDepth: DefaultHybridShallowDepth,
		},
	)

	if err != nil {
		return
	}

	tuner.branches = branches

	tree, err := perspectives.NewTreeFromBranches(tuner.ctx, tuner.branches)

	if err != nil {
		return
	}

	tuner.tree = tree
}

/*
Branches returns the best branch registry found by ScanSearch.
*/
func (tuner *Tuner) Branches() perspectives.BranchList {
	tuner.mu.Lock()
	defer tuner.mu.Unlock()

	return tuner.branches.Clone()
}

/*
Close shuts down the tuner.
*/
func (tuner *Tuner) Close() error {
	tuner.Finish()
	tuner.cancel()

	return nil
}

/*
SessionSummary is the optimizer output for one replay pass.
*/
type SessionSummary struct {
	MeasurementCount int     `json:"measurement_count"`
	BranchCount      int     `json:"branch_count"`
	Candidates       int     `json:"candidates"`
	Workers          int     `json:"workers"`
	HybridSeeds      int     `json:"hybrid_seeds,omitempty"`
	MCTSRounds       int     `json:"mcts_rounds,omitempty"`
	BestScore        float64 `json:"best_score"`
}

/*
Summary reports the current session counters.
*/
func (tuner *Tuner) Summary() SessionSummary {
	tuner.mu.Lock()
	defer tuner.mu.Unlock()

	summary := SessionSummary{
		MeasurementCount: tuner.profile.Len(),
		BranchCount:      len(tuner.branches),
	}

	if len(tuner.branches) > 0 {
		summary.BestScore = tuner.evaluator()(tuner.branches)
	}

	return summary
}

/*
String formats the session summary for stderr.
*/
func (summary SessionSummary) String() string {
	if summary.MCTSRounds > 0 {
		return fmt.Sprintf(
			"measurements=%d branches=%d candidates=%d workers=%d seeds=%d mcts=%d score=%.6f",
			summary.MeasurementCount,
			summary.BranchCount,
			summary.Candidates,
			summary.Workers,
			summary.HybridSeeds,
			summary.MCTSRounds,
			summary.BestScore,
		)
	}

	if summary.Candidates > 0 {
		return fmt.Sprintf(
			"measurements=%d branches=%d candidates=%d workers=%d score=%.6f",
			summary.MeasurementCount,
			summary.BranchCount,
			summary.Candidates,
			summary.Workers,
			summary.BestScore,
		)
	}

	return fmt.Sprintf(
		"measurements=%d branches=%d score=%.6f",
		summary.MeasurementCount,
		summary.BranchCount,
		summary.BestScore,
	)
}
