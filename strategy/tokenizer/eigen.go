package tokenizer

import (
	"math"

	"github.com/theapemachine/symm/system"
)

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

	scale := float64(byteVal) / float64(system.Cfg.Learning.VocabSize)
	theta = scale * math.Pi
	phi = scale * math.Pi / 2

	return theta, phi
}
