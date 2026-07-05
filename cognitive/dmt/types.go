package dmt

/*
CognitiveState stores learned support for one sensory prefix or attractor basin.
*/
type CognitiveState struct {
	Count       uint64  `json:"count"`
	Probability float64 `json:"probability"`
}

/*
LookaheadPrediction is the next-token probability read from sensory prefixes.
*/
type LookaheadPrediction struct {
	Token       []byte
	Probability float64
}

/*
ClassScore records one posterior class probability.
*/
type ClassScore struct {
	ClassName []byte
	Value     float64
}

/*
ClassificationResult is the sorted posterior matrix for one input sequence.
*/
type ClassificationResult struct {
	Scores  []ClassScore
	Winner  []byte
	Highest float64
}

/*
ClassificationScratch exists so callers can reuse the same typed API without
allocating an artifact-shaped scratch carrier.
*/
type ClassificationScratch struct{}

/*
BeamPath is one scored multi-hop sensory prefix.
*/
type BeamPath struct {
	Sequence []byte
	Score    float64
}

/*
BeamSearchScratch holds reusable beam buffers for live classification.
*/
type BeamSearchScratch struct {
	CurrentBeams []BeamPath
	NextBeams    []BeamPath
	LookupBuffer []LookaheadPrediction
	PathBuffer   []byte
}
