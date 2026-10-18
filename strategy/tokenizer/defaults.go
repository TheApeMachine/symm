package tokenizer

/*
Defaults mirror the numeric limits used by the six sequencer MDL/calibrator
stack, kept local so tokenizer does not import six.
*/
const (
	VocabSize         = 256
	DefaultWindowSize = VocabSize
	ShannonCapacity   = 0.45
)
