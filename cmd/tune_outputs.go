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
	if document == nil {
		return tunePersistedOutputs{}, fmt.Errorf("best candidate has no perspective document")
	}

	if err := writeTuneLeaderFiles(tunablesPath, perspectivesPath, cfg, document); err != nil {
		return tunePersistedOutputs{}, err
	}

	if err := installTuneLeaderFiles(cfg, document); err != nil {
		return tunePersistedOutputs{}, err
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
	installTunablesAbs, err := filepath.Abs(config.DefaultTunedInstallPath())

	if err != nil {
		return tunePersistedOutputs{}, err
	}

	installPerspectivesAbs, err := filepath.Abs(config.DefaultPerspectiveInstallPath())

	if err != nil {
		return tunePersistedOutputs{}, err
	}

	reporter.Summary(fmt.Sprintf("Installed desk tunables → %s", installTunablesAbs))
	reporter.Summary(fmt.Sprintf("Installed playbook trees → %s", installPerspectivesAbs))
	reporter.Summary("Next boot loads config/tuned.json and config/perspectives.yaml when present (otherwise Go builtin playbooks)")

	return tunePersistedOutputs{
		tunablesPath:     tunablesAbs,
		perspectivesPath: perspectivesAbs,
	}, nil
}

func writeTuneLeaderFiles(
	tunablesPath string,
	perspectivesPath string,
	cfg *config.Config,
	document *perspectives.Document,
) error {
	if document == nil {
		return fmt.Errorf("best candidate has no perspective document")
	}

	if err := config.SaveTunablesFile(tunablesPath, cfg); err != nil {
		return fmt.Errorf("save tunables %q: %w", tunablesPath, err)
	}

	if err := perspectives.SaveDocumentFile(perspectivesPath, *document); err != nil {
		return fmt.Errorf("save perspectives %q: %w", perspectivesPath, err)
	}

	return nil
}

func installTuneLeaderFiles(
	cfg *config.Config,
	document *perspectives.Document,
) error {
	return writeTuneLeaderFiles(
		config.DefaultTunedInstallPath(),
		config.DefaultPerspectiveInstallPath(),
		cfg,
		document,
	)
}

func loadBaselineDocument(path string) (perspectives.Document, error) {
	payload, err := os.ReadFile(path)

	if err != nil {
		return perspectives.Document{}, err
	}

	return perspectives.DecodeDocument(payload)
}

func snapshotTuneCandidate(document perspectives.Document, tunables config.Tunables) tuneCandidate {
	cloned := perspectives.CloneDocument(document)

	return tuneCandidate{
		tunables:     config.CloneTunables(tunables),
		perspectives: &cloned,
	}
}

func saveTuneLeader(
	_ *TuneReporter,
	options tuneRunOptions,
	candidate tuneCandidate,
) error {
	if candidate.perspectives == nil {
		return fmt.Errorf("leader candidate has no perspective document")
	}

	trialConfig := config.NewConfig()
	candidate.tunables.Apply(trialConfig)

	if err := writeTuneLeaderFiles(
		options.output,
		options.perspectiveOutput,
		trialConfig,
		candidate.perspectives,
	); err != nil {
		return err
	}

	return installTuneLeaderFiles(trialConfig, candidate.perspectives)
}
