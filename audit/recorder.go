package audit

import (
	"context"
	"os"
	"sync"

	"github.com/bytedance/sonic"
	"github.com/theapemachine/nomagique/runtime"
)

var Record *Recorder

func init() {
	Record = NewRecorder(context.Background(), "audit.jsonl")
}

type Recorder struct {
	*runtime.System
	mu sync.Mutex
	fh *os.File
}

func NewRecorder(ctx context.Context, path string) *Recorder {
	fh, err := os.Create(path)

	if err != nil {
		return nil
	}

	return &Recorder{
		System: runtime.NewSystem(ctx, "recorder"),
		fh:     fh,
	}
}

func (r *Recorder) Close() error {
	return r.fh.Close()
}

func (r *Recorder) Write(obj any) (n int, err error) {
	if r == nil || r.fh == nil {
		return 0, nil
	}

	buf, err := sonic.Marshal(obj)
	if err != nil {
		return 0, err
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, err := r.fh.Write(append(buf, '\n')); err != nil {
		return 0, err
	}

	return len(buf) + 1, nil
}
