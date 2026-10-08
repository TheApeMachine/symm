package cognition

type Record struct {
	Context  string
	Class    string
	Feedback float64
	Graded   float64
}

type TrainRecord struct {
	Context  string
	Class    string
	Feedback float64
	Graded   float64
}

type RecallQuery struct {
	Context string
	Stance  string
}

type RecallResult struct {
	Winner       string
	RunnerUp     string
	Confidence   float64
	Contrast     float64
	Support      float64
	Ambiguity    float64
	Surprisal    float64
	HasSurprisal bool
}

type CensusResult struct {
	Records float64
	Span    float64
	Classes map[string]float64
}
