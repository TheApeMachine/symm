// manifoldexperiment replays explicit resident-state batches through the current
// Sensorium. It does not recreate the removed market-coordinate A/B experiment.
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"reflect"
	"strings"

	"github.com/theapemachine/symm/nomagique/physics/sensorium"
)

const replaySchema = "sensorium-state-replay/v1"

// ExperimentCommand owns input/output and an explicit per-replay compute budget.
// The old geometry/denomination flags are intentionally not accepted: their
// deleted A/B projection model is not equivalent to the current resident field.
type ExperimentCommand struct {
	Input, Output        string
	ConfigPath           string
	configuration        *StudyConfiguration
	Grid, Steps, Repeats int
}

type ReplayFrame struct {
	Schema     string           `json:"schema"`
	State      *sensorium.State `json:"state"`
	Departures []int64          `json:"departures,omitempty"`
}
type ReplaySample struct {
	Frame, Population int
	Reading           sensorium.Reading
}
type ReplayRun struct {
	Steps      int
	Trajectory []ReplaySample
	Failure    string
}

// Geometry and controls are fixed before the first advance. This is a parameter
// study over the actual engine, not a replacement numerical or projection model.
type StudyConfiguration struct {
	ID                      string
	Controls                sensorium.PhysicsControls
	Potential, MetricVolume []float32
}

type ExperimentReport struct {
	ConfigurationID, ConfigurationSHA256 string
	NativeEngineExercised                bool
	NumericalImplementation              string
	Schema                               string
	InputSHA256                          string
	Grid, Budget, Runs                   int
	ReplayExercised, ExactReplayEquality bool
	MaximumReadingDifference             float64
	Replays                              []ReplayRun
}

func main() {
	command := &ExperimentCommand{}
	flag.StringVar(&command.Input, "input", "", "sensorium-state-replay/v1 JSONL input (not raw market capture)")
	flag.StringVar(&command.ConfigPath, "configuration", "", "optional fixed physical-study JSON configuration")
	flag.StringVar(&command.Output, "output", "", "replay report JSON path")
	flag.IntVar(&command.Grid, "grid", 32, "spatial cells per axis")
	flag.IntVar(&command.Steps, "steps", 0, "required maximum field advances per replay")
	flag.IntVar(&command.Repeats, "repeats", 2, "identical-input repeat count")
	flag.Parse()
	if err := command.Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func (command *ExperimentCommand) Run() (err error) {
	if command.Input == "" || command.Output == "" || command.Grid < 2 || command.Steps < 1 || command.Repeats < 1 {
		return errors.New("input, output, grid >= 2 and positive steps/repeats are required")
	}
	configHash := ""
	if command.ConfigPath != "" {
		raw, e := os.ReadFile(command.ConfigPath)
		if e != nil {
			return e
		}
		configuration := StudyConfiguration{Controls: sensorium.DefaultPhysicsControls()}
		reader := json.NewDecoder(strings.NewReader(string(raw)))
		reader.DisallowUnknownFields()
		if e = reader.Decode(&configuration); e != nil {
			return fmt.Errorf("study configuration: %w", e)
		}
		var trailing any
		if reader.Decode(&trailing) != io.EOF {
			return errors.New("trailing study configuration content")
		}
		if configuration.ID == "" {
			return errors.New("configuration ID required")
		}
		hash := sha256.Sum256(raw)
		configHash = hex.EncodeToString(hash[:])
		command.configuration = &configuration
	}
	input, err := os.Open(command.Input)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, input.Close()) }()
	// Never overwrite the capture being checked, even through a hard link.
	if outputInfo, statErr := os.Stat(command.Output); statErr == nil {
		inputInfo, statErr := input.Stat()
		if statErr != nil {
			return statErr
		}
		if os.SameFile(inputInfo, outputInfo) {
			return errors.New("replay output must differ from input")
		}
	} else if !os.IsNotExist(statErr) {
		return statErr
	}
	digest := sha256.New()
	if _, err = io.Copy(digest, input); err != nil {
		return err
	}
	report := ExperimentReport{Schema: replaySchema, InputSHA256: hex.EncodeToString(digest.Sum(nil)), Grid: command.Grid, Budget: command.Steps, Runs: command.Repeats, ExactReplayEquality: true}
	report.NumericalImplementation = "native-device-only"
	report.ConfigurationID = "default"
	report.ConfigurationSHA256 = configHash
	if command.configuration != nil {
		report.ConfigurationID = command.configuration.ID
	}
	var failures error
	for repeat := 0; repeat < command.Repeats; repeat++ {
		if _, err = input.Seek(0, io.SeekStart); err != nil {
			return err
		}
		run, runErr := command.replay(input)
		report.Replays = append(report.Replays, run)
		if runErr != nil {
			failures = errors.Join(failures, fmt.Errorf("replay %d: %w", repeat+1, runErr))
			report.ExactReplayEquality = false
		}
		if repeat == 0 {
			continue
		}
		original := report.Replays[0]
		report.ExactReplayEquality = report.ExactReplayEquality && reflect.DeepEqual(original.Trajectory, run.Trajectory)
		for index, sample := range run.Trajectory {
			if index >= len(original.Trajectory) {
				break
			}
			left, right := sample.Reading, original.Trajectory[index].Reading
			for _, difference := range []float64{left.Divergence - right.Divergence, left.GuidanceSpeed - right.GuidanceSpeed, left.CoherenceMag2 - right.CoherenceMag2, left.PressureGradNorm - right.PressureGradNorm, left.ViscosityProxy - right.ViscosityProxy, left.KuramotoR - right.KuramotoR} {
				report.MaximumReadingDifference = math.Max(report.MaximumReadingDifference, math.Abs(difference))
			}
		}
	}
	report.NativeEngineExercised = failures == nil && len(report.Replays) > 0 && report.Replays[0].Steps > 0
	report.ReplayExercised = command.Repeats > 1 && failures == nil && report.Replays[0].Steps > 0
	report.ExactReplayEquality = report.ExactReplayEquality && report.ReplayExercised
	output, err := os.Create(command.Output)
	if err != nil {
		return err
	}
	encoder := json.NewEncoder(output)
	encoder.SetIndent("", "  ")
	if err = encoder.Encode(report); err != nil {
		return errors.Join(err, output.Close())
	}
	if err = output.Close(); err != nil {
		return err
	}
	fmt.Printf("Resident-state replay: runs=%d exercised=%t exact_equal=%t report=%s\n", report.Runs, report.ReplayExercised, report.ExactReplayEquality, command.Output)
	return failures
}

