package depthflow

import (
	"testing"

	"github.com/theapemachine/symm/internal/testconfig"
)

func TestMain(m *testing.M) {
	testconfig.SeedRegimeDefaults()
	m.Run()
}
