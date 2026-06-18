package resonance

import "fmt"

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
DefaultArchitecture derives network width from the sensory channel count.
*/
func DefaultArchitecture() []int {
	return DeriveArchitecture(SensoryChannelCount)
}

func DeriveArchitecture(channelCount int) []int {
	if channelCount < 2 {
		return nil
	}

	hiddenWidth := channelCount * 2
	latentModes := 3

	return []int{channelCount, hiddenWidth, latentModes}
}

func validateArchitecture(arch []int) error {
	if len(arch) < 2 {
		return errArchitectureLength
	}

	if arch[0] != SensoryChannelCount {
		return errArchitectureInput
	}

	if arch[len(arch)-1] != 3 {
		return errArchitectureLatent
	}

	return nil
}
