package cmd

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/symm/system"
)

func TestInitConfig(t *testing.T) {
	Convey("Given a runtime buffer specified in the selected configuration file", t, func() {
		settings, previousConfig, previousFile := viper.AllSettings(), system.Cfg, cfgFile
		flag := rootCmd.PersistentFlags().Lookup("config")
		previousChanged := flag.Changed
		Reset(func() {
			viper.Reset()
			So(viper.MergeConfigMap(settings), ShouldBeNil)
			system.Cfg, cfgFile, flag.Changed = previousConfig, previousFile, previousChanged
		})
		cfgFile = filepath.Join(t.TempDir(), "config.yml")
		So(os.WriteFile(cfgFile, []byte("runtime:\n  workspace:\n    buffer: 64\n"), 0600), ShouldBeNil)
		flag.Changed = true

		Convey("Startup should construct typed configuration from that loaded file", func() {
			initConfig()
			So(system.Cfg.Runtime.Workspace.Buffer, ShouldEqual, 64)
			So(system.Cfg.Runtime.Workspace.Mask, ShouldEqual, 63)
		})
	})
}

func TestStartPprof(t *testing.T) {
	Convey("Given the default HTTP mux used by the profiling server", t, func() {
		request := httptest.NewRequest(http.MethodGet, "/debug/pprof/", nil)
		response := httptest.NewRecorder()

		http.DefaultServeMux.ServeHTTP(response, request)

		Convey("It should expose the registered profiling index", func() {
			So(response.Code, ShouldEqual, http.StatusOK)
		})
	})
}
