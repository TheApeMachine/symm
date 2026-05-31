package cmd

import "runtime"

func resolveTuneEvalWorkers(workers int) int {
	return resolveTuneEvalWorkersFor(workers, runtime.NumCPU())
}

func resolveTuneEvalWorkersFor(workers int, cpus int) int {
	if cpus <= 0 {
		return 1
	}

	if workers <= 1 {
		return cpus
	}

	budget := cpus / workers

	if budget < 1 {
		return 1
	}

	return budget
}

// resolveTuneEngineWorkers maps per-subprocess CPU budget to SYMM_ENGINE_WORKERS.
func resolveTuneEngineWorkers(evalCPUBudget int) int {
	if evalCPUBudget <= 0 {
		return 1
	}

	return evalCPUBudget * 4
}
