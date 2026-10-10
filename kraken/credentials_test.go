package kraken

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
)

func TestUseCredentials(t *testing.T) {
	// Placeholder variable names and values only; no real credentials.
	viper.Set("kraken.credentials.collector.key_env", "SYMM_TEST_COLLECTOR_KEY")
	viper.Set("kraken.credentials.collector.secret_env", "SYMM_TEST_COLLECTOR_SECRET")
	viper.Set("kraken.credentials.main.key_env", "SYMM_TEST_MAIN_KEY")
	viper.Set("kraken.credentials.main.secret_env", "SYMM_TEST_MAIN_SECRET")

	defer processCredentials.Store(nil)

	Convey("Given only the main role's variables set", t, func() {
		t.Setenv("SYMM_TEST_MAIN_KEY", "main-key")
		t.Setenv("SYMM_TEST_MAIN_SECRET", "main-secret")
		processCredentials.Store(nil)

		Convey("the main role resolves its own pair", func() {
			So(UseCredentials(RoleMain), ShouldBeNil)
			keys, err := processKeys()
			So(err, ShouldBeNil)
			So(keys.key, ShouldEqual, "main-key")
			So(keys.secret, ShouldEqual, "main-secret")
		})

		Convey("the collector role is a startup error naming its variable, not a fall back to main", func() {
			err := UseCredentials(RoleCollector)
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "SYMM_TEST_COLLECTOR_KEY")
			_, err = processKeys()
			So(err, ShouldNotBeNil)
		})

		Convey("a role without configured variable names is an error", func() {
			So(UseCredentials("unknown"), ShouldNotBeNil)
		})

		Convey("signing without a selected role is an error", func() {
			_, err := NewAuthenticatedREST()
			So(err, ShouldNotBeNil)
		})
	})
}
