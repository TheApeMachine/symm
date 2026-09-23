package data

import (
	"context"
	"strconv"
	"strings"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/* FormatServer renders one template per evaluation. */
type FormatServer struct {
	*runtime.System
	out []byte
}

func NewFormat(ctx context.Context) *FormatServer {
	return &FormatServer{
		System: runtime.NewSystem(ctx, "data.format"),
	}
}

func (server *FormatServer) Write(ctx context.Context, call Format_write) error {
	template, err := call.Args().Template()

	if err != nil {
		return server.Error(errnie.Err(errnie.Validation, "format: template", err))
	}
	values, err := call.Args().Values()

	if err != nil {
		return server.Error(errnie.Err(errnie.Validation, "format: values", err))
	}
	server.out = nil
	arrived := false

	for index := range values.Len() {
		value, err := values.At(index)

		if err != nil {
			return server.Error(errnie.Err(errnie.Validation, "format: value", err))
		}
		arrived = arrived || len(value) > 0
	}

	if !arrived {
		return nil
	}
	out := make([]byte, 0, len(template))

	for len(template) > 0 {
		open := strings.IndexAny(template, "{}")

		if open < 0 {
			out = append(out, template...)
			break
		}
		out = append(out, template[:open]...)
		template = template[open:]

		if strings.HasPrefix(template, "{{") || strings.HasPrefix(template, "}}") {
			out = append(out, template[0])
			template = template[2:]
			continue
		}
		closing := strings.IndexByte(template, '}')

		if template[0] == '}' || closing < 0 {
			return server.Error(errnie.Err(errnie.Validation, "format: unbalanced brace in template", nil))
		}
		index, err := strconv.Atoi(template[1:closing])

		if err != nil || index < 0 || index >= values.Len() {
			return server.Error(errnie.Err(errnie.Validation, "format: placeholder "+template[:closing+1]+" names no value", err))
		}
		value, err := values.At(index)

		if err != nil {
			return server.Error(errnie.Err(errnie.Validation, "format: value", err))
		}

		if len(value) == 0 {
			return server.Error(errnie.Err(errnie.Validation, "format: value "+template[:closing+1]+" did not arrive", nil))
		}
		out = append(out, value...)
		template = template[closing+1:]
	}
	server.out = out
	return nil
}

func (server *FormatServer) Done(ctx context.Context, call Format_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return server.Error(errnie.Err(errnie.Internal, "format: allocate result", err))
	}

	if server.out == nil {
		results.SetIdle()
		return nil
	}

	if err := results.SetOut(server.out); err != nil {
		return server.Error(errnie.Err(errnie.Internal, "format: emit", err))
	}
	server.out = nil
	return nil
}
