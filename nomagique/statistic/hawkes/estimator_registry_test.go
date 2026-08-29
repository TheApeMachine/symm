package hawkes

import "testing"

func TestEstimatorRegistryFitsAfterEnoughEvents(testingT *testing.T) {
	registry := NewEstimatorRegistry()
	fitted := false
	atSec := 0.0

	for index := 0; index < 200; index++ {
		atSec = float64(index) * 0.3
		buy := index%2 == 0
		_, ok := registry.Observe("BTCUSD", atSec, buy)

		if ok {
			fitted = true
		}
	}

	if !fitted {
		testingT.Fatal("expected the registry to converge on a fit given enough arrivals")
	}

	params, ok := registry.Observe("BTCUSD", atSec+0.3, true)

	if !ok {
		testingT.Fatal("expected a fitted result on the next observation")
	}

	if params.Beta <= 0 {
		testingT.Fatalf("expected a positive fitted beta, got %v", params.Beta)
	}
}

func TestEstimatorRegistryKeepsSymbolsIndependent(testingT *testing.T) {
	registry := NewEstimatorRegistry()

	for index := 0; index < 200; index++ {
		atSec := float64(index) * 0.3
		registry.Observe("BTCUSD", atSec, index%2 == 0)
	}

	_, ok := registry.Observe("ETHUSD", 0, true)

	if ok {
		testingT.Fatal("expected a fresh symbol to have no fit yet")
	}
}

func TestEstimatorRegistryWithholdsFitBeforeEnoughEvents(testingT *testing.T) {
	registry := NewEstimatorRegistry()

	_, ok := registry.Observe("BTCUSD", 0, true)

	if ok {
		testingT.Fatal("expected no fit from a single observation")
	}
}
