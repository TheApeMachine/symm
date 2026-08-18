package driver

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/klauspost/compress/zstd"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/backtest"
	"github.com/theapemachine/symm/types"
	"github.com/theapemachine/symm/ui"
	"github.com/theapemachine/symm/utils"
)

/*
Command returns the `symm backtest` entrypoint: it serves the dashboard hub
with playback controls and replays captured sessions through the full stack.
*/
func Command() *cobra.Command {
	command := &cobra.Command{
		Use:   "backtest",
		Short: "Replay captured sessions with dashboard playback controls.",
		RunE: func(run *cobra.Command, arguments []string) error {
			if importPath, err := run.Flags().GetString("import"); importPath != "" || err != nil {
				if err != nil {
					return errnie.Error(errnie.Err(errnie.Internal, "backtest: get import flag", err))
				}

				return importCapture(importPath)
			}

			errnie.Apply(&errnie.Config{
				Level: viper.GetString("system.log.level"),
			})

			ctx := run.Context()
			uiChannel := make(chan []byte, 1024)
			manifoldChannel := make(chan types.FluidFrame, 1024)

			dataPath := utils.ResolveDataPath()
			store, err := backtest.NewStore(filepath.Join(dataPath, "symm.sqlite"))

			if err != nil {
				return errnie.Error(errnie.Err(errnie.Internal, "backtest: open store", err))
			}

			defer store.Close()

			hub := ui.NewHub(ctx, nil, nil, nil, uiChannel, manifoldChannel)
			replay := NewDriver(ctx, store, hub, uiChannel,
				func(state State) {
					utils.Publish(uiChannel, datura.NewMap("backtest", state))
				},
			)

			hub.SetPlayback(replay, func() any {
				captures, listErr := store.ListCaptures()

				if listErr != nil {
					errnie.Error(errnie.Err(
						errnie.Internal,
						"backtest: list captures: "+listErr.Error(),
						listErr,
					))

					return []backtest.CaptureInfo{}
				}

				return captures
			})

			captureID, _ := run.Flags().GetInt64("capture")

			if captureID == 0 {
				captures, err := store.ListCaptures()

				if err != nil {
					errnie.Error(errnie.Err(errnie.Internal,
						"backtest: list captures: "+err.Error(), err,
					))
				}

				if len(captures) > 0 {
					captureID = captures[0].ID
				}
			}

			if captureID != 0 {
				replay.Select(captureID)
			}

			errnie.Info("symm backtest ready — dashboard: cd frontend && pnpm dev")

			return hub.Serve()
		},
	}

	command.Flags().Int64("capture", 0, "capture id to load (default: newest)")
	command.Flags().String("import", "", "import a legacy market-frames capture file and exit")

	return command
}

/*
importCapture loads one legacy market-frames capture (plain or zstd JSONL)
into the sqlite capture store verbatim, so sessions recorded before the store
existed become replayable.
*/
func importCapture(path string) error {
	errnie.Apply(&errnie.Config{Level: "info"})

	file, err := os.Open(path)

	if err != nil {
		return errnie.Error(errnie.Err(errnie.IO, "backtest: open import", err))
	}

	defer file.Close()

	var reader bufio.Reader

	if filepath.Ext(path) == ".zst" {
		decoder, decodeErr := zstd.NewReader(file)

		if decodeErr != nil {
			return errnie.Error(errnie.Err(errnie.IO, "backtest: zstd reader", decodeErr))
		}

		defer decoder.Close()
		reader = *bufio.NewReader(decoder)
	} else {
		reader = *bufio.NewReader(file)
	}

	dataPath := utils.ResolveDataPath()
	store, err := backtest.NewStore(filepath.Join(dataPath, "symm.sqlite"))

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "backtest: open store", err))
	}

	defer store.Close()

	writer, err := store.OpenCapture()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "backtest: open capture", err))
	}

	var frame struct {
		Endpoint   string          `json:"endpoint"`
		Payload    json.RawMessage `json:"payload"`
		ReceivedAt time.Time       `json:"received_at"`
	}

	decoder := json.NewDecoder(&reader)
	imported := 0

	for {
		if err := decoder.Decode(&frame); err != nil {
			break
		}

		if err := writer.Write(backtest.Frame{
			Endpoint:   frame.Endpoint,
			Payload:    frame.Payload,
			ReceivedAt: frame.ReceivedAt,
		}); err != nil {
			break
		}

		imported++
	}

	if err := writer.Close(); err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "backtest: close import", err))
	}

	errnie.Info(fmt.Sprintf("backtest: imported %d frames from %s", imported, path))

	return nil
}
