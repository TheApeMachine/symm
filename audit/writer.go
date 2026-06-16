package audit

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/bytedance/sonic"
	"github.com/theapemachine/errnie"
)

/*
Recorder is a generic jsonl data recorder that can be used
anywhere we need to record data to a file.
Producers marshal locally and append with O_APPEND so concurrent Write calls
need no mutex or channel.
*/
type Recorder struct {
	filename string
	fh       *os.File
}

func NewRecorder(filename string) (*Recorder, error) {
	if filename == "" {
		return nil, fmt.Errorf("audit: filename is required")
	}

	if err := os.MkdirAll(filepath.Dir(filename), 0o755); err != nil {
		return nil, err
	}

	fh, err := os.OpenFile(
		filename,
		os.O_CREATE|os.O_WRONLY|os.O_APPEND,
		0o644,
	)

	if err != nil {
		return nil, err
	}

	return &Recorder{
		filename: filename,
		fh:       fh,
	}, nil
}

func (recorder *Recorder) Write(event any) error {
	if recorder == nil || recorder.fh == nil {
		return errnie.Error(errnie.Err(
			errnie.IO,
			"audit: recorder is closed",
			nil,
		))
	}

	payload, err := sonic.Marshal(event)

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.IO,
			"audit: failed to marshal event",
			err,
		))
	}

	payload = append(payload, '\n')

	_, err = recorder.fh.Write(payload)

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.IO,
			"audit: failed to write event",
			err,
		))
	}

	return errnie.Error(err)
}

func (recorder *Recorder) Close() error {
	if recorder == nil || recorder.fh == nil {
		return errnie.Error(errnie.Err(
			errnie.IO,
			"audit: recorder is closed",
			nil,
		))
	}

	fh := recorder.fh
	recorder.fh = nil

	return fh.Close()
}
