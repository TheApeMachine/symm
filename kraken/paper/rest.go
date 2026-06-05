package paper

import (
	"context"
	"fmt"
	"time"

	"github.com/bytedance/sonic"
	"github.com/gofiber/fiber/v3"
)

/*
Rest is a fake REST client for paper trading that simulates Kraken private REST
responses used by token bootstrap and reconciliation paths.
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
	return fmt.Errorf("paper rest: GET not supported")
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
	if err := ctx.Err(); err != nil {
		return err
	}

	return rest.webSocketsToken(model)
}

func (rest *Rest) webSocketsToken(model any) error {
	if model == nil {
		return nil
	}

	payload, err := sonic.Marshal(map[string]any{
		"token":   fmt.Sprintf("paper-%d", time.Now().UnixNano()),
		"expires": int((15 * time.Minute).Seconds()),
	})

	if err != nil {
		return fmt.Errorf("paper rest: encode token: %w", err)
	}

	return sonic.Unmarshal(payload, model)
}

/*
Close closes the REST client.
*/
func (rest *Rest) Close() error {
	rest.cancel()
	return nil
}

/*
Error returns the error of the REST client.
*/
func (rest *Rest) Error() error {
	return nil
}
