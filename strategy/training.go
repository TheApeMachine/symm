package strategy

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"sync"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/nomagique/cognition"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/types"
)

type Action string

const (
	ActionEnter Action = "enter"
	ActionExit  Action = "exit"
	ActionWait  Action = "wait"
)

const TrainingFormat = "symm-training-v1"

type Training struct {
	*runtime.System
	grid   *store.Grid
	trie   *store.Radix
	trader *Trader
	tee    runtime.Tee
	mutex  sync.RWMutex
}

func NewTraining(
	ctx context.Context,
	price *broker.Price,
	trader *Trader,
	tee runtime.Tee,
) *Training {
	training := &Training{
		System: runtime.NewSystem(ctx, "training", price),
		grid:   store.NewGrid(),
		trie:   store.NewRadix(),
		trader: trader,
		tee:    tee,
	}

	training.Transition(runtime.INIT)
	return training
}

func (training *Training) Register() *data.Measurement[float64] {
	measurement := data.NewMeasurement[float64]("training", nil)
	measurement.Label = types.Focus()
	measurement.Metadata["peer-interest"] = "*"
	return measurement
}

/*
Step is the primary learning pipeline step that develops the manifold grid
from peer signal measurements and queries the radix trie for cognitive decisions.
*/
func (training *Training) Step(
	measurement *data.Measurement[float64],
) *data.Measurement[float64] {
	if measurement == nil {
		return nil
	}

	peers := measurement.Peers

	if len(peers) == 0 && len(measurement.Metrics) > 0 {
		peers = []*data.Measurement[float64]{measurement}
	}

	for _, peer := range peers {
		if peer == nil || peer.Metrics == nil || len(peer.Metrics) == 0 {
			continue
		}

		if measurement.Label == "" {
			if peer.Label != "" {
				measurement.Label = peer.Label
			}

			if peer.Label == "" {
				measurement.Label = types.Focus()
			}
		}

		if peer == measurement {
			training.grid.Update(peer)
		}

		if peer != measurement {
			training.grid.Observe(peer)
		}
	}

	var topRegion uint8
	var maxActivity float64
	var regionActivity [256]float64

	for _, peer := range peers {
		if peer == nil {
			continue
		}

		for label, metric := range peer.Metrics {
			region := training.grid.Region(label)

			if region > 0 {
				activity := math.Abs(metric.Raw)
				regionActivity[region] += activity

				if regionActivity[region] > maxActivity {
					maxActivity = regionActivity[region]
					topRegion = region
				}
			}
		}
	}

	if topRegion > 0 && maxActivity > 0 {
		token := []byte{byte(topRegion)}
		actionBytes, found := training.trie.Get(token)
		action := ActionWait

		if found && len(actionBytes) > 0 {
			action = Action(actionBytes)
		}

		if !found || len(actionBytes) == 0 {
			training.trie.Insert(token, []byte(ActionWait))
		}

		if action != ActionWait && training.trader != nil {
			training.trader.OnAction(measurement.Label, action)
		}

		if measurement.Provenance == nil {
			measurement.Provenance = make(map[string]string)
		}

		measurement.Provenance["stage"] = "MODEL DEVELOPMENT"
		measurement.Provenance["stage_blocker"] = "collecting initial historical development samples"
		measurement.Provenance["temporal_precursor"] = fmt.Sprintf("0x%02x", topRegion)

		if measurement.Metrics == nil {
			measurement.Metrics = make(map[string]data.Metric[float64])
		}

		var actionVal float64

		if action == ActionEnter {
			actionVal = 1
		}

		if action == ActionExit {
			actionVal = 2
		}

		actionMetric := data.NewMetric[float64](
			"action", data.UnitCount, data.TimescaleInstantaneous, 0, 1,
		)
		
		actionMetric.Raw = actionVal
		measurement.Metrics["action"] = actionMetric
	}

	measurement.Peers = nil
	return measurement
}

