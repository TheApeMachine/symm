package compiler

import (
	"fmt"
	"os"
	"path/filepath"
	goruntime "runtime"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/cognition"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/transport"
	"github.com/theapemachine/symm/nomagique/types"
)

func NewSignals() types.Value[any, []float64] {
	dir := definitionsDir()
	grid := store.NewGrid[any, any]()
	errnie.Debug(fmt.Sprintf("[NewSignals] loading metric definitions from %s...", dir))

	if files, err := os.ReadDir(dir); err == nil {
		for _, file := range files {
			name := file.Name()
			if filepath.Ext(name) != ".json" ||
				name == "training.json" ||
				name == "logic.json" ||
				name == "execution.json" ||
				name == "system.json" {
				continue
			}

			builder, err := NewBuilder(filepath.Join(dir, name))
			if err != nil {
				continue
			}

			metricPipeline, err := builder.Compose()
			if err != nil {
				continue
			}

			metric := metricPipeline
			grid(transport.NewMessage[any, any](transport.REGISTER, nil, types.Value[any, any](metric)))
			errnie.Debug(fmt.Sprintf("[NewSignals] registered metric %s on store.Grid", name))
		}
	}

	errnie.Debug("[NewSignals] all metric pipelines registered on store.Grid")

	return func(tick any) []float64 {
		if tick == nil {
			return nil
		}

		rawResults := grid(transport.NewMessage[any, any](transport.POKE, tick, nil))
		if len(rawResults) == 0 {
			return nil
		}

		readings := make([]float64, len(rawResults))
		for i, r := range rawResults {
			switch v := r.(type) {
			case float64:
				readings[i] = v
			case *float64:
				if v != nil {
					readings[i] = *v
				}
			case int:
				readings[i] = float64(v)
			case int64:
				readings[i] = float64(v)
			}
		}

		return readings
	}
}

func NewLogic() types.Value[[]float64, cognition.Evaluation] {
	dir := definitionsDir()
	path := filepath.Join(dir, "logic.json")
	errnie.Debug(fmt.Sprintf("[NewLogic] loading logic definition from %s...", path))

	builder, err := NewBuilder(path)
	if err != nil {
		path = filepath.Join(dir, "training.json")
		builder, err = NewBuilder(path)
	}

	if err != nil {
		return func([]float64) cognition.Evaluation { return nil }
	}

	pipeline, err := builder.Compose()
	if err != nil {
		return func([]float64) cognition.Evaluation { return nil }
	}

	errnie.Debug("[NewLogic] logic pipeline composed successfully")

	return func(readings []float64) cognition.Evaluation {
		if len(readings) == 0 {
			return nil
		}

		res := pipeline(readings)
		if eval, ok := res.(cognition.Evaluation); ok {
			return eval
		}

		return nil
	}
}

func NewExecution() types.Value[cognition.Evaluation, any] {
	dir := definitionsDir()
	path := filepath.Join(dir, "execution.json")
	errnie.Debug(fmt.Sprintf("[NewExecution] loading execution definition from %s...", path))

	builder, err := NewBuilder(path)
	if err != nil {
		return func(cognition.Evaluation) any { return nil }
	}

	pipeline, err := builder.Compose()
	if err != nil {
		return func(cognition.Evaluation) any { return nil }
	}

	errnie.Debug("[NewExecution] execution pipeline composed successfully")

	return func(eval cognition.Evaluation) any {
		if eval == nil {
			return nil
		}

		return pipeline(eval)
	}
}

func definitionsDir() string {
	candidates := []string{
		"signal/definitions",
		"../../signal/definitions",
		"../../../signal/definitions",
	}
	for _, c := range candidates {
		if fi, err := os.Stat(c); err == nil && fi.IsDir() {
			return c
		}
	}
	_, goFile, _, _ := goruntime.Caller(0)
	return filepath.Join(filepath.Dir(goFile), "..", "..", "signal", "definitions")
}
