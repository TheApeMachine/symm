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
count. Its dimensions describe the per-symbol sensory hierarchy — sensory input,
micro expansion, meso compression, and macro latent modes — none of which depend
on how many symbols are live.
*/
func DefaultArchitecture() []int {
	return DeriveArchitecture(SensoryChannelCount)
}

func DeriveArchitecture(channelCount int) []int {
	if channelCount < 2 {
		return nil
	}

	// The wire view is intentionally four levels for x-ray: sensory channels,
	// micro fan-out, meso compression, then the macro attention modes. These
	// widths are functions of the channel contract, not live universe size.
	microWidth := channelCount * 2
	mesoWidth := channelCount

	return []int{channelCount, microWidth, mesoWidth, resonanceLatentWidth}
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
