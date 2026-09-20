package cognition

import (
	"github.com/theapemachine/symm/nomagique/types"
	"bytes"
	"context"
)

type ParseBasinKeyServer struct {
	Downstream func(context.Context, []byte, []byte, bool) error
}

func (s *ParseBasinKeyServer) Write(ctx context.Context, call ParseBasinKey_write) error {
	k, _ := call.Args().Key()
	if len(k) < 4 || k[0] != 'b' || k[1] != '/' {
		return s.Downstream(ctx, nil, nil, false)
	}
	rem := k[2:]
	idx := bytes.LastIndexByte(rem, '/')
	if idx <= 0 || idx == len(rem)-1 {
		return s.Downstream(ctx, nil, nil, false)
	}
	return s.Downstream(ctx, rem[idx+1:], rem[:idx], true)
}

func (s *ParseBasinKeyServer) Done(ctx context.Context, call ParseBasinKey_done) error {
	return nil
}



type ParseBasinKeyNode types.StreamNode[any, any]

func NewParseBasinKey() ParseBasinKeyNode {
	server := &ParseBasinKeyServer{}
	return types.NewStreamNode(
		server,
		func(ctx context.Context, payload any) error {
			return nil
		},
		func(next func(context.Context, any) error) {},
	)
}
