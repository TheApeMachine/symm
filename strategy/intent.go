package strategy

import "github.com/theapemachine/symm/types"

/*
Intent carries one selected Thesis decision into broker without changing its
meaning. The index keeps the command attached to the journal entry it executes.
*/
type Intent struct {
	Thesis   *types.Thesis
	Decision int
}

/*
Selected returns the immutable decision selected by strategy for execution.
*/
func (intent Intent) Selected() types.Decision {
	return intent.Thesis.Decisions[intent.Decision]
}
