package cognition

import (
	"bytes"
	"context"

	"github.com/theapemachine/errnie"
)

type ParseBasinKeyServer struct {
	class        []byte
	contextBytes []byte
	ok           bool
}

func NewParseBasinKey() *ParseBasinKeyServer {
	return &ParseBasinKeyServer{}
}

func (s *ParseBasinKeyServer) Write(ctx context.Context, call ParseBasinKey_write) error {
	k, _ := call.Args().Key()
	if len(k) < 4 || k[0] != 'b' || k[1] != '/' {
		s.ok = false
		return nil
	}

	rem := k[2:]
	idx := bytes.LastIndexByte(rem, '/')
	if idx <= 0 || idx == len(rem)-1 {
		s.ok = false
		return nil
	}

	s.class = bytes.Clone(rem[idx+1:])
	s.contextBytes = bytes.Clone(rem[:idx])
	s.ok = true
	return nil
}

func (s *ParseBasinKeyServer) Done(ctx context.Context, call ParseBasinKey_done) error {
	results, err := call.AllocResults()
	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "failed to alloc results", err))
	}

	_ = results.SetClass(s.class)
	_ = results.SetContextBytes(s.contextBytes)
	results.SetOk(s.ok)

	s.class = nil
	s.contextBytes = nil
	s.ok = false
	return nil
}
