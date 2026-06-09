package private

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"github.com/bytedance/sonic"
	"github.com/gofiber/fiber/v3"
	"github.com/theapemachine/symm/kraken/public"
	"github.com/theapemachine/symm/kraken/types"
)

/*
Rest adds Kraken private API signing on top of public.Rest.
*/
type Rest struct {
	ctx      context.Context
	client   public.RestClient
	endpoint public.EndpointType
	apiKey   string
	secret   []byte
	nonce    atomic.Uint64
}

/*
NewRest builds a signed client for one private Kraken endpoint.
*/
func NewRest(
	ctx context.Context,
	apiKey, apiSecret string,
	endpoint public.EndpointType,
) (*Rest, error) {
	if strings.TrimSpace(apiKey) == "" || strings.TrimSpace(apiSecret) == "" {
		return nil, fmt.Errorf("kraken api key and secret are required")
	}

	secret, err := base64.StdEncoding.DecodeString(apiSecret)

	if err != nil {
		return nil, fmt.Errorf("decode kraken api secret: %w", err)
	}

	return &Rest{
		ctx:      ctx,
		client:   public.NewRest(ctx, endpoint),
		endpoint: endpoint,
		apiKey:   apiKey,
		secret:   secret,
	}, nil
}

/*
ForEndpoint returns a client with the same credentials on another endpoint.
*/
func (rest *Rest) ForEndpoint(endpoint public.EndpointType) (*Rest, error) {
	return &Rest{
		ctx:      rest.ctx,
		client:   public.NewRest(rest.ctx, endpoint),
		endpoint: endpoint,
		apiKey:   rest.apiKey,
		secret:   rest.secret,
	}, nil
}

/*
Get is not supported on private REST endpoints.
*/
func (rest *Rest) Get(
	ctx context.Context,
	request fiber.Map,
	model any,
	headers ...map[string]string,
) error {
	return fmt.Errorf("kraken private rest: GET not supported")
}

/*
Post sends one signed private REST request through the public client.
*/
func (rest *Rest) Post(
	ctx context.Context,
	request fiber.Map,
	model any,
	headers ...map[string]string,
) error {
	nonce := rest.nextNonce()
	request["nonce"] = nonce

	body, err := sonic.Marshal(request)

	if err != nil {
		return fmt.Errorf("kraken private encode: %w", err)
	}

	signature, err := rest.sign(rest.endpoint.SignPath(), nonce, string(body))

	if err != nil {
		return err
	}

	return rest.client.Post(ctx, request, model, map[string]string{
		"API-Key":  rest.apiKey,
		"API-Sign": signature,
	})
}

func (rest *Rest) WebSocketToken(ctx context.Context, token *types.Token) error {
	return rest.Post(ctx, fiber.Map{}, token)
}

func (rest *Rest) Error() error {
	return rest.client.Error()
}

func (rest *Rest) Close() error {
	return rest.client.Close()
}

func (rest *Rest) nextNonce() string {
	sequence := rest.nonce.Add(1)

	return fmt.Sprintf("%d", time.Now().UnixNano()+int64(sequence))
}

func (rest *Rest) sign(path, nonce, body string) (string, error) {
	sha := sha256.New()
	sha.Write([]byte(nonce + body))
	digest := sha.Sum(nil)

	mac := hmac.New(sha512.New, rest.secret)
	mac.Write([]byte(path))
	mac.Write(digest)

	return base64.StdEncoding.EncodeToString(mac.Sum(nil)), nil
}
