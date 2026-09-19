package system

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/transport"
)

/*
Paper coordinates simulated paper trading via the external kraken paper CLI.
It wraps the generic transport.Process primitive.
*/
type Paper struct {
	ctx         context.Context
	cancel      context.CancelFunc
	commandMu   sync.Mutex
	proc        transport.Process
	onBalance   func(map[string]any)
	onExecution func(map[string]any)
}

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

	return &Paper{
		ctx:         ctx,
		cancel:      cancel,
		proc:        transport.NewProcess(binary, "paper"),
		onBalance:   onBalance,
		onExecution: onExecution,
	}
}

func (p *Paper) Close() {
	p.cancel()
}

func (p *Paper) Balance() (map[string]any, error) {
	p.commandMu.Lock()
	defer p.commandMu.Unlock()

	out := p.proc([]string{"balance", "-o", "json"})
	if len(out) == 0 {
		return nil, errnie.Error(errnie.Err(
			errnie.IO,
			"paper: balance command returned empty response",
			nil,
		))
	}

	trimmed := strings.TrimSpace(string(out))
	lines := strings.Split(trimmed, "\n")
	var jsonStr string

	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if strings.HasPrefix(line, "{") && strings.HasSuffix(line, "}") {
			jsonStr = line
			break
		}
	}

	if jsonStr == "" {
		jsonStr = trimmed
	}

	var parsed map[string]any
	if err := json.Unmarshal([]byte(jsonStr), &parsed); err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"paper: failed to parse balance json",
			err,
		))
	}

	if p.onBalance != nil {
		p.onBalance(parsed)
	}

	return parsed, nil
}

func (p *Paper) Status() (map[string]any, error) {
	p.commandMu.Lock()
	defer p.commandMu.Unlock()

	out := p.proc([]string{"status", "-o", "json"})
	if len(out) == 0 {
		return nil, errnie.Error(errnie.Err(
			errnie.IO,
			"paper: status command returned empty response",
			nil,
		))
	}

	trimmed := strings.TrimSpace(string(out))
	var parsed map[string]any

	if err := json.Unmarshal([]byte(trimmed), &parsed); err != nil {
		return map[string]any{"raw": trimmed}, nil
	}

	return parsed, nil
}

func (p *Paper) Execute(action, symbol string, amount, price float64) (map[string]any, error) {
	p.commandMu.Lock()
	defer p.commandMu.Unlock()

	args := []string{
		action,
		symbol,
		fmt.Sprintf("%f", amount),
	}

	if price > 0 {
		args = append(args, fmt.Sprintf("%f", price))
	}

	out := p.proc(args)
	if len(out) == 0 {
		return nil, errnie.Error(errnie.Err(
			errnie.IO,
			fmt.Sprintf("paper: order execution %s failed", action),
			nil,
		))
	}

	trimmed := strings.TrimSpace(string(out))
	var parsed map[string]any

	if err := json.Unmarshal([]byte(trimmed), &parsed); err != nil {
		parsed = map[string]any{
			"action":    action,
			"symbol":    symbol,
			"raw":       trimmed,
			"timestamp": time.Now().UnixNano(),
		}
	}

	if p.onExecution != nil {
		p.onExecution(parsed)
	}

	return parsed, nil
}

func (p *Paper) StartPoll(interval time.Duration) {
	if interval <= 0 {
		interval = 5 * time.Second
	}

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-p.ctx.Done():
				return
			case <-ticker.C:
				_, _ = p.Balance()
			}
		}
	}()
}
