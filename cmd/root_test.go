package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/theapemachine/symm/system"
	"github.com/theapemachine/symm/tests/replay"
	_ "gocloud.dev/blob/fileblob"
)

// TestExecute drives the production composition from original S3 capture objects.
// It is opt-in because the real normalizer and paper-account CLI are initialized.
func TestExecute(t *testing.T) {
	directory := os.Getenv("SYMM_REPLAY_CAPTURE_DIR")

	if directory == "" {
		t.Skip("set SYMM_REPLAY_CAPTURE_DIR to cached original S3 capture objects")
	}
	Convey("The application consumes the recorded multi-symbol websocket tape", t, func() {
		// Five minutes bounds test startup, recorded pacing and backlog drainage.
		ctx, cancel := context.WithTimeout(t.Context(), 5*time.Minute)
		defer cancel()
		viper.SetConfigFile("cfg/config.yml")
		So(viper.ReadInConfig(), ShouldBeNil)
		system.Cfg = system.NewConfig()
		endpoints := system.Cfg.WebSocket.Endpoints
		server := replay.NewServer(ctx, map[string]string{
			endpoints.Public: "public", endpoints.Private: "private",
			endpoints.Level3: "level3", endpoints.Futures: "futures",
		})
		defer func() {
			cancel()
			So(server.Close(), ShouldBeNil)
		}()
		for _, role := range []string{"public", "private", "level3", "futures"} {
			viper.Set("system.websocket.endpoints."+role, server.URL(role))
		}
		output := os.Getenv("SYMM_REPLAY_OUTPUT_DIR")

		if output == "" {
			output = t.TempDir()
		}
		So(os.MkdirAll(output, 0700), ShouldBeNil)
		entries, err := os.ReadDir(output)
		So(err, ShouldBeNil)
		So(entries, ShouldBeEmpty)
		viper.Set("storage.s3.bucket", "replay")
		// These are the production simulated accounts, never live execution.
		viper.Set("trading.model", "paper")
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		So(err, ShouldBeNil)
		address := listener.Addr().String()
		viper.Set("ui.addr", address)
		So(listener.Close(), ShouldBeNil)
		viper.Set("system.pprof.enabled", false)
		configuration := filepath.Join(output, "config.yml")
		So(viper.WriteConfigAs(configuration), ShouldBeNil)
		viper.SetConfigFile(configuration)
		system.Cfg = system.NewConfig()
		command := &cobra.Command{}
		command.SetContext(ctx)
		done := make(chan error, 1)
		go func() {
			done <- rootCmd.RunE(command, nil)
			cancel()
		}()
		observer := &replay.Observer{}
		observed := make(chan error, 1)
		go func() {
			observed <- observer.Read(ctx, "ws://"+address+"/ws", viper.GetDuration("ui.websocket.learning_interval"))
			cancel()
		}()
		tape := replay.Tape{Directory: directory}

		if through := os.Getenv("SYMM_REPLAY_THROUGH"); through != "" {
			tape.Through, err = time.Parse(time.RFC3339Nano, through)
			So(err, ShouldBeNil)
		}
		err = tape.Read(ctx, server.Deliver)
		audit := &replay.Audit{Directory: output}
		clock := time.NewTicker(viper.GetDuration("hindsight.capture.flush_interval"))
		defer clock.Stop()

		for err == nil {
			err = audit.Read()

			if err != nil || (audit.Captures == server.Frames && audit.Manifests > 0 && observer.Completed.Load() >= audit.Manifests) {
				break
			}

			select {
			case <-ctx.Done():
				err = ctx.Err()
			case <-clock.C:
			}
		}
		t.Logf("accepted=%d ingress=%d completed=%d learning_steps=%d decisions=%d markets=%d agents=%d backlog=%d",
			audit.Captures, audit.Manifests, observer.Completed.Load(), observer.Steps.Load(), observer.Decisions.Load(),
			observer.Markets.Load(), observer.Agents.Load(), observer.Backlog.Load())
		t.Logf("replay frames=%d derived=%d source=%s..%s elapsed=%s max_lateness=%s output=%s",
			server.Frames, server.Derived, server.First.Format(time.RFC3339Nano),
			server.Last.Format(time.RFC3339Nano), time.Since(server.Started), server.MaxLateness, output)
		cancel()
		runErr := <-done
		observerErr := <-observed
		report, reportErr := json.MarshalIndent(map[string]any{
			"sourceRun": server.Source, "sourceFirst": server.First, "sourceLast": server.Last,
			"sent": server.Frames, "derived": server.Derived, "accepted": audit.Captures,
			"ingress": audit.Manifests, "completed": observer.Completed.Load(),
			"learningSteps": observer.Steps.Load(), "decisions": observer.Decisions.Load(),
			"markets": observer.Markets.Load(), "agents": observer.Agents.Load(),
			"maxLateness": server.MaxLateness.String(), "configDigest": configDigest(),
			"codeCommit": buildCodeCommit(), "buildId": buildBuildID(),
		}, "", "  ")
		So(reportErr, ShouldBeNil)
		So(os.WriteFile(filepath.Join(output, "replay-report.json"), report, 0600), ShouldBeNil)

		if observerErr != nil && !errors.Is(observerErr, context.Canceled) && !errors.Is(observerErr, context.DeadlineExceeded) {
			t.Fatal(observerErr)
		}

		if runErr != nil && !errors.Is(runErr, context.Canceled) {
			t.Fatal(fmt.Errorf("production application: %w", runErr))
		}
		So(err, ShouldBeNil)
		So(audit.Captures, ShouldEqual, server.Frames)
		So(observer.Completed.Load(), ShouldEqual, audit.Manifests)
		So(observer.Steps.Load(), ShouldBeGreaterThan, 0)
		So(observer.Agents.Load(), ShouldEqual, system.Cfg.Learning.Traders)
	})
}

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
