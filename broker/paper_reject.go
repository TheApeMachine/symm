package broker

import (
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"math"

	"github.com/theapemachine/symm/config"
)

/*
ShouldRejectPaperOrder simulates an exchange reject on the paper path. Uses
PaperOrderRejectRate always, and the higher of that and regime-scaled
ExecutionStressRejectRate when execution stress is enabled.
*/
func ShouldRejectPaperOrder(scope config.ExecutionScope, regime StressRegime) error {
	configured := configuredRejectRate(scope)

	if configured <= 0 {
		return nil
	}

	if scope.ExecutionStressEnabled {
		return GlobalStressMachine().RejectOutcome(configured, regime)
	}

	draw, err := cryptoFloat64()

	if err != nil {
		return fmt.Errorf("paper reject entropy: %w", err)
	}

	if draw < configured {
		return fmt.Errorf("paper order reject (rate=%.4f)", configured)
	}

	return nil
}

func configuredRejectRate(scope config.ExecutionScope) float64 {
	if scope.QuoteCurrency == "" {
		scope = config.ExecutionScopeFrom(config.System)
	}

	rate := scope.PaperOrderRejectRate

	if scope.ExecutionStressEnabled {
		rate = math.Max(rate, scope.ExecutionStressRejectRate)
	}

	return rate
}

func cryptoFloat64() (float64, error) {
	var bytes [8]byte

	if _, err := rand.Read(bytes[:]); err != nil {
		return 0, err
	}

	bits := binary.LittleEndian.Uint64(bytes[:])

	return float64(bits) / (1 << 64), nil
}
