package optimizer

import (
	"fmt"
	"os"
)

/*
TuneLog prints one progress line to stderr for long-running tune phases.
*/
func TuneLog(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "symm tune: "+format+"\n", args...)
}
