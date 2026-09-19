package transport

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/theapemachine/errnie"
)

/*
Paper manages simulated spot trading by delegating to the native `kraken paper` CLI.
It maintains thread-safe execution of CLI commands and notifies subscribers of wallet
and order execution updates.
*/
type Paper struct {
	ctx         context.Context
	cancel      context.CancelFunc
	commandMu   sync.Mutex
	binaryPath  string
	onBalance   func(map[string]any)
	onExecution func(map[string]any)
}

/*
NewPaper constructs a paper trading coordinator.
*/
func NewPaper(
	ctx context.Context,
	onBalance func(map[string]any),
	onExecution func(map[string]any),
) *Paper {
	ctx, cancel := context.WithCancel(ctx)

	binary := "kraken"
	if customBin := os.Getenv("KRAKEN_BIN"); customBin != "" {
		binary = customBin
	}

	if binary == "kraken" {
		if _, err := exec.LookPath("kraken"); err != nil {
			cargoPath := "/Users/theapemachine/.cargo/bin/kraken"
			if _, cargoErr := os.Stat(cargoPath); cargoErr == nil {
				binary = cargoPath
			}
		}
	}

	paper := &Paper{
		ctx:         ctx,
		cancel:      cancel,
		binaryPath:  binary,
		onBalance:   onBalance,
		onExecution: onExecution,
	}

	return paper
}

/*
Execute runs an arbitrary `kraken paper <command...>` invocation with `--output json`.
*/
func (paper *Paper) Execute(command ...string) (map[string]any, error) {
	paper.commandMu.Lock()
	defer paper.commandMu.Unlock()

	input := []string{"paper"}
	input = append(input, command...)
	input = append(input, "--output", "json")

	cmd := exec.CommandContext(paper.ctx, paper.binaryPath, input...)
	if errors.Is(cmd.Err, exec.ErrDot) {
		cmd.Err = nil
	}

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	stdout, err := cmd.Output()
	if err != nil {
		details := strings.TrimSpace(stderr.String())
		if details == "" {
			details = strings.TrimSpace(string(stdout))
		}

		if details == "" {
			details = err.Error()
		}

		return nil, errnie.Error(errnie.Err(
			errnie.IO,
			"paper: kraken command failed: "+details,
			err,
		))
	}

	var result map[string]any
	if err := json.Unmarshal(stdout, &result); err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"paper: unmarshal json output",
			err,
		))
	}

	if errMsg, ok := result["error"].(string); ok && errMsg != "" {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"paper: command error: "+errMsg,
			nil,
		))
	}

	return result, nil
}

/*
Status retrieves current paper simulation account status.
*/
func (paper *Paper) Status() (map[string]any, error) {
	return paper.Execute("status", "--verbose")
}

/*
Balance retrieves paper account balances and fires onBalance if configured.
*/
func (paper *Paper) Balance() (map[string]any, error) {
	result, err := paper.Execute("balance", "--verbose")
	if err != nil {
		return nil, err
	}

	if paper.onBalance != nil {
		paper.onBalance(result)
	}

	return result, nil
}

/*
Buy submits a paper buy order.
*/
func (paper *Paper) Buy(pair, volume, price string) (map[string]any, error) {
	args := []string{"buy", pair, volume}
	if price != "" {
		args = append(args, "--type", "limit", "--price", price)
	}

	result, err := paper.Execute(args...)
	if err != nil {
		return nil, err
	}

	if paper.onExecution != nil {
		paper.onExecution(result)
	}

	_, _ = paper.Balance()
	return result, nil
}

/*
Sell submits a paper sell order.
*/
func (paper *Paper) Sell(pair, volume, price string) (map[string]any, error) {
	args := []string{"sell", pair, volume}
	if price != "" {
		args = append(args, "--type", "limit", "--price", price)
	}

	result, err := paper.Execute(args...)
	if err != nil {
		return nil, err
	}

	if paper.onExecution != nil {
		paper.onExecution(result)
	}

	_, _ = paper.Balance()
	return result, nil
}

/*
Cancel cancels an open paper order.
*/
func (paper *Paper) Cancel(orderID string) (map[string]any, error) {
	return paper.Execute("cancel", orderID, "--yes")
}

/*
Reset resets the paper trading wallet to initial state.
*/
func (paper *Paper) Reset() (map[string]any, error) {
	return paper.Execute("reset", "--yes")
}

/*
StartPoll begins periodic balance polling on the given interval.
*/
func (paper *Paper) StartPoll(interval time.Duration) {
	if interval <= 0 {
		interval = 5 * time.Second
	}

	ticker := time.NewTicker(interval)
	go func() {
		defer ticker.Stop()

		for {
			select {
			case <-paper.ctx.Done():
				return
			case <-ticker.C:
				_, _ = paper.Balance()
			}
		}
	}()
}

/*
Close cleans up the paper runner.
*/
func (paper *Paper) Close() error {
	paper.cancel()
	return nil
}
