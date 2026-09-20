package data

import (
	"context"
	"strings"

	"github.com/bytedance/sonic"
	"github.com/theapemachine/symm/nomagique/types"
)

type SelectServer struct {
	Downstream func(context.Context, any) error
	Path       string
}

func NewSelectServer(path string) *SelectServer {
	return &SelectServer{Path: path}
}

func (s *SelectServer) Evaluate(ctx context.Context, in any) (any, error) {
	segments := strings.Split(s.Path, ".")
	curr := in

	for _, segment := range segments {
		if curr == nil {
			break
		}

		asMap, ok := curr.(map[string]any)
		if !ok {
			curr = nil
			break
		}

		curr = asMap[segment]
	}

	if curr == nil {
		return nil, nil
	}

	if s.Downstream != nil {
		return curr, s.Downstream(ctx, curr)
	}

	return curr, nil
}

func (s *SelectServer) Write(ctx context.Context, call Select_write) error {
	args, err := call.Args().Select()
	if err != nil {
		return err
	}

	pathStr, err := args.Path()
	if err != nil {
		return err
	}
	s.Path = pathStr

	payloadPtr, err := args.Payload()
	if err != nil {
		return err
	}

	if !payloadPtr.IsValid() {
		return nil
	}

	data := payloadPtr.Data()
	if len(data) == 0 {
		return nil
	}

	var in any
	if err := sonic.Unmarshal(data, &in); err != nil {
		return err
	}

	_, evalErr := s.Evaluate(ctx, in)
	return evalErr
}

func (s *SelectServer) Done(ctx context.Context, call Select_done) error {
	return nil
}

type SelectNode types.StreamNode[any, any]

func NewSelect(path types.String) SelectNode {
	server := &SelectServer{}
	return types.NewStreamNode(server, func(ctx context.Context, in any) error {
		p := ""
		if path != nil {
			p = path(in)
		}
		server.Path = p
		_, err := server.Evaluate(ctx, in)
		return err
	}, func(next func(context.Context, any) error) {
		server.Downstream = func(ctx context.Context, res any) error {
			return next(ctx, res)
		}
	})
}


