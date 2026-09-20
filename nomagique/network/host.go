package network

import (
	"context"
	"os"

	"github.com/theapemachine/errnie"
)

type Host struct {
	ctx    context.Context
	cancel context.CancelFunc
	err    error
	Name   string
	IP     string
	Port   int
}

type hostOption func(*Host)

func NewHost(ctx context.Context, opts ...hostOption) (*Host, error) {
	ctx, cancel := context.WithCancel(ctx)

	host := &Host{
		ctx:    ctx,
		cancel: cancel,
	}

	if host.Name, host.err = os.Hostname(); host.err != nil {
		return host, host.err
	}

	for _, opt := range opts {
		opt(host)
	}

	return host, errnie.Require(map[string]any{
		"ctx":    host.ctx,
		"cancel": host.cancel,
		"name":   host.Name,
		"ip":     host.IP,
		"port":   host.Port,
	})
}

func (host *Host) Close() error {
	host.cancel()
	return host.err
}

func (host *Host) Error() error {
	return host.err
}
