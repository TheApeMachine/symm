package paper

import (
	"fmt"
	"math/rand"
	"sync"

	"github.com/spf13/viper"
)

const defaultFailureInjectionSeed int64 = 1

/*
paperFailureInjection owns optional chaos behavior for deterministic paper sockets.
*/
type paperFailureInjection struct {
	mutex              sync.Mutex
	random             *rand.Rand
	enabled            bool
	connectFailureRate float64
	disconnectRate     float64
}

func newPaperFailureInjectionFromConfig() (*paperFailureInjection, error) {
	connectFailureRate := viper.GetFloat64(
		"trading.paper.failure_injection.connect_failure_rate",
	)
	disconnectRate := viper.GetFloat64(
		"trading.paper.failure_injection.disconnect_rate",
	)

	if err := validateFailureRate("connect_failure_rate", connectFailureRate); err != nil {
		return nil, err
	}

	if err := validateFailureRate("disconnect_rate", disconnectRate); err != nil {
		return nil, err
	}

	seed := viper.GetInt64("trading.paper.failure_injection.seed")

	if seed == 0 {
		seed = defaultFailureInjectionSeed
	}

	return &paperFailureInjection{
		random:             rand.New(rand.NewSource(seed)),
		enabled:            viper.GetBool("trading.paper.failure_injection.enabled"),
		connectFailureRate: connectFailureRate,
		disconnectRate:     disconnectRate,
	}, nil
}

func validateFailureRate(name string, failureRate float64) error {
	if failureRate < 0 || failureRate > 1 {
		return fmt.Errorf(
			"paper websocket: trading.paper.failure_injection.%s must be between 0 and 1",
			name,
		)
	}

	return nil
}

func (injection *paperFailureInjection) ConnectFailed() bool {
	if injection == nil {
		return false
	}

	return injection.failureHit(injection.connectFailureRate)
}

func (injection *paperFailureInjection) Disconnected() bool {
	if injection == nil {
		return false
	}

	return injection.failureHit(injection.disconnectRate)
}

func (injection *paperFailureInjection) failureHit(failureRate float64) bool {
	if !injection.enabled || failureRate <= 0 {
		return false
	}

	if failureRate >= 1 {
		return true
	}

	injection.mutex.Lock()
	defer injection.mutex.Unlock()

	return injection.random.Float64() < failureRate
}
