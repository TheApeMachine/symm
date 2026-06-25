package manifold

import (
	"fmt"
	"time"

	mkernel "github.com/theapemachine/nomagique/physics/manifold"
)

func (field *Field) maybeStep(at time.Time) error {
	if at.IsZero() {
		return fmt.Errorf("manifold: step event time must be set")
	}

	stepInterval := field.config.IntegrationInterval()
	shouldIntegrate := field.lastStepAt.IsZero() || at.Sub(field.lastStepAt) >= stepInterval

	if !shouldIntegrate {
		return nil
	}

	integrated, err := field.integrate(at)

	if err != nil {
		return err
	}

	if !integrated {
		return nil
	}

	return nil
}

func (field *Field) hasPublishableSnapshot() bool {
	return field.solver != nil &&
		!field.lastStepAt.IsZero() &&
		len(field.lastCarriers) > 0 &&
		readingFinite(field.lastReading)
}

func readingFinite(reading mkernel.Reading) bool {
	return reading.IsFinite()
}
