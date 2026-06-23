package private

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken/public"
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
	ctx       context.Context
	cancel    context.CancelFunc
	err       error
	tree      *dmt.Tree
	client    *http.Client
	endpoint  public.EndpointType
	apiKey    string
	apiSecret string
}

func NewRest(
	ctx context.Context,
	endpoint public.EndpointType,
	tree *dmt.Tree,
) *Rest {
	ctx, cancel := context.WithCancel(ctx)

	return &Rest{
		ctx:       ctx,
		cancel:    cancel,
		tree:      tree,
		client:    &http.Client{Timeout: 10 * time.Second},
		endpoint:  endpoint,
		apiKey:    strings.TrimSpace(os.Getenv("SYMM_KRAKEN_API_KEY")),
		apiSecret: strings.TrimSpace(os.Getenv("SYMM_KRAKEN_API_SECRET")),
	}
}

func (rest *Rest) Do(
	ctx context.Context, artifact *datura.Artifact,
) *datura.Artifact {
	request, requestErr := http.NewRequestWithContext(
		ctx,
		datura.Peek[string](artifact, "method"),
		datura.Peek[string](artifact, "destination"),
		nil,
	)

	if requestErr != nil {
		rest.err = requestErr

		return datura.Acquire(string(rest.endpoint), datura.APPJSON).WithError(
			errnie.Error(errnie.Err(
				errnie.Validation,
				"kraken/private: failed to build request",
				requestErr,
			)),
		)
	}

	for key, value := range datura.Peek[map[string]string](artifact, "headers") {
		request.Header.Set(key, value)
	}

	response, sendErr := rest.client.Do(request)

	if sendErr != nil {
		rest.err = sendErr
		statusCode := 0

		if response != nil {
			statusCode = response.StatusCode
		}

		return datura.Acquire(
			string(rest.endpoint), datura.APPJSON,
		).WithError(
			errnie.Error(errnie.Err(
				errnie.IO,
				fmt.Sprintf(
					"kraken/private: failed to get %s, error code: %d",
					string(rest.endpoint),
					statusCode,
				),
				sendErr,
			)),
		)
	}

	defer response.Body.Close()

	body, readErr := io.ReadAll(response.Body)

	if readErr != nil {
		rest.err = readErr
		return datura.Acquire(string(rest.endpoint), datura.APPJSON).WithError(readErr)
	}

	out := datura.Acquire(
		string(rest.endpoint), datura.APPJSON,
	).WithDestination(
		string(rest.endpoint),
	).WithPayload(
		body,
	)

	if rest.tree != nil {
		rest.tree.Insert(out.Prefix(), out.Marshal())
	}

	return out
}

func (rest *Rest) WebSocketToken(ctx context.Context, token *types.Token) error {
	if token == nil {
		return fmt.Errorf("kraken/private: token target is nil")
	}

	return rest.signedPost(ctx, public.EndpointWebSocketsToken, token)
}

func (rest *Rest) signedPost(
	ctx context.Context,
	endpoint public.EndpointType,
	target any,
) error {
	if rest.apiKey == "" || rest.apiSecret == "" {
		return fmt.Errorf("kraken/private: SYMM_KRAKEN_API_KEY and SYMM_KRAKEN_API_SECRET are required")
	}

	nonce := fmt.Sprintf("%d", time.Now().UnixNano())
	body := url.Values{"nonce": []string{nonce}}.Encode()
	signature, signErr := rest.sign(endpoint.SignPath(), nonce, body)

	if signErr != nil {
		return signErr
	}

	request, requestErr := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		string(endpoint),
		strings.NewReader(body),
	)

	if requestErr != nil {
		return requestErr
	}

	request.Header.Set("API-Key", rest.apiKey)
	request.Header.Set("API-Sign", signature)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	response, sendErr := rest.client.Do(request)

	if sendErr != nil {
		return sendErr
	}

	defer response.Body.Close()

	payload, readErr := io.ReadAll(response.Body)

	if readErr != nil {
		return readErr
	}

	var envelope struct {
		Error  []string        `json:"error"`
		Result json.RawMessage `json:"result"`
	}

	if err := json.Unmarshal(payload, &envelope); err != nil {
		return fmt.Errorf("kraken/private: decode token response: %w", err)
	}

	if len(envelope.Error) > 0 {
		return fmt.Errorf("kraken/private: %s", strings.Join(envelope.Error, ", "))
	}

	if len(envelope.Result) == 0 {
		return fmt.Errorf("kraken/private: empty token result")
	}

	return json.Unmarshal(envelope.Result, target)
}

func (rest *Rest) sign(path string, nonce string, body string) (string, error) {
	secret, err := base64.StdEncoding.DecodeString(rest.apiSecret)

	if err != nil {
		return "", fmt.Errorf("kraken/private: decode API secret: %w", err)
	}

	hash := sha256.Sum256([]byte(nonce + body))
	message := append([]byte(path), hash[:]...)
	signature := hmac.New(sha512.New, secret)
	signature.Write(message)

	return base64.StdEncoding.EncodeToString(signature.Sum(nil)), nil
}

func (rest *Rest) Error() error {
	return errnie.Error(rest.err)
}

func (rest *Rest) Close() error {
	rest.cancel()
	return errnie.Error(rest.ctx.Err())
}
