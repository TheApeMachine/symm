package arithmetic

import (
	"context"

	capnp "capnproto.org/go/capnp/v3"
)

/* decimalResult drives one decimal node through its protocol and returns its out. */
func decimalResult(client capnp.Client, send func(context.Context) error, done func(context.Context) ([]byte, error)) ([]byte, error) {
	ctx := context.Background()

	if err := send(ctx); err != nil {
		return nil, err
	}

	if err := client.WaitStreaming(); err != nil {
		return nil, err
	}
	return done(ctx)
}
