package strategy

import (
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/types"
)

/*
Persisting what the desk has learned, so a restart is not a reset.

The memory the traders build is the whole point of running them, and it lives
only in process. Without this, every restart throws away everything the tape
ever taught and begins again from an empty model — which is why the agent could
run for hours and still know nothing.

What is written is the model itself: the developments it has seen, the moves
tried in them, and the moments those moves accumulated. Not the wallets, and not
the traders. Wallets are how the memory was explored; they are meant to be
recycled. The memory is what survives.

Nothing in-flight is written. A decision the tape has not settled yet is not
knowledge, and persisting it would restore a claim nothing ever verified.
*/

/* CheckpointPrior is one action's accumulated experience in one development. */
type CheckpointPrior struct {
	Action LearningAction  `json:"action"`
	State  EconomicMoments `json:"state"`
}

/* CheckpointNode is one development path and everything learned along it. */
type CheckpointNode struct {
	Parent int               `json:"parent"`
	Token  uint64            `json:"token"`
	Priors []CheckpointPrior `json:"priors"`
}

/* CheckpointScope is one symbol-and-account-state view of the memory. */
type CheckpointScope struct {
	Symbol string           `json:"symbol"`
	State  string           `json:"state"`
	Epoch  uint64           `json:"epoch"`
	Nodes  []CheckpointNode `json:"nodes"`
}

/*
Checkpoint is the whole shared memory at one moment, in a form that can be
written down and read back.

Version travels with it. A checkpoint written by a build that measured
something differently is not evidence about this build's world, and restoring
it would silently mix two meanings of the same number.
*/
type Checkpoint struct {
	Version string            `json:"version"`
	Scopes  []CheckpointScope `json:"scopes"`
	Nodes   int               `json:"nodes"`
	Priors  int               `json:"priors"`
}

/*
CheckpointVersion names what the numbers in a checkpoint mean.

It changes whenever the meaning changes — a different objective, a different
grading scale, a different context encoding. Restoring across such a change
would carry forward numbers that no longer say what they used to.
*/
const CheckpointVersion = "separate-tape-economics-1"

/*
Checkpoint captures the shared memory for persistence.

The walk is over learned paths only. An empty node carries nothing and is not
written, so a memory that has seen little produces a small checkpoint rather
than a large one full of structure with nothing in it.
*/
func (model *EconomicModel) Checkpoint() Checkpoint {
	checkpoint := Checkpoint{Version: CheckpointVersion}

	for key, node := range model.contexts {
		scope := CheckpointScope{Symbol: key[0], State: key[1], Epoch: node.epoch}
		scope.Nodes = appendCheckpointNodes(scope.Nodes, node, -1, 0)

		if len(scope.Nodes) == 0 {
			continue
		}

		for _, written := range scope.Nodes {
			checkpoint.Priors += len(written.Priors)
		}
		checkpoint.Nodes += len(scope.Nodes)
		checkpoint.Scopes = append(checkpoint.Scopes, scope)
	}

	return checkpoint
}

/* appendCheckpointNodes walks one scope's learned paths depth first. */
func appendCheckpointNodes(
	nodes []CheckpointNode, node *economicNode, parent int, token uint64,
) []CheckpointNode {
	if node == nil {
		return nodes
	}
	written := CheckpointNode{Parent: parent, Token: token}

	for action, prior := range node.priors {
		if prior == nil || prior.state.Samples == 0 {
			continue
		}
		state := prior.state
		state.Pending = 0
		written.Priors = append(written.Priors, CheckpointPrior{Action: action, State: state})
	}

	if len(written.Priors) == 0 {
		return nodes
	}
	index := len(nodes)
	nodes = append(nodes, written)

	for token, child := range node.children {
		nodes = appendCheckpointNodes(nodes, child, index, token)
	}
	return nodes
}

/*
Restore loads a checkpoint into the model, so every trader begins from what the
desk collectively learned rather than from nothing.

A checkpoint from a different version is refused rather than adapted. Its
numbers were measured against a different meaning, and the honest thing to do
with evidence you can no longer interpret is to decline it and say so.

Restoring never resurrects anything in flight. Pending counts start at zero,
because a decision that was open when the process stopped was never settled and
never became knowledge.
*/
func (model *EconomicModel) Restore(checkpoint Checkpoint) error {
	if checkpoint.Version != CheckpointVersion {
		return errnie.Error(errnie.Err(errnie.Validation, "checkpoint: incompatible version "+checkpoint.Version, nil))
	}
	contexts := make(map[[2]string]*economicNode, len(checkpoint.Scopes))
	nodeCount, priorCount := 0, 0

	for _, scope := range checkpoint.Scopes {
		key := [2]string{scope.Symbol, scope.State}

		if contexts[key] != nil || len(scope.Nodes) == 0 {
			return errnie.Error(errnie.Err(errnie.Validation, "checkpoint: duplicate or empty scope", nil))
		}
		nodes := make([]*economicNode, len(scope.Nodes))

		for index, written := range scope.Nodes {
			node := &economicNode{}
			nodes[index] = node

			if index == 0 {
				if written.Parent != -1 || written.Token != 0 {
					return errnie.Error(errnie.Err(errnie.Validation, "checkpoint: invalid scope root", nil))
				}
				node.epoch = scope.Epoch
				contexts[key] = node
			}

			if index > 0 {
				if written.Parent < 0 || written.Parent >= index {
					return errnie.Error(errnie.Err(errnie.Validation, "checkpoint: parent must precede child", nil))
				}
				parent := nodes[written.Parent]

				if parent.children == nil {
					parent.children = make(map[uint64]*economicNode)
				}

				if parent.children[written.Token] != nil {
					return errnie.Error(errnie.Err(errnie.Validation, "checkpoint: duplicate child", nil))
				}
				parent.children[written.Token] = node
			}

			for _, restored := range written.Priors {
				switch restored.Action.Kind {
				case types.ActionEnter, types.ActionExit, types.ActionHold, types.ActionScale:
				default:
					return errnie.Error(errnie.Err(errnie.Validation, "checkpoint: unknown action", nil))
				}

				if node.priors[restored.Action] != nil || restored.State.Pending != 0 {
					return errnie.Error(errnie.Err(errnie.Validation, "checkpoint: duplicate or pending prior", nil))
				}
				prior := node.prior(restored.Action, model.memory)
				prior.state = restored.State
				priorCount++
			}
			nodeCount++
		}
	}

	if checkpoint.Nodes != nodeCount || checkpoint.Priors != priorCount {
		return errnie.Error(errnie.Err(errnie.Validation, "checkpoint: node or prior count mismatch", nil))
	}
	model.contexts = contexts
	model.pending = make(map[uint64]pendingEconomicDecision)
	model.sequence = 0
	return nil
}
