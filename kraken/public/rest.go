package public

import (
	"context"
	"fmt"

	"github.com/gofiber/fiber/v3/client"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/errnie"
)

type RestClient interface {
	Do(
		ctx context.Context,
		artifact *datura.Artifact,
	) *datura.Artifact
}

/*
Rest is the REST client for the Kraken public API.
*/
type Rest struct {
	ctx    context.Context
	cancel context.CancelFunc
	err    error
	tree   *dmt.Tree
	client *client.Client
}

func NewRest(
	ctx context.Context,
	tree *dmt.Tree,
) *Rest {
	ctx, cancel := context.WithCancel(ctx)

	return &Rest{
		ctx:    ctx,
		cancel: cancel,
		tree:   tree,
		client: client.New(),
	}
}

func (rest *Rest) Do(
	ctx context.Context, artifact *datura.Artifact,
) *datura.Artifact {
	destination := errnie.Does(func() (string, error) {
		return artifact.Destination()
	}).Or(func(err error) {
		errnie.Error(errnie.Err(
			errnie.Validation,
			"kraken/public: failed to get destination",
			err,
		))
	}).Value()

	request := rest.client.R().SetMethod(
		datura.Peek[string](artifact, "method"),
	).SetURL(
		destination,
	).SetHeaders(
		datura.Peek[map[string]string](artifact, "headers"),
	)

	response, sendErr := request.Send()

	if sendErr != nil {
		rest.err = sendErr
		statusCode := 0

		if response != nil {
			statusCode = response.StatusCode()
		}

		return datura.Acquire(
			destination, datura.APPJSON,
		).WithError(
			errnie.Error(errnie.Err(
				errnie.IO,
				fmt.Sprintf(
					"kraken/public: failed to get %s, error code: %d",
					destination,
					statusCode,
				),
				sendErr,
			)),
		)
	}

	out := datura.Acquire(
		destination, datura.APPJSON,
	).WithDestination(
		destination,
	).WithPayload(
		response.Body(),
	)

	rest.tree.Insert(out.Prefix(), errnie.Does(func() ([]byte, error) {
		return out.Message().Marshal()
	}).Or(func(err error) {
		errnie.Error(errnie.Err(
			errnie.Validation,
			"kraken/public: failed to marshal artifact",
			err,
		))
	}).Value())

	out.Release()

	return out
}

func (rest *Rest) Error() error {
	return errnie.Error(rest.err)
}

func (rest *Rest) Close() error {
	rest.cancel()
	return errnie.Error(rest.ctx.Err())
}
