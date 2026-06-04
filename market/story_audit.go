package market

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/spf13/viper"
	"github.com/theapemachine/symm/market/perspectives"
)

const defaultPlaybookWalkCooldown = 60 * time.Second

type playbookWalkDedup struct {
	cooldown time.Duration
	mu       sync.Mutex
	last     map[string]playbookWalkSignature
}

type playbookWalkSignature struct {
	key      string
	loggedAt time.Time
}

func newPlaybookWalkDedup() playbookWalkDedup {
	cooldown := viper.GetDuration("trading.audit.gate_cooldown")

	if cooldown <= 0 {
		if raw := strings.TrimSpace(os.Getenv("SYMM_AUDIT_GATE_COOLDOWN")); raw != "" {
			parsed, err := time.ParseDuration(raw)

			if err == nil {
				cooldown = parsed
			}
		}
	}

	if cooldown <= 0 {
		cooldown = defaultPlaybookWalkCooldown
	}

	return playbookWalkDedup{
		cooldown: cooldown,
		last:     make(map[string]playbookWalkSignature),
	}
}

func (dedup *playbookWalkDedup) shouldLog(
	symbol string,
	walkAudit *perspectives.WalkAudit,
	blockReason string,
) bool {
	if walkAudit == nil {
		return false
	}

	key := playbookWalkKey(walkAudit, blockReason)

	dedup.mu.Lock()
	defer dedup.mu.Unlock()

	previous, seen := dedup.last[symbol]
	now := time.Now()

	if seen && previous.key == key && now.Sub(previous.loggedAt) < dedup.cooldown {
		return false
	}

	dedup.last[symbol] = playbookWalkSignature{
		key:      key,
		loggedAt: now,
	}

	return true
}

func playbookWalkKey(
	walkAudit *perspectives.WalkAudit,
	blockReason string,
) string {
	verdict := "none"

	if walkAudit.Verdict != nil {
		verdict = walkAudit.Verdict.String()
	}

	return fmt.Sprintf("%s:%d:%s", verdict, walkAudit.VerdictDepth, blockReason)
}

func playbookWalkFrame(
	measurement perspectives.Measurement,
	walkAudit *perspectives.WalkAudit,
	evalErr error,
	blockReason string,
) (map[string]any, error) {
	if walkAudit == nil {
		return nil, fmt.Errorf("playbook walk frame: nil audit")
	}

	frame := map[string]any{
		"audit_event": "playbook_walk",
		"symbol":      measurement.Symbol,
		"source":      measurement.Source.String(),
		"category":    measurement.Category,
		"snr":         measurement.SNR,
		"confidence":  measurement.Confidence,
		"last":        measurement.Last,
		"steps":       walkAudit.Steps,
		"selected":    walkAudit.SelectedPath,
		"depth":       walkAudit.VerdictDepth,
	}

	if walkAudit.Verdict != nil {
		frame["verdict"] = *walkAudit.Verdict
	}

	if blockReason != "" {
		frame["block_reason"] = blockReason
	}

	if evalErr != nil {
		frame["eval_err"] = evalErr.Error()
	}

	holding := false

	for observation, value := range walkAudit.Context.Observations {
		if observation == perspectives.ObservationHolding && value != 0 {
			holding = true
		}
	}

	frame["holding"] = holding

	snapshots := make([]map[string]any, 0, len(walkAudit.Context.Measurements))

	for _, snapshot := range walkAudit.Context.Measurements {
		snapshots = append(snapshots, map[string]any{
			"source":     snapshot.Source.String(),
			"category":   snapshot.Category,
			"snr":        snapshot.SNR,
			"confidence": snapshot.Confidence,
			"strength":   snapshot.Strength,
		})
	}

	frame["snapshots"] = snapshots

	return frame, nil
}
