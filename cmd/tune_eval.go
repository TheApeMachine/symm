package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/theapemachine/symm/config"
)

type evalTrialOptions struct {
	replayFile string
	tunables   config.Tunables
	stress     bool
}

type trialScores struct {
	trainScore    float64
	holdoutScores []float64
	selection     float64
}

/*
trialSelectionScore ranks candidates by minimum holdout score when holdouts exist,
otherwise by train score. This avoids picking configs that only fit the training
replay.
*/
func trialSelectionScore(trainScore float64, holdoutScores []float64) float64 {
	if len(holdoutScores) == 0 {
		return trainScore
	}

	selection := holdoutScores[0]

	for _, score := range holdoutScores[1:] {
		if score < selection {
			selection = score
		}
	}

	return selection
}

func scoreTrial(
	trainReplay string,
	holdoutReplays []string,
	stressHoldout bool,
	candidate config.Tunables,
) (trialScores, error) {
	trainScore, err := runEvalTrial(evalTrialOptions{
		replayFile: trainReplay,
		tunables:   candidate,
		stress:     false,
	})

	if err != nil {
		return trialScores{}, err
	}

	holdoutScores := make([]float64, 0, len(holdoutReplays))

	for _, holdoutReplay := range holdoutReplays {
		holdoutScore, holdoutErr := runEvalTrial(evalTrialOptions{
			replayFile: holdoutReplay,
			tunables:   candidate,
			stress:     stressHoldout,
		})

		if holdoutErr != nil {
			return trialScores{}, holdoutErr
		}

		holdoutScores = append(holdoutScores, holdoutScore)
	}

	return trialScores{
		trainScore:    trainScore,
		holdoutScores: holdoutScores,
		selection:     trialSelectionScore(trainScore, holdoutScores),
	}, nil
}

func runEvalTrial(options evalTrialOptions) (float64, error) {
	tempFile, err := os.CreateTemp("", "symm-tune-*.json")

	if err != nil {
		return 0, err
	}

	tempPath := tempFile.Name()
	_ = tempFile.Close()
	defer os.Remove(tempPath)

	trialConfig := config.NewConfig()
	options.tunables.Apply(trialConfig)

	if err := config.SaveTunablesFile(tempPath, trialConfig); err != nil {
		return 0, err
	}

	executable, err := os.Executable()

	if err != nil {
		return 0, err
	}

	env := map[string]string{
		"SYMM_HEADLESS":    "1",
		"SYMM_REPLAY_FILE": options.replayFile,
		"SYMM_CONFIG_FILE": tempPath,
		"SYMM_LOG_STDOUT":  "0",
	}

	if options.stress {
		env["SYMM_EXECUTION_STRESS"] = "1"
	}

	output, err := execEval(executable, env)

	if err != nil {
		return 0, err
	}

	var result struct {
		ScoreEUR float64 `json:"score_eur"`
	}

	if err := json.Unmarshal(output, &result); err != nil {
		return 0, fmt.Errorf("decode eval score: %w", err)
	}

	return result.ScoreEUR, nil
}
