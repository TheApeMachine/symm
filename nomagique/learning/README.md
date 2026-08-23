# Predictive Coding

> In neuroscience, psychology and cognitive science, predictive coding (also known as predictive processing) 
> is a theory of brain function which postulates that the brain is constantly generating and updating a 
> "mental model" of the environment. According to the theory, such a mental model is used to predict input signals 
> from the senses that are then compared with the actual input signals from those senses. Predictive coding is 
> one member of a wider set of theories that follow the Bayesian brain hypothesis.

### Key Usage Patterns

#### 1. Quick-Start: Directional Predictive Coder (Auto-Derived Dictionary)
If you just supply `InputDim`, it automatically sets up an overcomplete sparse dictionary ($4\times$ the input dimension) and a temporal state layer:

```go
// 1. Instantiate: 4 inputs -> 16 sparse dictionary units -> 4 temporal units
coder := learning.NewPredictiveCoder(learning.PredictiveCoderConfig{
    InputDim: 4,
    Target:   learning.DirectionalTarget(0), // Up (+1) / Down (-1)
})

// 2. Feed sequence step
out, _ := coder.Step(learning.PredictiveInput{
    Features:     []float64{0.5, -0.2, 1.2, 0.1},
    Reference:    104.50, // Ground-truth reference signal to track changes of
    HasReference: true,
    Step:         1,
})

// 3. Read out:
fmt.Println("Direction:",  out.Direction)  // +1.0 (Up) or -1.0 (Down)
fmt.Println("Confidence:", out.Confidence) // [0.0, 1.0] calibrated confidence
fmt.Println("Surprise:",   out.Surprise)   // Instantaneous market surprise
```

---

#### 2. Custom Deep Hierarchy (`CustomArch`)
You can pass any arbitrary layer shape via `CustomArch`. For example, an expanding overcomplete layer followed by multi-timescale compression:

```go
// Sensory (4) -> Wide Sparse Dictionary (64) -> Micro Dynamics (16) -> Macro State (4)
coder := learning.NewPredictiveCoder(learning.PredictiveCoderConfig{
    CustomArch: []int{4, 64, 16, 4},
    MaxHorizon: 8,                       // Multi-step forward rollouts up to t+8
    Target:     learning.DirectionalTarget(0.01), // With deadband threshold
    Pace:       0.03,                    // Adaptive learning pace
    Learn:      true,
})

out, _ := coder.Step(learning.PredictiveInput{
    Features:     sensorVector,
    Reference:    groundTruthSignal,
    HasReference: true,
    Step:         currentStep,
})

// Forward return curve projected across t+1 ... t+K
fmt.Println("Forward Curve:", out.ForwardCurve)
// Temporal decay envelope of the contraction
fmt.Println("Signal Retention:", out.ForwardRetention)
// Full multi-layer representation [z1(64) + z2(16) + z3(4) + e0(4) + e1(64) + e2(16)]
fmt.Println("Readout Dimension:", len(out.Readout)) // 168 features
```

---

#### 3. Custom Target Objectives (Configured from Outside)
`nomagique` doesn't assume anything about the target; the caller passes the objective:

```go
// A. Binary increase (1.0 vs 0.0)
coderA := learning.NewPredictiveCoder(learning.PredictiveCoderConfig{
    InputDim: 4,
    Target:   learning.BinaryTarget(),
})

// B. Continuous difference (Delta)
coderB := learning.NewPredictiveCoder(learning.PredictiveCoderConfig{
    InputDim: 4,
    Target:   learning.DeltaTarget(),
})

// C. Custom Breakout Objective (only triggers on significant moves)
coderC := learning.NewPredictiveCoder(learning.PredictiveCoderConfig{
    InputDim: 4,
    Target: func(current, past float64) (float64, bool) {
        diff := current - past
        if diff > 5.0 {
            return 1.0, true // Strong Up Breakout
        } else if diff < -5.0 {
            return -1.0, true // Strong Down Breakdown
        }
        return 0.0, true // Neutral / Choppy
    },
})
```

---

#### 4. Universal `nomagique.Stream` & `Frame` Pipeline Integration
When running inside a pure `nomagique` pipeline with interned `Frame` slots:

