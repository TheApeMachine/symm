package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/market/perspectives"
)

type tunePersistedOutputs struct {
	tunablesPath     string
	perspectivesPath string
}

func persistTuneOutputs(
	reporter *TuneReporter,
	tunablesPath string,
	perspectivesPath string,
	cfg *config.Config,
	document *perspectives.Document,
) (tunePersistedOutputs, error) {
	if err := config.SaveTunablesFile(tunablesPath, cfg); err != nil {
		return tunePersistedOutputs{}, fmt.Errorf("save tunables %q: %w", tunablesPath, err)
	}

	if document == nil {
		return tunePersistedOutputs{}, fmt.Errorf("best candidate has no perspective document")
	}

	if err := perspectives.SaveDocumentFile(perspectivesPath, *document); err != nil {
		return tunePersistedOutputs{}, fmt.Errorf("save perspectives %q: %w", perspectivesPath, err)
	}

	tunablesAbs, err := filepath.Abs(tunablesPath)

	if err != nil {
		return tunePersistedOutputs{}, err
	}

	perspectivesAbs, err := filepath.Abs(perspectivesPath)

	if err != nil {
		return tunePersistedOutputs{}, err
	}

	reporter.Summary(fmt.Sprintf("Saved desk tunables → %s", tunablesAbs))
	reporter.Summary(fmt.Sprintf("Saved playbook trees → %s", perspectivesAbs))
	reporter.Summary("Next boot loads runs/tuned.json and runs/perspectives.yaml (falls back to config/perspectives.yaml if missing)")

	return tunePersistedOutputs{
		tunablesPath:     tunablesAbs,
		perspectivesPath: perspectivesAbs,
	}, nil
}

func loadBaselineDocument(path string) (perspectives.Document, error) {
	payload, err := os.ReadFile(path)

	if err != nil {
		return perspectives.Document{}, err
	}

	return perspectives.DecodeDocument(payload)
}