func (command *ExperimentCommand) replay(input io.Reader) (run ReplayRun, err error) {
	defer func() {
		if err != nil {
			run.Failure = err.Error()
		}
	}()
	decoder := json.NewDecoder(input)
	decoder.DisallowUnknownFields()
	var physics *sensorium.Manifold
	defer func() {
		if physics != nil {
			physics.Close()
		}
	}()
	for frameIndex := 0; run.Steps < command.Steps; frameIndex++ {
		var frame ReplayFrame
		if err = decoder.Decode(&frame); err == io.EOF {
			err = nil
			break
		} else if err != nil {
			return run, err
		}
		if err = frame.validate(); err != nil {
			return run, fmt.Errorf("frame %d: %w", frameIndex+1, err)
		}
		if physics == nil {
			physics = sensorium.NewManifold(command.Grid, command.Grid, command.Grid)
			if physics == nil {
				return run, errors.New("Sensorium initialization failed; a native Metal or CUDA runtime and compiled kernels are required")
			}

			if command.configuration != nil {
				if e := physics.SetPhysicsControls(command.configuration.Controls); e != nil {
					return run, e
				}
				if e := physics.SetSpectralGeometry(command.configuration.Potential, command.configuration.MetricVolume); e != nil {
					return run, e
				}
			}
		}
		remaining, removeErr := physics.Remove(frame.Departures)
		if removeErr != nil {
			return run, removeErr
		}
		if remaining == 0 && (frame.State == nil || frame.State.N == 0) {
			continue
		}
		state, stepErr := physics.Step(frame.State)
		if stepErr != nil {
			return run, stepErr
		}
		if state == nil || state.N == 0 {
			return run, errors.New("field returned no resident population")
		}
		reading := physics.Reading()
		if !reading.IsFinite() {
			return run, errors.New("field returned a non-finite reading")
		}
		run.Steps++
		run.Trajectory = append(run.Trajectory, ReplaySample{Frame: frameIndex + 1, Population: state.N, Reading: reading})
	}
	if run.Steps == 0 {
		return run, errors.New("input exercised no physics advances")
	}
	return run, nil
}

// validate rejects malformed batches before GPU code indexes their tensors.
func (frame ReplayFrame) validate() error {
	if frame.Schema != replaySchema {
		return fmt.Errorf("schema must be %q; raw market captures require an explicit projection export", replaySchema)
	}
	s := frame.State
	if s == nil {
		if len(frame.Departures) == 0 {
			return errors.New("state or departures required")
		}
		return nil
	}
	n := s.N
	if n < 0 {
		return errors.New("negative population")
	}
	for _, length := range []int{len(s.Bytes), len(s.Seqs), len(s.TokenIDs), len(s.ContentIDs), len(s.Phase), len(s.Omega), len(s.Energy), len(s.Mass), len(s.Heat), len(s.Amp), len(s.Clamped), len(s.Dark)} {
		if length != n {
			return errors.New("tensor length does not match N")
		}
	}
	if len(s.Pos) != 3*n || len(s.Vel) != 3*n {
		return errors.New("position and velocity must have 3*N values")
	}
	for _, field := range []struct {
		name   string
		values []float32
		count  int
	}{
		{"material energy", s.MaterialEnergy, n}, {"previous pilot", s.PilotVel, 3 * n}, {"previous phase potential", s.PhasePotential, n}, {"previous coherence position", s.CoherencePosition, 3 * n},
	} {
		if len(field.values) != 0 && len(field.values) != field.count {
			return fmt.Errorf("partial %s metadata", field.name)
		}
		for _, v := range field.values {
			if math.IsNaN(float64(v)) || math.IsInf(float64(v), 0) {
				return fmt.Errorf("nonfinite %s", field.name)
			}
		}
	}
	for _, v := range s.MaterialEnergy {
		if v < 0 {
			return errors.New("negative material total energy")
		}
	}
	seen := make(map[int64]bool, n)
	for i, id := range s.ContentIDs {
		if seen[id] {
			return errors.New("duplicate content identity")
		}
		seen[id] = true
		if s.Mass[i] <= 0 || s.Energy[i] <= 0 || s.Amp[i] <= 0 || s.Heat[i] < 0 {
			return errors.New("positive mass/energy/amplitude and non-negative heat required")
		}
	}
	for _, values := range [][]float32{s.Phase, s.Omega, s.Energy, s.Mass, s.Heat, s.Amp, s.Pos, s.Vel} {
		for _, value := range values {
			if math.IsNaN(float64(value)) || math.IsInf(float64(value), 0) {
				return errors.New("non-finite tensor value")
			}
		}
	}
	return nil
}
