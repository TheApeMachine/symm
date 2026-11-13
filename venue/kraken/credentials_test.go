package kraken

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
)

func TestUseCredentials(t *testing.T) {
	viper.Set("kraken.credentials.collector.key_env", "SYMM_TEST_COLLECTOR_KEY")
	viper.Set("kraken.credentials.collector.secret_env", "SYMM_TEST_COLLECTOR_SECRET")
	viper.Set("kraken.credentials.main.key_env", "SYMM_TEST_MAIN_KEY")
	viper.Set("kraken.credentials.main.secret_env", "SYMM_TEST_MAIN_SECRET")
	defer processCredentials.Store(nil)

	Convey("Setup", t, func() {
		t.Setenv("SYMM_TEST_MAIN_KEY", "main-key")
		t.Setenv("SYMM_TEST_MAIN_SECRET", "main-secret")
		processCredentials.Store(nil)

		Convey("When UseCredentials selects the main role", func() {
			err := UseCredentials(RoleMain)
			keys, keyErr := processKeys()

			Convey("the main key pair resolves", func() {
				So(err, ShouldBeNil)
				So(keyErr, ShouldBeNil)
				So(keys.key, ShouldEqual, "main-key")
				So(keys.secret, ShouldEqual, "main-secret")
			})
		})

		Convey("When the collector env is unset", func() {
			err := UseCredentials(RoleCollector)
			_, keyErr := processKeys()

			Convey("the error names the missing variable", func() {
				So(err, ShouldNotBeNil)
				So(err.Error(), ShouldContainSubstring, "SYMM_TEST_COLLECTOR_KEY")
				So(keyErr, ShouldNotBeNil)
			})
		})

		Convey("When the role is unknown", func() {
			err := UseCredentials("unknown")

			Convey("configuration is rejected", func() {
				So(err, ShouldNotBeNil)
			})
		})

		Convey("When NewAuthenticatedREST runs without a selected role", func() {
			_, err := NewAuthenticatedREST()

			Convey("REST construction fails", func() {
				So(err, ShouldNotBeNil)
			})
		})
	})
}

func BenchmarkUseCredentials(b *testing.B) {
	viper.Set("kraken.credentials.collector.key_env", "SYMM_BENCH_COLLECTOR_KEY")
	viper.Set("kraken.credentials.collector.secret_env", "SYMM_BENCH_COLLECTOR_SECRET")
	viper.Set("kraken.credentials.main.key_env", "SYMM_BENCH_MAIN_KEY")
	viper.Set("kraken.credentials.main.secret_env", "SYMM_BENCH_MAIN_SECRET")
	b.Setenv("SYMM_BENCH_MAIN_KEY", "main-key")
	b.Setenv("SYMM_BENCH_MAIN_SECRET", "main-secret")

	b.ReportAllocs()

	for b.Loop() {
		processCredentials.Store(nil)
		_ = UseCredentials(RoleMain)
	}
}
