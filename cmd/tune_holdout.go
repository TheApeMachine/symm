package cmd

import (
	"fmt"
	"strings"

	"github.com/theapemachine/symm/replay"
)

const defaultReplayFile = "runs/capture.jsonl"

type tuneReplayPaths struct {
	trainPath    string
	holdoutPaths []string
	cleanup      func()
}

func resolveTuneReplayPaths(
	replayFile string,
	holdoutPaths []string,
	autoHoldout bool,
) (tuneReplayPaths, error) {
	replayFile = strings.TrimSpace(replayFile)

	if replayFile == "" {
		replayFile = defaultReplayFile
	}

	paths := tuneReplayPaths{trainPath: replayFile}

	if len(holdoutPaths) > 0 {
		paths.holdoutPaths = append([]string(nil), holdoutPaths...)

		return paths, nil
	}

	if !autoHoldout {
		return paths, nil
	}

	trainPath, holdoutPath, ok, err := replay.SplitHoldout(
		replayFile,
		replay.DefaultHoldoutFraction(),
		0,
	)

	if err != nil {
		return tuneReplayPaths{}, err
	}

	if !ok {
		return paths, nil
	}

	paths.trainPath = trainPath
	paths.holdoutPaths = []string{holdoutPath}
	paths.cleanup = func() {
		replay.RemoveSplitFiles(trainPath, holdoutPath)
	}

	return paths, nil
}

func requireReplayFile(replayFile string) (string, error) {
	replayFile = strings.TrimSpace(replayFile)

	if replayFile == "" {
		replayFile = defaultReplayFile
	}

	return replayFile, nil
}

func tuneReplayMissingMessage(replayFile string) error {
	return fmt.Errorf("missing replay %s — run: make record", replayFile)
}
