package fluid

import (
	"os"
	"testing"

	"github.com/theapemachine/symm/internal/testconfig"
)

func TestMain(m *testing.M) {
	testconfig.SeedRegimeDefaults()
	os.Exit(m.Run())
}
