package trader

/*
CandidateFrame is one scored rescore publish for the trader decision loop.
*/
type CandidateFrame struct {
	Executable    []SignalCandidate
	Live          []SignalCandidate
	Ready         bool
	PulseSeq      int
	TrustWeights  map[string]float64
	MeasurementCount int
}
