package model

import (
	"encoding/gob"

	"github.com/theapemachine/errnie"
)

/*
Clone gives a new agent independent retained evidence and no inflight actions.
The snapshot reads evidence under the model lock. Runtime tickets and pending
counts are private and are never checkpointed.
Branches carrying no completed or provisional samples are not learned state
and are omitted, including empty branches from older checkpoints.
*/
func (model *Model[Key, Action]) Clone() *Model[Key, Action] {
	model.mutex.RLock()
	defer model.mutex.RUnlock()

	cloned := New[Key, Action](model.Memory)
	cloned.Sequence = model.Sequence
	cloned.Ordered = model.Ordered

	for key, node := range model.Contexts {
		cloned.Contexts[key] = node.clone()
	}

	return cloned
}

func (node *modelContext[Action]) clone() *modelContext[Action] {
	cloned := &modelContext[Action]{Epoch: node.Epoch}

	for action, record := range node.Priors {
		if record.State.Samples == 0 && record.Provisional.Samples == 0 {
			continue
		}

		if cloned.Priors == nil {
			cloned.Priors = make(map[Action]*modelPrior)
		}

		cloned.Priors[action] = &modelPrior{
			State: record.State, Provisional: record.Provisional, Memory: record.Memory,
		}
	}

	for token, child := range node.Children {
		retained := child.clone()

		if len(retained.Priors) == 0 && len(retained.Children) == 0 {
			continue
		}

		if cloned.Children == nil {
			cloned.Children = make(map[uint64]*modelContext[Action])
		}

		cloned.Children[token] = retained
	}

	return cloned
}

/*
Encode writes the learned state through the trie's own read lock.

A checkpoint reads the same maps the observation path writes, so encoding is
the model's operation rather than something a caller performs on its fields.
Only exported state is written; runtime tickets and pending counts are not
checkpoints.
*/
func (model *Model[Key, Action]) Encode(encoder *gob.Encoder) error {
	model.mutex.RLock()
	defer model.mutex.RUnlock()

	return errnie.Error(encoder.Encode(model))
}
