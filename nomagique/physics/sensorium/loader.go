package sensorium

/*
Loader streams self-tuning token batches from tokenizer datasets.
*/
type Loader struct {
	tokenizer       *Tokenizer
	batch           int
	TotalRaw        int
	TotalCompressed int
}

func NewLoader(tokenizer *Tokenizer) *Loader {
	return &Loader{
		tokenizer: tokenizer,
		batch:     1024,
	}
}

/*
Stream iterates every dataset's yielded State and groups them into batches of
at most `batch` particles, as the resident buffer is resized per batch. Each
dataset now yields fully-formed particles directly, so the loader no longer
routes content through the byte tokenizer's MakeBatch compression.
*/
func (loader *Loader) Stream() []*State {
	if loader == nil || loader.tokenizer == nil {
		return nil
	}

	var batches []*State

	for _, dataset := range loader.tokenizer.Datasets {
		accumulated := newState(0)
		accumulated.N = 0

		for state := range dataset.Generate() {
			if state == nil || state.N == 0 {
				continue
			}

			accumulated = appendState(accumulated, state)
			loader.TotalRaw += state.N
			loader.TotalCompressed += state.N

			if accumulated.N < loader.batch {
				continue
			}

			batches = append(batches, accumulated)
			accumulated = newState(0)
			accumulated.N = 0
		}

		if accumulated.N == 0 {
			continue
		}

		batches = append(batches, accumulated)
	}

	return batches
}

/*
appendState folds one single-particle State into an accumulated batch, growing
the accumulator's columns in place.
*/
func appendState(accumulated, state *State) *State {
	return &State{
		N:          accumulated.N + state.N,
		Bytes:      append(accumulated.Bytes, state.Bytes...),
		Seqs:       append(accumulated.Seqs, state.Seqs...),
		TokenIDs:   append(accumulated.TokenIDs, state.TokenIDs...),
		ContentIDs: append(accumulated.ContentIDs, state.ContentIDs...),
		Phase:      append(accumulated.Phase, state.Phase...),
		Omega:      append(accumulated.Omega, state.Omega...),
		Energy:     append(accumulated.Energy, state.Energy...),
		Mass:       append(accumulated.Mass, state.Mass...),
		Heat:       append(accumulated.Heat, state.Heat...),
		Pos:        append(accumulated.Pos, state.Pos...),
		Vel:        append(accumulated.Vel, state.Vel...),
		Clamped:    append(accumulated.Clamped, state.Clamped...),
		Dark:       append(accumulated.Dark, state.Dark...),
	}
}
