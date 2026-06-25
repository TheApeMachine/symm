package resonance

import (
	"fmt"
)

var (
	errArchitectureLength = fmt.Errorf("resonance: architecture must contain input, hidden, and latent layers")
	errArchitectureInput  = fmt.Errorf("resonance: architecture input width must match sensory channel count")
	errArchitectureLatent = fmt.Errorf("resonance: architecture latent width must be three attention modes")
)

/*
SensoryChannelCount is the fixed width of the market-sense input vector.
*/
const SensoryChannelCount = 12

const (
	CategoryFlow     = "laminar_resonance"
	CategoryStress   = "turbulent_resonance"
	CategoryCoupling = "equilibrium"
)

/*
DefaultArchitecture derives the autoencoder shape from the sensory channel
count. Its dimensions describe the per-symbol sensory vector — input width,
hidden width, latent modes — none of which depend on how many symbols are live.
*/
func DefaultArchitecture() []int {
	return DeriveArchitecture(SensoryChannelCount)
}

func DeriveArchitecture(channelCount int) []int {
	if channelCount < 2 {
		return nil
	}

	// Hidden width is a fixed fan-out of the input channels; the latent layer
	// is exactly resonanceLatentWidth because the attention/wire layer consumes
	// that many modes. Neither scales with the live universe size.
	hiddenWidth := channelCount * 2

	return []int{channelCount, hiddenWidth, resonanceLatentWidth}
}

func validateArchitecture(arch []int) error {
	if len(arch) < 2 {
		return errArchitectureLength
	}

	if arch[0] != SensoryChannelCount {
		return errArchitectureInput
	}

	if arch[len(arch)-1] < 1 || arch[len(arch)-1] > 3 {
		return errArchitectureLatent
	}

	return nil
}
