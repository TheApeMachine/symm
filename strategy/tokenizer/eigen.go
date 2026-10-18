package tokenizer

import "math"

/*
EigenMode is a lightweight phase tracker for MDL forced splits. When Trained
is false, MDL falls back to byte-to-byte delta only.
*/
type EigenMode struct {
	Trained bool
}

func NewEigenMode() *EigenMode {
	return &EigenMode{}
}

func (eigen *EigenMode) PhaseForByte(byteVal byte) (theta, phi float64) {
	if eigen == nil || !eigen.Trained {
		return 0, 0
	}

	scale := float64(byteVal) / VocabSize
	theta = scale * math.Pi
	phi = scale * math.Pi / 2

	return theta, phi
}
