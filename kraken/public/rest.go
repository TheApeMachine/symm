package public

import (
	"context"
	"fmt"

	"github.com/gofiber/fiber/v3/client"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/errnie"
	"github.com/valyala/fasthttp"
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
	ctx      context.Context
	cancel   context.CancelFunc
	err      error
	tree     *dmt.Tree
	client   *client.Client
	endpoint EndpointType
}

func NewRest(
	ctx context.Context,
	endpoint EndpointType,
	tree *dmt.Tree,
) *Rest {
	ctx, cancel := context.WithCancel(ctx)

	return &Rest{
		ctx:      ctx,
		cancel:   cancel,
		tree:     tree,
		client:   client.New(),
		endpoint: endpoint,
	}
}

func (rest *Rest) Do(
	ctx context.Context, artifact *datura.Artifact,
) *datura.Artifact {
	request := rest.client.R().SetMethod(
		datura.Peek[string](artifact, "method"),
	).SetURL(
		datura.Peek[string](artifact, "destination"),
	).SetHeaders(
		datura.Peek[map[string]string](artifact, "headers"),
	)

	response := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseResponse(response)

	if rest.err = rest.client.Do(request.RawRequest, response); rest.err != nil || response == nil {
		return datura.Acquire(
			string(rest.endpoint), datura.APPJSON,
		).WithError(
			errnie.Error(errnie.Err(
				errnie.IO,
				fmt.Sprintf(
					"kraken/public: failed to get %s, error code: %d",
					string(rest.endpoint),
					response.StatusCode(),
				),
				rest.err,
			)),
		)
	}

	out := datura.Acquire(
		string(rest.endpoint), datura.APPJSON,
	).WithDestination(
		string(rest.endpoint),
	).WithPayload(
		response.Body(),
	)

	rest.tree.Insert(out.Prefix(), out.Marshal())
	return out
}

func (rest *Rest) Error() error {
	return errnie.Error(rest.err)
}

func (rest *Rest) Close() error {
	rest.cancel()
	return errnie.Error(rest.ctx.Err())
}