/*
Run is the primary layer of the training process, which uses the model developed by
the initial, tape fragment based training process to make decisions into the real-time market
using the paper trading mechanism.
*/
func (training *Training) Run() {
	go func() {
		select {
		case <-training.Context().Done():
			return
		default:
		}

		if training.Status() != runtime.BUSY {
			return
		}

		// Replay tape fragments to develop skill...
		for _, metric := range training.grid.Metrics {
			if metric.Region > 0 {
				token := []byte{byte(metric.Region)}
				training.trie.Insert(token, []byte(ActionWait))
			}
		}

		training.Transition(runtime.READY)

		if err := training.SaveCheckpoint(); err != nil {
			errnie.Error(err)
		}
	}()
}

var checkpointMu sync.Mutex

func (training *Training) SaveCheckpoint() error {
	checkpointMu.Lock()
	defer checkpointMu.Unlock()

	payload, err := json.MarshalIndent(struct {
		Grid *store.Grid  `json:"grid"`
		Trie *store.Radix `json:"trie"`
	}{
		Grid: training.grid,
		Trie: training.trie,
	}, "", "  ")

	if err != nil {
		return errnie.Error(err)
	}

	return os.WriteFile("grid_checkpoint.json", payload, 0644)
}

func (training *Training) LoadCheckpoint() error {
	checkpointMu.Lock()
	defer checkpointMu.Unlock()

	payload, err := os.ReadFile("grid_checkpoint.json")

	if err != nil {
		return err
	}

	state := struct {
		Grid *store.Grid  `json:"grid"`
		Trie *store.Radix `json:"trie"`
	}{
		Grid: training.grid,
		Trie: training.trie,
	}

	if err := json.Unmarshal(payload, &state); err != nil {
		return errnie.Error(err)
	}

	if training.grid.Settled {
		training.Transition(runtime.READY)
	}

	return nil
}

func (training *Training) CognitionTree() cognition.CognitionTreeExport {
	training.mutex.RLock()
	defer training.mutex.RUnlock()

	tree := training.trie.Tree()

	if tree == nil || tree.Len() == 0 {
		return cognition.CognitionTreeExport{
			Root: &cognition.TrieNodeJSON{
				ID:          "root",
				TokenPrefix: "ROOT",
				Probability: 1.0,
				State:       "ESTIMATED",
			},
			Branches: []cognition.TrieBranchJSON{},
			Feasible: []cognition.FeasibleActionJSON{},
		}
	}

	rootNode := &cognition.TrieNodeJSON{
		ID:          "root",
		TokenPrefix: "ROOT",
		Probability: 1.0,
		State:       "EVALUATED",
		Children:    make([]*cognition.TrieNodeJSON, 0),
	}

	branches := make([]cognition.TrieBranchJSON, 0)
	feasible := make([]cognition.FeasibleActionJSON, 0)
	treeLen := float64(tree.Len())

	iterator := tree.Root().Iterator()
	rank := 1

	for key, val, ok := iterator.Next(); ok; key, val, ok = iterator.Next() {
		actionStr := string(val)
		hashStr := fmt.Sprintf("0x%x", key)

		childNode := &cognition.TrieNodeJSON{
			ID:          hashStr,
			TokenPrefix: fmt.Sprintf("%s (%s)", hashStr, actionStr),
			Probability: 1.0 / treeLen,
			State:       "EVALUATED",
		}
		rootNode.Children = append(rootNode.Children, childNode)

		branches = append(branches, cognition.TrieBranchJSON{
			Hash:       hashStr,
			Depth:      len(key),
			Visits:     1,
			MeanEdge:   0.0,
			Confidence: 100.0,
			Policy:     actionStr,
		})

		feasible = append(feasible, cognition.FeasibleActionJSON{
			Rank:        rank,
			Action:      actionStr,
			TokenPrefix: fmt.Sprintf("ROOT / %s", hashStr),
			Probability: 1.0 / treeLen,
			State:       "EVALUATED",
		})
		rank++
	}

	if len(feasible) > 0 {
		feasible[0].State = "POLICY CHOICE"
	}

	return cognition.CognitionTreeExport{
		Root:     rootNode,
		Branches: branches,
		Feasible: feasible,
	}
}
