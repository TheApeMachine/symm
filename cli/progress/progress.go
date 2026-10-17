package progress

import (
	"fmt"
	"os"

	"github.com/schollz/progressbar/v3"
)

type Bar struct {
	inner *progressbar.ProgressBar
}

func New(label string, total int64) *Bar {
	if total < 1 {
		total = 1
	}

	bar := progressbar.NewOptions64(
		total,
		progressbar.OptionSetDescription(label),
		progressbar.OptionSetWriter(os.Stderr),
		progressbar.OptionShowCount(),
		progressbar.OptionShowIts(),
		progressbar.OptionSetPredictTime(true),
		progressbar.OptionEnableColorCodes(true),
		progressbar.OptionSetWidth(40),
	)

	return &Bar{inner: bar}
}

func (bar *Bar) Step() {
	if bar == nil || bar.inner == nil {
		return
	}

	_ = bar.inner.Add(1)
}

/*
SetTotal resizes the bar after the real workload is known (e.g. trainable
excursion count). Current progress is preserved; callers should set total
before stepping when starting from a placeholder max.
*/
func (bar *Bar) SetTotal(total int64) {
	if bar == nil || bar.inner == nil {
		return
	}

	if total < 1 {
		total = 1
	}

	bar.inner.ChangeMax64(total)
}

func (bar *Bar) Finish() {
	if bar == nil || bar.inner == nil {
		return
	}

	_, _ = bar.inner.Write([]byte("\n"))
	_ = bar.inner.Finish()
}

func (bar *Bar) Describe(format string, args ...any) {
	if bar == nil || bar.inner == nil {
		return
	}

	bar.inner.Describe(fmt.Sprintf(format, args...))
}
