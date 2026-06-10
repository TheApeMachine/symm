package audit

import (
	"os"

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
	fh, err := os.OpenFile(
		filename,
		os.O_CREATE|os.O_WRONLY|os.O_TRUNC|os.O_APPEND,
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
	payload, err := sonic.Marshal(event)

	if err != nil {
		return errnie.Error(err)
	}

	payload = append(payload, '\n')

	_, err = recorder.fh.Write(payload)

	return errnie.Error(err)
}

func (recorder *Recorder) Close() error {
	return recorder.fh.Close()
}
