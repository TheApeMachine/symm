package config_test

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/config"
)

func TestReplayPerturbEnvOverride(t *testing.T) {
	Convey("Given replay perturbation env vars", t, func() {
		t.Setenv("SYMM_REPLAY_PERTURB", "1")
		t.Setenv("SYMM_REPLAY_PERTURB_SEED", "42")
		t.Setenv("SYMM_REPLAY_QTY_JITTER_SIGMA", "0.08")
		t.Setenv("SYMM_REPLAY_TS_JITTER", "75ms")
		cfg := &config.Config{}

		Convey("It should enable replay perturbation settings", func() {
			So(config.ApplyEnvironment(cfg), ShouldBeNil)
			So(cfg.ReplayPerturbEnabled, ShouldBeTrue)
			So(cfg.ReplayPerturbSeed, ShouldEqual, 42)
			So(cfg.ReplayQtyJitterSigma, ShouldEqual, 0.08)
			So(cfg.ReplayTimestampJitter, ShouldEqual, 75*time.Millisecond)
		})
	})
}

func TestReplayPerturbInvalidSigma(t *testing.T) {
	Convey("Given a negative qty jitter sigma", t, func() {
		t.Setenv("SYMM_REPLAY_QTY_JITTER_SIGMA", "-1")
		cfg := &config.Config{}

		Convey("It should fail closed", func() {
			err := config.ApplyEnvironment(cfg)

			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "SYMM_REPLAY_QTY_JITTER_SIGMA")
		})
	})
}
