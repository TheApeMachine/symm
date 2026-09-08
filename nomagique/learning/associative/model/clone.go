package model

/*
Clone gives a new agent independent retained evidence and no inflight actions.
Exported fields are the original learned state and can be encoded directly;
runtime tickets and pending counts are private and are never checkpointed.
*/
func (model *Model[Key, Action]) Clone() *Model[Key, Action] {
	cloned := New[Key, Action](model.Memory)
	cloned.Sequence = model.Sequence
	cloned.Ordered = model.Ordered

	for key, node := range model.Contexts {
		cloned.Contexts[key] = node.clone()
	}

	return cloned
}

func (node *modelContext[Action]) clone() *modelContext[Action] {
	cloned := &modelContext[Action]{
		Epoch:    node.Epoch,
		Priors:   make(map[Action]*modelPrior, len(node.Priors)),
		Children: make(map[uint64]*modelContext[Action], len(node.Children)),
	}

	for action, record := range node.Priors {
		cloned.Priors[action] = &modelPrior{
			State: record.State, Provisional: record.Provisional, Memory: record.Memory,
		}
	}

	for token, child := range node.Children {
		cloned.Children[token] = child.clone()
	}

	return cloned
}
