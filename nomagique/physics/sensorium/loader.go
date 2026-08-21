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

func (loader *Loader) Stream() []*State {
	if loader == nil || loader.tokenizer == nil {
		return nil
	}

	var batches []*State

	for _, dataset := range loader.tokenizer.Datasets {
		pairs := dataset.Generate()
		bytes := make([]int64, 0, loader.batch)
		seqs := make([]int64, 0, loader.batch)

		for _, pair := range pairs {
			bytes = append(bytes, pair[0])
			seqs = append(seqs, pair[1])

			if len(bytes) < loader.batch {
				continue
			}

			if batch := loader.emit(bytes, seqs); batch != nil {
				batches = append(batches, batch)
			}

			bytes = bytes[:0]
			seqs = seqs[:0]
		}

		if len(bytes) == 0 {
			continue
		}

		if batch := loader.emit(bytes, seqs); batch != nil {
			batches = append(batches, batch)
		}
	}

	return batches
}

func (loader *Loader) emit(bytes, seqs []int64) *State {
	raw := len(bytes)
	loader.TotalRaw += raw
	batch, err := loader.tokenizer.MakeBatch(bytes, seqs)

	if err != nil || batch == nil {
		return nil
	}

	loader.TotalCompressed += batch.N
	return batch
}
