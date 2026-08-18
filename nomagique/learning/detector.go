package learning

import (
	"fmt"

	"github.com/theapemachine/symm/nomagique"
)

/*
FeatureDetectorConfig configures the 1-2 line plug-and-play feature detection manifold.
*/
type FeatureDetectorConfig struct {
	// InputDim is the dimension of the raw sensory input frame (e.g. 4).
	InputDim int
	// DictionaryDim is the overcomplete layer size (e.g. 32). Set to 0 to skip overcomplete expansion.
	DictionaryDim int
	// LatentDim is the compact macro/temporal state layer size (e.g. 8).
	LatentDim int
	// CustomArch optionally overrides with an explicit architecture slice (e.g. []int{4, 64, 16, 4}).
	CustomArch []int
	// TargetDim enables the online RLS supervised readout head when > 0.
	TargetDim int
	// Pace sets the adaptive learning rate (defaults to 0.03).
	Pace float64
	// Learn enables continuous online learning on every step (defaults to true).
	Learn bool
}

/*
FeatureOutput carries the complete, self-supervised representation output.
*/
type FeatureOutput struct {
	Energy              float64
	Surprise            float64
	ReconstructionError float64
	InferenceSteps      int
	Readout             []float64
	Prediction          []float64
}

/*
FeatureDetector provides a single-line, zero-allocation predictive coding component
integrating dictionary learning, multi-timescale temporal matrices, and innovation features.
*/
type FeatureDetector struct {
	manifold  *ResonanceManifold
	primitive nomagique.Primitive
	learn     bool
	inputBuf  nomagique.Frame
}

/*
NewFeatureDetector instantiates an abstracted feature detector in one line.
*/
func NewFeatureDetector(config FeatureDetectorConfig) *FeatureDetector {
	pace := config.Pace
	if pace <= 0 {
		pace = 0.03
	}

	arch := config.CustomArch
	if len(arch) == 0 {
		inputDim := config.InputDim
		if inputDim <= 0 {
			inputDim = 4
		}

		dictDim := config.DictionaryDim
		if dictDim <= 0 {
			dictDim = inputDim * 4 // Default to 4x overcomplete dictionary
		}

		latentDim := config.LatentDim
		if latentDim <= 0 {
			latentDim = max(2, inputDim)
		}

		arch = []int{inputDim, dictDim, latentDim}
	}

	learn := true
	if !config.Learn && config.Pace < 0 {
		learn = false
	}

	manifold := NewResonanceManifold(arch, config.TargetDim, pace)
	primitive := FramePrimitive(manifold, learn)

	return &FeatureDetector{
		manifold:  manifold,
		primitive: primitive,
		learn:     learn,
	}
}

/*
Primitive returns the nomagique.Primitive reducer for use with nomagique.Stream,
KeyedStreams, or pipeline composition.
*/
func (fd *FeatureDetector) Primitive() nomagique.Primitive {
	return fd.primitive
}

/*
Manifold exposes the underlying ResonanceManifold for advanced inspection.
*/
func (fd *FeatureDetector) Manifold() *ResonanceManifold {
	return fd.manifold
}

/*
Step processes raw slice values and returns the structured FeatureOutput in one line.
*/
func (fd *FeatureDetector) Step(values ...float64) (FeatureOutput, error) {
	if len(values) != fd.manifold.arch[0] {
		return FeatureOutput{}, fmt.Errorf(
			"detector: expected %d inputs, got %d",
			fd.manifold.arch[0], len(values),
		)
	}

	if err := fd.manifold.Settle(values, !fd.learn); err != nil {
		return FeatureOutput{}, err
	}

	if fd.learn {
		if err := fd.manifold.Learn(nil); err != nil {
			return FeatureOutput{}, err
		}
	}

	return FeatureOutput{
		Energy:              fd.manifold.Energy(),
		Surprise:            fd.manifold.ReconstructionError(),
		ReconstructionError: fd.manifold.ReconstructionError(),
		InferenceSteps:      fd.manifold.lastInferenceSteps,
		Readout:             fd.manifold.ReadoutVector(),
		Prediction:          fd.manifold.TaskPrediction(),
	}, nil
}

/*
StepFrame accepts and updates a nomagique.Frame in one line.
*/
func (fd *FeatureDetector) StepFrame(input nomagique.Frame) (nomagique.Frame, error) {
	_, output, err := fd.primitive(nomagique.Frame{}, input)
	return output, err
}
