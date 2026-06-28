package public

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/bytedance/sonic"
	"github.com/gofiber/fiber/v3/client"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken/types"
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
	ctx         context.Context
	cancel      context.CancelFunc
	err         error
	tree        *dmt.Tree
	client      *client.Client
	destination string
	token       *Token
}

func NewRest(
	ctx context.Context,
	tree *dmt.Tree,
	destination string,
) *Rest {
	ctx, cancel := context.WithCancel(ctx)

	return &Rest{
		ctx:         ctx,
		cancel:      cancel,
		tree:        tree,
		client:      client.New(),
		destination: destination,
		token:       NewToken(ctx, destination),
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
	).SetContext(
		ctx,
	).SetURL(
		destination,
	).SetHeaders(
		datura.Peek[map[string]string](
			rest.token.Header(artifact), "headers",
		),
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

func (rest *Rest) WebSocketToken(ctx context.Context, token *types.Token) error {
	if rest == nil || token == nil {
		return fmt.Errorf("kraken/public: token rest unavailable")
	}

	nonce := strconv.FormatInt(time.Now().UnixNano(), 10)
	body := url.Values{"nonce": []string{nonce}}.Encode()
	signature, err := rest.token.sign(EndpointWebSocketsToken.SignPath(), nonce, body)

	if err != nil {
		return err
	}

	response, sendErr := rest.client.R().
		SetMethod(http.MethodPost).
		SetContext(ctx).
		SetURL(string(EndpointWebSocketsToken)).
		SetHeaders(map[string]string{
			"API-Key":      rest.token.apiKey,
			"API-Sign":     signature,
			"Content-Type": "application/x-www-form-urlencoded",
		}).
		SetRawBody([]byte(body)).
		Send()

	if sendErr != nil {
		return fmt.Errorf("kraken/public: websocket token request failed: %w", sendErr)
	}

	var payload struct {
		Error  []string    `json:"error"`
		Result types.Token `json:"result"`
	}

	if err := sonic.Unmarshal(response.Body(), &payload); err != nil {
		return fmt.Errorf("kraken/public: websocket token response invalid: %w", err)
	}

	if len(payload.Error) > 0 {
		return fmt.Errorf("kraken/public: websocket token rejected: %s", strings.Join(payload.Error, ", "))
	}

	*token = payload.Result
	return nil
}

func (rest *Rest) Error() error {
	return errnie.Error(rest.err)
}

func (rest *Rest) Close() error {
	rest.cancel()
	return errnie.Error(rest.ctx.Err())
}
