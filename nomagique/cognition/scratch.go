package cognition

import "math"

const (
	MaxClasses      = 8
	MaxBackoffOrder = 4
	KeyScratchSize  = 128
)

type ClassEvidence struct {
	Class       [32]byte
	ClassLen    int
	Probability float64
	LogEvidence float64
}

/*
Scratch provides zero-allocation working memory for Infer() and Classify().
*/
type Scratch struct {
	Classes    [MaxClasses]ClassEvidence
	NumClasses int
	KeyBuf     [KeyScratchSize]byte
	Lookahead  [16]PackedWeight
}

func (s *Scratch) Reset() {
	s.NumClasses = 0
}

func (s *Scratch) Accumulate(class []byte, logP float64) {
	for i := 0; i < s.NumClasses; i++ {
		if s.Classes[i].ClassLen == len(class) &&
			string(s.Classes[i].Class[:s.Classes[i].ClassLen]) == string(class) {
			s.Classes[i].LogEvidence += logP
			return
		}
	}

	if s.NumClasses >= MaxClasses {
		return
	}

	idx := s.NumClasses
	s.Classes[idx].ClassLen = copy(s.Classes[idx].Class[:], class)
	s.Classes[idx].LogEvidence = logP
	s.NumClasses++
}

/*
Softmax normalizes log evidence into probabilities in place.
*/
func (s *Scratch) Softmax() {
	if s.NumClasses == 0 {
		return
	}
	maxLog := s.Classes[0].LogEvidence

	for i := 1; i < s.NumClasses; i++ {
		if s.Classes[i].LogEvidence > maxLog {
			maxLog = s.Classes[i].LogEvidence
		}
	}

	sum := 0.0

	for i := 0; i < s.NumClasses; i++ {
		p := math.Exp(s.Classes[i].LogEvidence - maxLog)
		s.Classes[i].Probability = p
		sum += p
	}

	if sum > 0 {
		invSum := 1.0 / sum
		for i := 0; i < s.NumClasses; i++ {
			s.Classes[i].Probability *= invSum
		}
	}
}
