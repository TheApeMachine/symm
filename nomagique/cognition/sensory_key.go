package cognition

import (
	"context"

	"github.com/theapemachine/errnie"
)

type SensoryKeyServer struct {
	out []byte
}

func NewSensoryKey() *SensoryKeyServer {
	return &SensoryKeyServer{}
}

func (s *SensoryKeyServer) Write(ctx context.Context, call SensoryKey_write) error {
	contextBytes, _ := call.Args().ContextBytes()
	buf := make([]byte, 2+len(contextBytes))
	buf[0] = 's'
	buf[1] = '/'
	copy(buf[2:], contextBytes)
	s.out = buf
	return nil
}

func (s *SensoryKeyServer) Done(ctx context.Context, call SensoryKey_done) error {
	results, err := call.AllocResults()
	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "failed to alloc results", err))
	}

	if err := results.SetOut(s.out); err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "failed to set out", err))
	}

	s.out = nil
	return nil
}
