package cognition

import (
	"context"

	"github.com/theapemachine/errnie"
)

type BasinKeyServer struct {
	out []byte
}

func NewBasinKey() *BasinKeyServer {
	return &BasinKeyServer{}
}

func (s *BasinKeyServer) Write(ctx context.Context, call BasinKey_write) error {
	class, err := call.Args().Class()
	if err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "failed to read class", err))
	}

	contextBytes, err := call.Args().ContextBytes()
	if err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "failed to read contextBytes", err))
	}

	buf := make([]byte, 2+len(contextBytes)+1+len(class))
	buf[0] = 'b'
	buf[1] = '/'
	copy(buf[2:], contextBytes)
	buf[2+len(contextBytes)] = '/'
	copy(buf[3+len(contextBytes):], class)
	s.out = buf
	return nil
}

func (s *BasinKeyServer) Done(ctx context.Context, call BasinKey_done) error {
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
