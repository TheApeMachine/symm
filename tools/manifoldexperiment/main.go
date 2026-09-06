package main

import (
	"context"
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
	"time"

	"github.com/spf13/viper"
	"github.com/theapemachine/symm/logic/manifold"
)

/* ExperimentCommand owns the reproducible input/output and compute budget. */
type ExperimentCommand struct {
	Input, Output, Symbols      string
	Grid, Depth, Steps, Repeats int
	Alpha                       float64
	MaxAge                      time.Duration
	SecondsPerUnit              float64
	PriceUnit                   float64
}

type ExperimentReport struct {
	InputSHA256              string
	PriceUnit                float64
	Runs                     int
	ExactReplayEquality      bool
	ReplayExercised          bool
	MaximumReadingDifference float64
	Comparison               *manifold.Comparison
}

func main() {
	command := &ExperimentCommand{}
	flag.StringVar(&command.Input, "input", "", "public-market JSONL capture export")
	flag.StringVar(&command.Output, "output", "", "research report JSON path")
	flag.StringVar(&command.Symbols, "symbols", "", "comma-separated experimental universe")
	flag.IntVar(&command.Grid, "grid", 32, "spatial cells per axis (research resolution)")
	flag.IntVar(&command.Depth, "depth", 10, "captured subscription depth")
	flag.IntVar(&command.Steps, "steps", 0, "required maximum physics advances per variant")
	flag.IntVar(&command.Repeats, "repeats", 2, "identical-input repeat count")
	flag.Float64Var(&command.Alpha, "alpha", 0.01, "family-wise Fisher approximation error budget")
	flag.DurationVar(&command.MaxAge, "max-pair-age", time.Minute, "explicit maximum pair-evidence age")
	flag.Float64Var(&command.SecondsPerUnit, "seconds-per-unit", 0.000001, "seconds per solver time unit")
	flag.Float64Var(&command.PriceUnit, "price-unit", 1, "denomination sensitivity multiplier applied after checksum validation")
	flag.Parse()

	if err := command.Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func (command *ExperimentCommand) Run() (err error) {
	if command.Input == "" || command.Output == "" || command.Symbols == "" || command.Steps <= 0 || command.Repeats < 1 || command.Grid < 2 || command.Depth <= 0 || !(command.PriceUnit > 0) || !(command.SecondsPerUnit > 0) || !(command.Alpha > 0 && command.Alpha < 1) || command.MaxAge <= 0 {
		return fmt.Errorf("input, output, symbols and positive steps/repeats/grid/depth are required")
	}
	input, err := os.Open(command.Input)

	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, input.Close()) }()
	digest := sha256.New()

	if _, err := io.Copy(digest, input); err != nil {
		return err
	}
	report := &ExperimentReport{InputSHA256: hex.EncodeToString(digest.Sum(nil)), Runs: command.Repeats, PriceUnit: command.PriceUnit, ExactReplayEquality: true}
	viper.Set("market.l3_depth", command.Depth)
	symbols := strings.Split(command.Symbols, ",")
	seen := make(map[string]bool)
	for _, symbol := range symbols {
		if symbol == "" || seen[symbol] {
			return fmt.Errorf("symbols must be nonempty and unique")
		}
		seen[symbol] = true
	}

	if len(symbols) < 2 {
		return fmt.Errorf("geometry requires at least two symbols")
	}
	// Every supported shortest path has at most n-1 edges, each <= sqrt(2).
	// This graph-derived bound supplies a fixed length conversion for this universe.
	extent := 1 - 1/float64(command.Grid)
	config := manifold.ProjectionConfig{Extent: [3]float64{extent, extent, extent}, GeometryUnit: extent / (2 * math.Sqrt2 * float64(max(1, len(symbols)-1))), DepthUnit: extent / float64(2*command.Depth), MassUnit: 1, SecondsPerUnit: command.SecondsPerUnit, Temperature: 0}
	for repeat := 0; repeat < command.Repeats; repeat++ {
		if _, err := input.Seek(0, io.SeekStart); err != nil {
			return err
		}
		comparison, err := command.compare(input, symbols, config)

		if err != nil {
			return err
		}

		if repeat == 0 {
			report.Comparison = comparison
			continue
		}
		report.ExactReplayEquality = report.ExactReplayEquality && reflect.DeepEqual(report.Comparison.Runs, comparison.Runs)
		for index, run := range comparison.Runs {
			original := report.Comparison.Runs[index]
			for step, sample := range run.Trajectory {
				if step >= len(original.Trajectory) {
					report.ExactReplayEquality = false
					break
				}
				left, right := sample.Reading, original.Trajectory[step].Reading
				for _, delta := range []float64{left.Divergence - right.Divergence, left.GuidanceSpeed - right.GuidanceSpeed, left.CoherenceMag2 - right.CoherenceMag2, left.PressureGradNorm - right.PressureGradNorm, left.ViscosityProxy - right.ViscosityProxy, left.KuramotoR - right.KuramotoR} {
					report.MaximumReadingDifference = math.Max(report.MaximumReadingDifference, math.Abs(delta))
				}
			}
		}
	}
	report.ReplayExercised = report.Comparison.Steps > 0 && command.Repeats > 1
	report.ExactReplayEquality = report.ExactReplayEquality && report.ReplayExercised
	output, err := os.Create(command.Output)

	if err != nil {
		return err
	}
	encoder := json.NewEncoder(output)
	encoder.SetIndent("", "  ")

	if err := encoder.Encode(report); err != nil {
		return errors.Join(err, output.Close())
	}

	if err := output.Close(); err != nil {
		return err
	}
	fmt.Printf("Compared %d shared advances across baseline/A/B; exact replay equality=%t; report=%s\n", report.Comparison.Steps, report.ExactReplayEquality, command.Output)
	fmt.Printf("input failure=%q; waiting reasons=%v\n", report.Comparison.InputFailure, report.Comparison.Waiting)
	for _, run := range report.Comparison.Runs {
		fmt.Printf("%s: steps=%d failure=%q\n", run.Name, run.Steps, run.Failure)
	}
	return nil
}

func (command *ExperimentCommand) compare(input io.Reader, symbols []string, config manifold.ProjectionConfig) (*manifold.Comparison, error) {
	comparison, err := manifold.NewComparison(context.Background(), symbols, command.Grid, config, command.Alpha, command.MaxAge)

	if err != nil {
		return nil, err
	}
	defer comparison.Close()
	comparison.Tape.PriceUnit = command.PriceUnit
	decoder := json.NewDecoder(input)
	for comparison.Steps < command.Steps {
		var frame manifold.MarketFrame
		err := decoder.Decode(&frame)

		if err == io.EOF {
			break
		}

		if err != nil {
			return nil, err
		}

		if err := comparison.Step(frame); err != nil {
			break
		} // retained in InputFailure; this is a failed experiment, not omitted input
	}
	comparison.Close()
	// Runtime pointers and projector maps are not part of replay equality.
	for _, run := range comparison.Runs {
		run.Physics, run.Baseline, run.Candidate = nil, nil, nil
	}
	return comparison, nil
}
