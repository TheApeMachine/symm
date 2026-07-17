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

	stamped := RotatedPath(filename)

	if err := os.Rename(filename, stamped); err != nil {
		return errnie.Err(errnie.IO, "audit: rotate rename failed", err)
	}

	return nil
}

/*
RotatedPath returns path with a UTC second stamp and nanosecond suffix so
same-second rotations never collide.
*/
func RotatedPath(path string) string {
	now := time.Now().UTC()

	return path + "." + now.Format("20060102T150405Z") +
		fmt.Sprintf(".%09d", now.Nanosecond())
}
