package tune

import (
	"context"
	"runtime"
	"sync"
	"time"

	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/market"
	"github.com/theapemachine/symm/market/perspectives"
	"github.com/theapemachine/symm/optimizer/budget"
	"github.com/theapemachine/symm/optimizer/mcts"
	"github.com/theapemachine/symm/optimizer/profile"
	"github.com/theapemachine/symm/optimizer/replay"
	"github.com/theapemachine/symm/optimizer/types"
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
	profile     profile.Profile
	tree        *perspectives.Tree
	branches    perspectives.BranchList
	ringWindow  []perspectives.Measurement
	trader      *replay.Trader
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
		profile:     *profile.NewProfile(ctx),
	}

	for _, channel := range []string{"measurements", "actions"} {
		tuner.broadcasts[channel] = pool.CreateBroadcastGroup(channel, 10*time.Millisecond)
	}

	tuner.subscribers["measurements"] = tuner.broadcasts["measurements"].Subscribe(
		"optimizer:tuner", 1024,
	)

	return tuner
}

/*
BindTrader wires the replay evaluator used to score candidate trees.
*/
func (tuner *Tuner) BindTrader(trader *replay.Trader) {
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

		return replay.NewReplaySimulation(tuner.ctx, branches, rows).Score()
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

	tuner.ringWindow = market.AppendRingMeasurement(tuner.ringWindow, measurement)
	snapshots := market.RingSnapshot(tuner.ringWindow, measurement.Symbol)
	tuner.tree.ResetWalk()

	action := tuner.tree.Walk(snapshots, tuner.tree.Branches()...)

	if action == perspectives.ActionNone {
		return
	}

	tuner.publish(measurement, action)
}

func (tuner *Tuner) publish(
	measurement perspectives.Measurement, actionType perspectives.ActionType,
) {
	tuner.broadcasts["actions"].Send(&qpool.QValue[any]{
		Value: perspectives.ActionFromMeasurement(actionType, measurement),
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
	tape := replay.PrecompileTape(rows)
	searchBudget := budget.DeriveSearchBudget(&tuner.profile, tape, runtime.NumCPU())

	branches, _, _ := mcts.RunHybridSearch(
		tuner.ctx,
		&tuner.profile,
		rows,
		tape,
		mcts.HybridOptions{
			ScanOptions: types.ScanOptions{
				Workers: runtime.NumCPU(),
				Budget:  searchBudget,
				Pool:    tuner.pool,
			},
			MCTSOptions: mcts.Options{
				Budget: searchBudget,
			},
			SeedCount: searchBudget.HybridSeedCount,
		},
	)

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
Summary reports the current session counters.
*/
func (tuner *Tuner) Summary() types.SessionSummary {
	tuner.mu.Lock()
	defer tuner.mu.Unlock()

	summary := types.SessionSummary{
		MeasurementCount: tuner.profile.Len(),
		BranchCount:      len(tuner.branches),
	}

	if len(tuner.branches) > 0 {
		summary.BestScore = tuner.evaluator()(tuner.branches)
	}

	return summary
}