```go
detector := learning.NewFeatureDetector(learning.FeatureDetectorConfig{
    CustomArch: []int{4, 32, 8},
})

// Wrap as a nomagique.Stream
stream := nomagique.NewStream(detector.Primitive(), types.Frame{})

input := types.Frame{}
input.Put(learning.SymbolFeatureCount, 4)
input.Put(learning.FeatureSymbol(0), 0.1)
input.Put(learning.FeatureSymbol(1), 0.4)
input.Put(learning.FeatureSymbol(2), -0.3)
input.Put(learning.FeatureSymbol(3), 0.8)

output, err := stream.Step(input)
if err != nil {
    panic(err)
}

// Access settled variational metrics and representation counts
surprise := output.MustGet(learning.SymbolSurprise)
energy   := output.MustGet(learning.SymbolEnergy)
latents  := output.MustGet(learning.SymbolLatentCount)
```

---

### File: `/nomagique/learning/example_test.go`

```go
package learning_test

import (
	"fmt"

	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/learning"
)

func ExampleNewPredictiveCoder_directional() {
	// 1. Instantiate: 3 inputs -> auto 12-dim sparse dictionary -> 4-dim temporal state
	coder := learning.NewPredictiveCoder(learning.PredictiveCoderConfig{
		InputDim:   3,
		MaxHorizon: 4,
		Target:     learning.DirectionalTarget(0.0), // Predict Up (+1) / Down (-1)
	})

	// 2. Feed sequence steps
	referenceSignal := 100.0
	for step := int64(1); step <= 5; step++ {
		referenceSignal += 1.0 // Upward trend

		out, err := coder.Step(learning.PredictiveInput{
			Features:     []float64{0.1 * float64(step), -0.2, 0.5},
			Reference:    referenceSignal,
			HasReference: true,
			Step:         step,
		})
		if err != nil {
			panic(err)
		}

		if step == 5 {
			fmt.Printf("Direction: %.1f\n", out.Direction)
			fmt.Printf("Has Forward Curve: %t\n", len(out.ForwardCurve) > 0)
			fmt.Printf("Has Dynamics: %t\n", out.Dynamics.MustGet(learning.SymbolDynamicsReady) == 1)
		}
	}

	// Output:
	// Direction: 1.0
	// Has Forward Curve: true
	// Has Dynamics: true
}

func ExampleNewPredictiveCoder_customArch() {
	// Custom deep hierarchy: Sensory (2) -> Overcomplete Dictionary (16) -> Temporal (4)
	coder := learning.NewPredictiveCoder(learning.PredictiveCoderConfig{
		CustomArch: []int{2, 16, 4},
		Target:     learning.DirectionalTarget(0.0),
	})

	out, err := coder.Step(learning.PredictiveInput{
		Features:     []float64{0.5, -0.5},
		Reference:    50.0,
		HasReference: true,
		Step:         1,
	})
	if err != nil {
		panic(err)
	}

	// Total readout dimension: [z1(16) + z2(4)] + [e0(2) + e1(16)] = 38 features
	fmt.Printf("Readout Features: %d\n", len(out.Readout))

	// Output:
	// Readout Features: 38
}

func ExampleFeatureDetector_stream() {
	// Overcomplete feature detector behind a types.Primitive
	detector := learning.NewFeatureDetector(learning.FeatureDetectorConfig{
		InputDim:      3,
		DictionaryDim: 12,
		LatentDim:     4,
	})

	stream := nomagique.NewStream(detector.Primitive(), types.Frame{})

	input := types.Frame{}
	input.Put(learning.SymbolFeatureCount, 3)
	input.Put(learning.FeatureSymbol(0), 0.2)
	input.Put(learning.FeatureSymbol(1), -0.1)
	input.Put(learning.FeatureSymbol(2), 0.4)

	output, err := stream.Step(input)
	if err != nil {
		panic(err)
	}

	fmt.Printf("Latents Extracted: %.0f\n", output.MustGet(learning.SymbolLatentCount))
	fmt.Printf("Innovations Extracted: %.0f\n", output.MustGet(learning.SymbolInnovationCount))

	// Output:
	// Latents Extracted: 16
	// Innovations Extracted: 15
}
```