package cmd

import (
	"context"
	"os"
	"os/signal"
	"syscall"
)

func signalNotifyContext() (context.Context, context.CancelFunc) {
	return signal.NotifyContext(
		context.Background(), os.Interrupt, syscall.SIGTERM,
	)
}
