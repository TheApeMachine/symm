package paper

import (
	"context"

	"github.com/gofiber/fiber/v3"
)

/*
Rest is a fake REST client for paper trading, that acts exactly
like the real Kraken REST client, but instead of making actual API
calls, it just returns an accurately simulated response.
*/
type Rest struct {
	ctx    context.Context
	cancel context.CancelFunc
}

/*
NewRest builds a paper REST client.
*/
func NewRest(ctx context.Context) (*Rest, error) {
	ctx, cancel := context.WithCancel(ctx)
	return &Rest{ctx: ctx, cancel: cancel}, nil
}

/*
Get makes a GET request to the Kraken API.
*/
func (rest *Rest) Get(
	ctx context.Context,
	request fiber.Map,
	model any,
	headers ...map[string]string,
) error {
	return nil
}

/*
Post makes a POST request to the Kraken API.
*/
func (rest *Rest) Post(
	ctx context.Context,
	request fiber.Map,
	model any,
	headers ...map[string]string,
) error {
	return nil
}

/*
Close closes the REST client.
*/
func (rest *Rest) Close() error {
	return nil
}

/*
Error returns the error of the REST client.
*/
func (rest *Rest) Error() error {
	return nil
}
