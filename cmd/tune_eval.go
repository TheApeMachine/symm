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
	gap           float64
	eligible      bool
}

type evalTrialResult struct {
	ScoreEUR   float64
	FitnessEUR float64
}

/*
trialSelectionScore ranks candidates by minimum holdout fitness when holdouts exist,
otherwise by train fitness. This avoids picking configs that only fit the training
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

/*
trialTrainHoldoutGap is train minus minimum holdout score. Large gaps indicate
memorization on the training replay rather than generalization.
*/
func trialTrainHoldoutGap(trainScore float64, holdoutScores []float64) float64 {
	if len(holdoutScores) == 0 {
		return 0
	}

	return trainScore - trialSelectionScore(trainScore, holdoutScores)
}

/*
trialEligible rejects candidates whose holdout score lags train by more than
maxGap EUR when holdouts exist.
*/
func trialEligible(trainScore float64, holdoutScores []float64, maxGap float64) bool {
	if len(holdoutScores) == 0 {
		return true
	}

	if maxGap < 0 {
		return true
	}

	return trialTrainHoldoutGap(trainScore, holdoutScores) <= maxGap
}

/*
resolveMaxTrainHoldoutGap maps CLI input to an EUR gap ceiling. Zero uses 3% of
WalletEUR when walletEUR > 0; zero wallet disables overfit rejection. Negative
requested values disable overfit rejection.
*/
func resolveMaxTrainHoldoutGap(requested float64, walletEUR float64) float64 {
	if requested < 0 {
		return -1
	}

	if requested > 0 {
		return requested
	}

	if walletEUR <= 0 {
		return -1
	}

	return walletEUR * 0.03
}

func scoreTrial(
	trainReplay string,
	holdoutReplays []string,
	stressHoldout bool,
	maxTrainHoldoutGap float64,
	candidate config.Tunables,
) (trialScores, error) {
	trainResult, err := runEvalTrial(evalTrialOptions{
		replayFile: trainReplay,
		tunables:   candidate,
		stress:     false,
	})

	if err != nil {
		return trialScores{}, err
	}

	holdoutScores := make([]float64, 0, len(holdoutReplays))

	for _, holdoutReplay := range holdoutReplays {
		holdoutResult, holdoutErr := runEvalTrial(evalTrialOptions{
			replayFile: holdoutReplay,
			tunables:   candidate,
			stress:     stressHoldout,
		})

		if holdoutErr != nil {
			return trialScores{}, holdoutErr
		}

		holdoutScores = append(holdoutScores, holdoutResult.FitnessEUR)
	}

	gap := trialTrainHoldoutGap(trainResult.FitnessEUR, holdoutScores)

	return trialScores{
		trainScore:    trainResult.FitnessEUR,
		holdoutScores: holdoutScores,
		selection:     trialSelectionScore(trainResult.FitnessEUR, holdoutScores),
		gap:           gap,
		eligible:      trialEligible(trainResult.FitnessEUR, holdoutScores, maxTrainHoldoutGap),
	}, nil
}

func runEvalTrial(options evalTrialOptions) (evalTrialResult, error) {
	tempFile, err := os.CreateTemp("", "symm-tune-*.json")

	if err != nil {
		return evalTrialResult{}, err
	}

	tempPath := tempFile.Name()
	_ = tempFile.Close()
	defer os.Remove(tempPath)

	trialConfig := config.NewConfig()
	options.tunables.Apply(trialConfig)

	if err := config.SaveTunablesFile(tempPath, trialConfig); err != nil {
		return evalTrialResult{}, err
	}

	executable, err := os.Executable()

	if err != nil {
		return evalTrialResult{}, err
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
		return evalTrialResult{}, err
	}

	var result struct {
		ScoreEUR   float64 `json:"score_eur"`
		FitnessEUR float64 `json:"fitness_eur"`
		Regret     struct {
			MissedForwardEUR float64 `json:"missed_forward_eur"`
		} `json:"gate_reject_regret"`
	}

	if err := json.Unmarshal(output, &result); err != nil {
		return evalTrialResult{}, fmt.Errorf("decode eval score: %w", err)
	}

	fitnessEUR := TuneFitness(result.ScoreEUR, result.Regret.MissedForwardEUR)

	return evalTrialResult{
		ScoreEUR:   result.ScoreEUR,
		FitnessEUR: fitnessEUR,
	}, nil
}
