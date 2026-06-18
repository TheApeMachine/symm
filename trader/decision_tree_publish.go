package trader

import (
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/ui"
)

/*
PublishDecisionTreeSnapshot ships the embedded playbook to dashboard subscribers.
*/
func (crypto *Crypto) PublishDecisionTreeSnapshot(pool *qpool.Q[any]) error {
	if crypto == nil || pool == nil || crypto.story == nil {
		return nil
	}

	branches := crypto.story.DecisionTreeBranches()

	if len(branches) == 0 {
		return nil
	}

	return ui.PublishDecisionTree(pool, branches)
}
