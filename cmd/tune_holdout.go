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
	walkForwardFolds int,
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

	if walkForwardFolds > 1 {
		return resolveWalkForwardReplayPaths(replayFile, walkForwardFolds)
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

func resolveWalkForwardReplayPaths(replayFile string, folds int) (tuneReplayPaths, error) {
	trainPath, _, ok, err := replay.SplitHoldout(
		replayFile,
		replay.DefaultHoldoutFraction(),
		0,
	)

	if err != nil {
		return tuneReplayPaths{}, err
	}

	if !ok {
		return tuneReplayPaths{trainPath: replayFile}, nil
	}

	holdoutPaths, holdoutCleanup, err := replay.WalkForwardHoldouts(
		replayFile,
		folds,
		replay.DefaultHoldoutFraction(),
	)

	if err != nil {
		replay.RemoveSplitFiles(trainPath)

		return tuneReplayPaths{}, err
	}

	if len(holdoutPaths) == 0 {
		_, holdoutPath, ok, splitErr := replay.SplitHoldout(
			replayFile,
			replay.DefaultHoldoutFraction(),
			0,
		)

		if splitErr != nil {
			replay.RemoveSplitFiles(trainPath)

			return tuneReplayPaths{}, splitErr
		}

		if !ok {
			return tuneReplayPaths{trainPath: trainPath, cleanup: func() {
				replay.RemoveSplitFiles(trainPath)
			}}, nil
		}

		return tuneReplayPaths{
			trainPath:    trainPath,
			holdoutPaths: []string{holdoutPath},
			cleanup: func() {
				replay.RemoveSplitFiles(trainPath, holdoutPath)
			},
		}, nil
	}

	return tuneReplayPaths{
		trainPath:    trainPath,
		holdoutPaths: holdoutPaths,
		cleanup: func() {
			replay.RemoveSplitFiles(trainPath)
			holdoutCleanup()
		},
	}, nil
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
