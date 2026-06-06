package replay

import (
	"testing"

	"github.com/theapemachine/symm/internal/testconfig"
	"github.com/theapemachine/symm/market/perspectives/types"
)

func init() {
	testconfig.MustLoad()
}

func mustPrecompileTape(t testing.TB, rows []types.Measurement) ReplayTape {
	t.Helper()

	tape, err := PrecompileTape(rows)

	if err != nil {
		t.Fatal(err)
	}

	return tape
}
