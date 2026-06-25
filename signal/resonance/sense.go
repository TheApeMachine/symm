package resonance

import (
	"fmt"
	"math"

	"github.com/theapemachine/symm/statutil"
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
DefaultArchitecture derives network width from the sensory channel count.
*/
func DefaultArchitecture(batchSize int) []int {
	if batchSize < 1 {
		batchSize = 1
	}

	return DeriveArchitecture(SensoryChannelCount, batchSize)
}

func DeriveArchitecture(channelCount, batchSize int) []int {
	if channelCount < 2 {
		return nil
	}

	if batchSize < 1 {
		batchSize = 1
	}

	hiddenScale := statutil.SampleBudgetFromCadence(float64(batchSize))

	if hiddenScale < 2 {
		hiddenScale = 2
	}

	hiddenWidth := channelCount * hiddenScale / 2

	if hiddenWidth < channelCount*2 {
		hiddenWidth = channelCount * 2
	}

	latentModes := int(math.Ceil(math.Sqrt(float64(batchSize))))

	if latentModes < 1 {
		latentModes = 1
	}

	if latentModes > 3 {
		latentModes = 3
	}

	return []int{channelCount, hiddenWidth, latentModes}
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
