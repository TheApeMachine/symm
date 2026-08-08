package types

import "time"

/*
CognitionBranch is one node in the sensory prefix tree exported for Cortex.
*/
type CognitionBranch struct {
	ID          int     `json:"id"`
	ParentID    int     `json:"parentId"`
	Token       string  `json:"token"`
	Prefix      string  `json:"prefix"`
	// Key is the machine-readable sequence address (encoded tokens joined by
	// "_"). Prefix is display text with arrow separators and cannot be split
	// back into tokens, so beam highlighting matches on Key.
	Key         string  `json:"key"`
	Depth       int     `json:"depth"`
	Probability float64 `json:"probability"`
	Count       uint64  `json:"count"`
}

/*
CognitionBeam is one scored beam-search path exported for Cortex.
*/
type CognitionBeam struct {
	Sequence string  `json:"sequence"`
	// Key is the machine-readable form of Sequence, addressing the same nodes
	// CognitionBranch.Key does.
	Key   string  `json:"key"`
	Score float64 `json:"score"`
}

/*
CognitionClass is one attractor-basin posterior exported for Cortex.
*/
type CognitionClass struct {
	Name        string  `json:"name"`
	Probability float64 `json:"probability"`
}

/*
CognitionContribution is one token's signed evidence for the winning class over
the runner-up, in bits, so a verdict can name the transition that carried it.
*/
type CognitionContribution struct {
	Token string  `json:"token"`
	Bits  float64 `json:"bits"`
}

/*
CognitionSymbol is a category path that identifies one class disproportionately.
*/
type CognitionSymbol struct {
	Symbol string  `json:"symbol"`
	Class  string  `json:"class"`
	Score  float64 `json:"score"`
	Purity float64 `json:"purity"`
}

/*
CognitionLexical records an observed category token resolved onto the vocabulary
the model actually knows.
*/
type CognitionLexical struct {
	Original   string  `json:"original"`
	Mapped     string  `json:"mapped"`
	Similarity float64 `json:"similarity"`
}

/*
Cognition is the DMT reading for one symbol's reduced category context. It
retains both the current basin classification and the next-step lookahead so
the Thesis can expose cognition as one influence among many, rather than as a
sovereign market verdict.
*/
type Cognition struct {
	Source           string             `json:"source"`
	Symbol           string             `json:"symbol"`
	At               time.Time          `json:"at"`
	Sequence         string             `json:"sequence"`
	RegimePrefix     string             `json:"regimePrefix"`
	Winner           string             `json:"winner"`
	WinnerClass      string             `json:"winnerClass,omitempty"`
	Ready            bool               `json:"ready"`
	Error            string             `json:"error,omitempty"`
	Confidence       float64            `json:"confidence"`
	ClassConfidence  float64            `json:"classConfidence,omitempty"`
	Contrast         float64            `json:"contrast"`
	ContrastEvidence float64            `json:"contrastEvidence,omitempty"`
	EntropyBits      float64            `json:"entropyBits"`
	EntropyThreshold float64            `json:"entropyThreshold"`
	Ambiguous        bool               `json:"ambiguous"`
	Cohort           uint64             `json:"cohort"`
	LookaheadScore   float64            `json:"lookaheadScore"`
	LookaheadPaths   int                `json:"lookaheadPaths"`
	BeamWidth        int                `json:"beamWidth"`
	MaxHops          int                `json:"maxHops"`
	NodeCount        int                `json:"nodeCount"`
	Predictions      map[string]float64 `json:"predictions"`
	Branches         []CognitionBranch  `json:"branches,omitempty"`
	Beams            []CognitionBeam    `json:"beams,omitempty"`
	Classes          []CognitionClass   `json:"classes,omitempty"`
	REMFrom          time.Time          `json:"remFrom,omitempty"`
	REMThrough       time.Time          `json:"remThrough,omitempty"`
	REMReplays       int                `json:"remReplays,omitempty"`

	// InterpolatedSurprisal scores the active sequence through backoff rather
	// than a single stored path, so a novel continuation of a familiar prefix is
	// surprising rather than unmeasurable.
	InterpolatedSurprisal float64 `json:"interpolatedSurprisal"`

	// NewConcept reports that this observation matched no existing basin well
	// enough to be claimed by one, and the model named a regime for it.
	NewConcept   bool   `json:"newConcept,omitempty"`
	SpawnedClass string `json:"spawnedClass,omitempty"`

	Contributions []CognitionContribution `json:"contributions,omitempty"`
	Symbols       []CognitionSymbol       `json:"symbols,omitempty"`
	Lexical       []CognitionLexical      `json:"lexical,omitempty"`
	Dreams        []string                `json:"dreams,omitempty"`
}
