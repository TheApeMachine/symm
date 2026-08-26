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
		accumulated := newStateWithCapacity(0, loader.batch)

		for state := range dataset.Generate() {
			if state == nil || state.N == 0 {
				continue
			}

			appendState(accumulated, state)
			loader.TotalRaw += state.N
			loader.TotalCompressed += state.N

			// Note: if dataset yields pooled states, they can be freed here
			// once we implement pooling in dataset.go.
			StatePool.Put(state)

			if accumulated.N < loader.batch {
				continue
			}

			batches = append(batches, accumulated)
			accumulated = newStateWithCapacity(0, loader.batch)
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
func appendState(accumulated, state *State) {
	accumulated.N += state.N
	accumulated.Bytes = append(accumulated.Bytes, state.Bytes...)
	accumulated.Seqs = append(accumulated.Seqs, state.Seqs...)
	accumulated.TokenIDs = append(accumulated.TokenIDs, state.TokenIDs...)
	accumulated.ContentIDs = append(accumulated.ContentIDs, state.ContentIDs...)
	accumulated.Phase = append(accumulated.Phase, state.Phase...)
	accumulated.Omega = append(accumulated.Omega, state.Omega...)
	accumulated.Energy = append(accumulated.Energy, state.Energy...)
	accumulated.Mass = append(accumulated.Mass, state.Mass...)
	accumulated.Heat = append(accumulated.Heat, state.Heat...)
	accumulated.Amp = append(accumulated.Amp, state.Amp...)
	accumulated.Pos = append(accumulated.Pos, state.Pos...)
	accumulated.Vel = append(accumulated.Vel, state.Vel...)
	accumulated.Clamped = append(accumulated.Clamped, state.Clamped...)
	accumulated.Dark = append(accumulated.Dark, state.Dark...)
}
