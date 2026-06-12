package broker

import (
	"os"
	"testing"

	"github.com/theapemachine/symm/internal/testconfig"
)

func TestMain(testMain *testing.M) {
	testconfig.MustLoad()
	os.Exit(testMain.Run())
}
