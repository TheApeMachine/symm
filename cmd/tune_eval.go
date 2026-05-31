package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/market/perspectives"
	"github.com/theapemachine/symm/trader/economics"
)

type evalTrialOptions struct {
	replayFile    string
	tunables      config.Tunables
	perspectives  *perspectives.Document
	stress        bool
	perturb       bool
	perturbSeed   int64
	evalCPUBudget int
}

type tuneCandidate struct {
	tunables     config.Tunables
	perspectives *perspectives.Document
}

type trialScores struct {
	trainScore    float64
	holdoutScores []float64
	selection     float64
	searchReward  float64
	gap           float64
	eligible      bool
	rejectReason  string
}

type evalTrialResult struct {
	ScoreEUR    float64
	FitnessEUR  float64
	Performance economics.PerformanceSummary
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

func trialSelectionIndex(holdoutScores []float64) int {
	if len(holdoutScores) == 0 {
		return -1
	}

	selection := 0

	for index, score := range holdoutScores[1:] {
		if score < holdoutScores[selection] {
			selection = index + 1
		}
	}

	return selection
}

func trialSelectionPerformance(
	trainResult evalTrialResult,
	holdoutResults []evalTrialResult,
	holdoutScores []float64,
) economics.PerformanceSummary {
	selection := trialSelectionIndex(holdoutScores)

	if selection < 0 {
		return trainResult.Performance
	}

	if selection >= len(holdoutResults) {
		return economics.PerformanceSummary{}
	}

	return holdoutResults[selection].Performance
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
const tuneScoreEpsilon = 1e-6

/*
betterTuneCandidate reports whether trial should replace the incumbent best.
Holdout fitness (selection) is primary; train fitness breaks ties so flat holdout
runs do not celebrate worse train losses as "new best".
*/
func betterTuneCandidate(
	selection float64,
	trainScore float64,
	bestSelection float64,
	bestTrainScore float64,
) bool {
	if selection > bestSelection+tuneScoreEpsilon {
		return true
	}

	if selection < bestSelection-tuneScoreEpsilon {
		return false
	}

	return trainScore > bestTrainScore+tuneScoreEpsilon
}

const (
	tuneRejectNoProfit = "rejected (no realized profitable trade on selection split)"
	tuneRejectOverfit  = "rejected (train >> holdout, overfit)"
)

func trialEligible(
	trainScore float64,
	holdoutScores []float64,
	maxGap float64,
	selectionPerformance economics.PerformanceSummary,
) (bool, string) {
	if selectionPerformance.ProfitableTrades == 0 {
		return false, tuneRejectNoProfit
	}

	if len(holdoutScores) == 0 {
		return true, ""
	}

	if maxGap < 0 {
		return true, ""
	}

	if trialTrainHoldoutGap(trainScore, holdoutScores) > maxGap {
		return false, tuneRejectOverfit
	}

	return true, ""
}

func trialSearchReward(
	selection float64,
	rejectReason string,
	selectionPerformance economics.PerformanceSummary,
) float64 {
	if rejectReason != tuneRejectNoProfit {
		return selection
	}

	if selectionPerformance.ClosedTrades == 0 {
		return selection - 10
	}

	return selection
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
	perturbTrain bool,
	perturbSeed int64,
	evalCPUBudget int,
	maxTrainHoldoutGap float64,
	candidate tuneCandidate,
) (trialScores, error) {
	trainResult, err := runEvalTrial(evalTrialOptions{
		replayFile:    trainReplay,
		tunables:      candidate.tunables,
		perspectives:  candidate.perspectives,
		stress:        false,
		perturb:       perturbTrain,
		perturbSeed:   perturbSeed,
		evalCPUBudget: evalCPUBudget,
	})

	if err != nil {
		return trialScores{}, err
	}

	holdoutResults := make([]evalTrialResult, 0, len(holdoutReplays))
	holdoutScores := make([]float64, 0, len(holdoutReplays))

	for _, holdoutReplay := range holdoutReplays {
		holdoutResult, holdoutErr := runEvalTrial(evalTrialOptions{
			replayFile:    holdoutReplay,
			tunables:      candidate.tunables,
			perspectives:  candidate.perspectives,
			stress:        stressHoldout,
			evalCPUBudget: evalCPUBudget,
		})

		if holdoutErr != nil {
			return trialScores{}, holdoutErr
		}

		holdoutResults = append(holdoutResults, holdoutResult)
		holdoutScores = append(holdoutScores, holdoutResult.FitnessEUR)
	}

	gap := trialTrainHoldoutGap(trainResult.FitnessEUR, holdoutScores)
	selectionPerformance := trialSelectionPerformance(trainResult, holdoutResults, holdoutScores)
	selection := trialSelectionScore(trainResult.FitnessEUR, holdoutScores)
	eligible, rejectReason := trialEligible(
		trainResult.FitnessEUR,
		holdoutScores,
		maxTrainHoldoutGap,
		selectionPerformance,
	)

	return trialScores{
		trainScore:    trainResult.FitnessEUR,
		holdoutScores: holdoutScores,
		selection:     selection,
		searchReward:  trialSearchReward(selection, rejectReason, selectionPerformance),
		gap:           gap,
		eligible:      eligible,
		rejectReason:  rejectReason,
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
	perspectivePath := ""

	trialConfig := config.NewConfig()
	options.tunables.Apply(trialConfig)

	if err := config.SaveTunablesFile(tempPath, trialConfig); err != nil {
		return evalTrialResult{}, err
	}

	if options.perspectives != nil {
		perspectiveFile, fileErr := os.CreateTemp("", "symm-perspectives-*.yaml")

		if fileErr != nil {
			return evalTrialResult{}, fileErr
		}

		perspectivePath = perspectiveFile.Name()
		_ = perspectiveFile.Close()
		defer os.Remove(perspectivePath)

		if err := perspectives.SaveDocumentFile(perspectivePath, *options.perspectives); err != nil {
			return evalTrialResult{}, err
		}
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
		"SYMM_LOG_FILE":    "0",
	}

	if options.evalCPUBudget > 0 {
		env["GOMAXPROCS"] = fmt.Sprintf("%d", options.evalCPUBudget)
		env[engineWorkersEnv] = fmt.Sprintf("%d", resolveTuneEngineWorkers(options.evalCPUBudget))
	}

	if perspectivePath != "" {
		env["SYMM_PERSPECTIVES_FILE"] = perspectivePath
	}

	if options.stress {
		env["SYMM_EXECUTION_STRESS"] = "1"
	}

	if options.perturb {
		env["SYMM_REPLAY_PERTURB"] = "1"

		if options.perturbSeed != 0 {
			env["SYMM_REPLAY_PERTURB_SEED"] = fmt.Sprintf("%d", options.perturbSeed)
		}
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
		Performance economics.PerformanceSummary `json:"trade_performance"`
	}

	if err := json.Unmarshal(output, &result); err != nil {
		return evalTrialResult{}, fmt.Errorf("decode eval score: %w", err)
	}

	fitnessEUR := TuneFitness(result.ScoreEUR, result.Regret.MissedForwardEUR, result.Performance)

	return evalTrialResult{
		ScoreEUR:    result.ScoreEUR,
		FitnessEUR:  fitnessEUR,
		Performance: result.Performance,
	}, nil
}
