package tests_test

import (
	"math"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/theapemachine/symm/nomagique/tests"
)

// Run the assertion in a child test process so a deliberately failed assertion
// can itself be tested. NaN must never compare equal to an expected finite value.
func TestEqualNumberRejectsNaN(t *testing.T) {
	if os.Getenv("NOMAGIQUE_ASSERT_NAN_CHILD") == "1" {
		tests.EqualNumber(t, math.NaN(), 1)
		return
	}
	command := exec.Command(os.Args[0], "-test.run=^TestEqualNumberRejectsNaN$")
	command.Env = append(os.Environ(), "NOMAGIQUE_ASSERT_NAN_CHILD=1")
	output, err := command.CombinedOutput()
	if err == nil || !strings.Contains(string(output), "wanted 1") {
		t.Fatalf("finite expectation accepted NaN: %s; error=%v", output, err)
	}
}
