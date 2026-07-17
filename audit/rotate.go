package audit

import (
	"fmt"
	"os"
	"time"

	"github.com/theapemachine/errnie"
)

/*
Rotate renames an existing audit file aside with a UTC stamp so the next
recorder opens a fresh append stream. Missing files are a no-op.
*/
func Rotate(filename string) error {
	if filename == "" {
		return fmt.Errorf("audit: filename is required")
	}

	info, err := os.Stat(filename)

	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}

		return errnie.Err(errnie.IO, "audit: stat rotate source", err)
	}

	if info.IsDir() {
		return fmt.Errorf("audit: rotate path is a directory: %s", filename)
	}

	stamped := filename + "." + time.Now().UTC().Format("20060102T150405Z")

	if err := os.Rename(filename, stamped); err != nil {
		return errnie.Err(errnie.IO, "audit: rotate rename failed", err)
	}

	return nil
}
