package cognition

import (
	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/store"
)

type Primitive struct {
	store    *store.Radix
	pipeline *nomagique.Number
}

func NewPrimitive() *Primitive {
	store := store.NewRadix()

	return &Primitive{
		store: store,
		pipeline: nomagique.NewNumber(
			NewEngine(store, Config{}),
			NewTrainer(store),
			NewDreamer(store),
		),
	}
}
