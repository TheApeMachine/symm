package data

import (
	"context"

	capnp "capnproto.org/go/capnp/v3"
	"github.com/bytedance/sonic"
	"github.com/theapemachine/errnie"
)

type SelectServer struct {
	path string
	out  float64
}

func NewSelect() *SelectServer {
	return &SelectServer{}
}

func (s *SelectServer) Write(ctx context.Context, call Select_write) error {
	pathStr, err := call.Args().Path()
	if err == nil && len(pathStr) > 0 {
		s.path = pathStr
	}

	dataBytes, err := call.Args().In()
	if err != nil || len(dataBytes) == 0 {
		return nil
	}

	// Try reading as Cap'n Proto WireMeasurement message
	msg, err := capnp.Unmarshal(dataBytes)
	if err == nil {
		measurement, err := ReadRootWireMeasurement(msg)
		if err == nil {
			metrics, err := measurement.Metrics()
			if err == nil && metrics.Len() > 0 {
				s.out = metrics.At(0).Raw()
				return nil
			}
		}
	}

	// Also support JSON map
	var m map[string]any
	if err := sonic.Unmarshal(dataBytes, &m); err == nil && m != nil {
		if val, ok := m[s.path].(float64); ok {
			s.out = val
			return nil
		}
	}

	return nil
}

func (s *SelectServer) Done(ctx context.Context, call Select_done) error {
	results, err := call.AllocResults()
	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "failed to alloc results", err))
	}

	results.SetOut(s.out)
	s.out = 0
	return nil
}
