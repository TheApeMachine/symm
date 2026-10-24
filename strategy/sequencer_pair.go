package strategy

import "github.com/theapemachine/symm/strategy/tokenizer"

/*
symbolSequencers keeps a flat-phase sequencer and a holding-phase sequencer
so a repeated letter in one phase does not silence the other.
*/
type symbolSequencers struct {
	enter *tokenizer.Sequencer
	exit  *tokenizer.Sequencer
}

func newSymbolSequencers() *symbolSequencers {
	return &symbolSequencers{
		enter: tokenizer.NewSequencer(),
		exit:  tokenizer.NewSequencer(),
	}
}

func (pair *symbolSequencers) ForEnter(enter bool) *tokenizer.Sequencer {
	if pair == nil {
		return nil
	}

	if enter {
		return pair.enter
	}

	return pair.exit
}
