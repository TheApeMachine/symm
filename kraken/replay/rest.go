package replay

import (
	"context"

	"github.com/gofiber/fiber/v3"
)

/*
Rest is a no-op during replay; market data comes from the capture file.
*/
type Rest struct {
	ctx    context.Context
	cancel context.CancelFunc
}

func NewRest(ctx context.Context) (*Rest, error) {
	ctx, cancel := context.WithCancel(ctx)

	return &Rest{ctx: ctx, cancel: cancel}, nil
}

func (rest *Rest) Get(
	ctx context.Context,
	request fiber.Map,
	model any,
	headers ...map[string]string,
) error {
	return nil
}

func (rest *Rest) Post(
	ctx context.Context,
	request fiber.Map,
	model any,
	headers ...map[string]string,
) error {
	return nil
}

func (rest *Rest) Close() error {
	rest.cancel()

	return nil
}

func (rest *Rest) Error() error {
	return nil
}
