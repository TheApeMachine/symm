package market

import (
	"context"
	"testing"

	"github.com/theapemachine/qpool"
)

func NewTestTouchRegistry(
	test *testing.T,
	ctx context.Context,
	pool *qpool.Q[any],
) *TouchRegistry {
	test.Helper()

	registry, registryErr := NewTouchRegistry(ctx, pool)

	if registryErr != nil {
		test.Fatal(registryErr)
	}

	RegisterTouchRegistry(registry)

	return registry
}
