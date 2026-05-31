package cmd

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestMergeEvalEnv(t *testing.T) {
	Convey("Given inherited variables and eval overrides", t, func() {
		env := mergeEvalEnv(
			[]string{"GOMAXPROCS=16", "PATH=/bin", "SYMM_LOG_FILE=1"},
			map[string]string{
				"GOMAXPROCS":          "2",
				engineWorkersEnv:      "8",
				"SYMM_LOG_FILE":       "0",
				"SYMM_REPLAY_FILE":    "runs/capture.jsonl",
				"SYMM_LOG_STDOUT":     "0",
				"SYMM_CONFIG_FILE":    "/tmp/tuned.json",
				"SYMM_HEADLESS":       "1",
				"SYMM_REPLAY_PERTURB": "1",
			},
		)

		Convey("It should replace inherited values deterministically", func() {
			So(env, ShouldResemble, []string{
				"PATH=/bin",
				"GOMAXPROCS=2",
				"SYMM_CONFIG_FILE=/tmp/tuned.json",
				"SYMM_ENGINE_WORKERS=8",
				"SYMM_HEADLESS=1",
				"SYMM_LOG_FILE=0",
				"SYMM_LOG_STDOUT=0",
				"SYMM_REPLAY_FILE=runs/capture.jsonl",
				"SYMM_REPLAY_PERTURB=1",
			})
		})
	})
}

func BenchmarkMergeEvalEnv(b *testing.B) {
	base := []string{
		"GOMAXPROCS=16",
		"PATH=/bin",
		"SYMM_LOG_FILE=1",
		"SYMM_LOG_STDOUT=1",
	}
	overrides := map[string]string{
		"GOMAXPROCS":       "2",
		engineWorkersEnv:   "8",
		"SYMM_LOG_FILE":    "0",
		"SYMM_REPLAY_FILE": "runs/capture.jsonl",
	}

	for b.Loop() {
		_ = mergeEvalEnv(base, overrides)
	}
}
