package public

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/bytedance/sonic"
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
	).SetContext(
		ctx,
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

/*
LoadAssetPairs fetches Kraken's public AssetPairs catalog and stores the maker /
taker fee schedules in the tree before trading starts.
*/
func (rest *Rest) LoadAssetPairs(ctx context.Context) (int, error) {
	if rest == nil || rest.tree == nil {
		return 0, errnie.Error(errnie.Err(
			errnie.Validation, "kraken/public: AssetPairs loader missing tree", nil,
		))
	}

	response, sendErr := rest.client.R().
		SetContext(ctx).
		SetMethod("GET").
		SetURL(string(EndpointTypeAssetPairs)).
		Send()

	if sendErr != nil {
		rest.err = sendErr
		return 0, errnie.Error(errnie.Err(
			errnie.IO,
			"kraken/public: failed to load AssetPairs",
			sendErr,
		))
	}

	if response == nil {
		return 0, errnie.Error(errnie.Err(
			errnie.IO, "kraken/public: AssetPairs returned no response", nil,
		))
	}

	if status := response.StatusCode(); status < 200 || status >= 300 {
		return 0, errnie.Error(errnie.Err(
			errnie.IO,
			fmt.Sprintf("kraken/public: AssetPairs returned HTTP %d", status),
			nil,
		))
	}

	return rest.ingestAssetPairs(response.Body())
}

func (rest *Rest) ingestAssetPairs(body []byte) (int, error) {
	var envelope struct {
		Error  []string                   `json:"error"`
		Result map[string]json.RawMessage `json:"result"`
	}

	if err := sonic.Unmarshal(body, &envelope); err != nil {
		return 0, errnie.Error(errnie.Err(
			errnie.Validation, "kraken/public: AssetPairs payload is malformed", err,
		))
	}

	if len(envelope.Error) > 0 {
		return 0, errnie.Error(errnie.Err(
			errnie.Validation,
			"kraken/public: AssetPairs returned "+strings.Join(envelope.Error, "; "),
			nil,
		))
	}

	if len(envelope.Result) == 0 {
		return 0, errnie.Error(errnie.Err(
			errnie.Validation, "kraken/public: AssetPairs result is empty", nil,
		))
	}

	count := 0

	for _, raw := range envelope.Result {
		var meta struct {
			WSName string `json:"wsname"`
			Status string `json:"status"`
		}

		if err := sonic.Unmarshal(raw, &meta); err != nil {
			continue
		}

		if meta.WSName == "" || (meta.Status != "" && meta.Status != "online") {
			continue
		}

		if !symbolMatchesQuoteCurrency(meta.WSName) {
			continue
		}

		for _, scope := range assetPairScopes(meta.WSName) {
			artifact := datura.Acquire("kraken:public", datura.APPJSON).
				WithRole("assetpairs").
				WithScope(scope).
				WithPayload(raw)

			rest.tree.InsertArtifact(artifact.Prefix(), artifact)
			count++
		}
	}

	if count == 0 {
		return 0, errnie.Error(errnie.Err(
			errnie.Validation, "kraken/public: AssetPairs contained no tradable schedules", nil,
		))
	}

	return count, nil
}

func assetPairScopes(wsname string) []string {
	scopes := []string{wsname}
	alias := strings.NewReplacer(
		"XBT/", "BTC/",
		"/XBT", "/BTC",
		"XDG/", "DOGE/",
		"/XDG", "/DOGE",
	).Replace(wsname)

	if alias != wsname {
		scopes = append(scopes, alias)
	}

	return scopes
}

func (rest *Rest) Error() error {
	return errnie.Error(rest.err)
}

func (rest *Rest) Close() error {
	rest.cancel()
	return errnie.Error(rest.ctx.Err())
}
