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

	"github.com/gofiber/fiber/v3"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken/public"
)

/*
Rest is the Kraken private REST API client for authenticated endpoints.
*/
type Rest struct {
	ctx      context.Context
	cancel   context.CancelFunc
	err      error
	client   *public.Rest
	endpoint EndpointType
	apiKey   string
	secret   []byte
	nonce    atomic.Uint64
}

/*
NewRest builds a private REST client bound to one endpoint.
*/
func NewRest(
	ctx context.Context,
	apiKey, apiSecret string,
	endpoint EndpointType,
) (*Rest, error) {
	if strings.TrimSpace(apiKey) == "" || strings.TrimSpace(apiSecret) == "" {
		return nil, fmt.Errorf("kraken api key and secret are required")
	}

	secret, err := base64.StdEncoding.DecodeString(apiSecret)

	if err != nil {
		return nil, fmt.Errorf("decode kraken api secret: %w", err)
	}

	ctx, cancel := context.WithCancel(ctx)

	return &Rest{
		ctx:      ctx,
		cancel:   cancel,
		client:   public.NewRest(ctx, public.EndpointType(endpoint)),
		endpoint: endpoint,
		apiKey:   apiKey,
		secret:   secret,
	}, nil
}

/*
Post sends one signed private REST request with a JSON body.
*/
func (rest *Rest) Post(ctx context.Context, request fiber.Map, model any) error {
	return errnie.Error(rest.client.Post(ctx, request, model, map[string]string{
		"X-API-Key":    rest.apiKey,
		"X-API-Secret": base64.StdEncoding.EncodeToString(rest.secret),
	}))
}

/*
WebSocketToken returns a short-lived token for the authenticated WebSocket v2 API.
*/
func (rest *Rest) WebSocketToken(ctx context.Context) (token string, expires time.Duration, err error) {
	var result struct {
		Token   string `json:"token"`
		Expires int    `json:"expires"`
	}

	tokenRest := rest

	if rest.endpoint != EndpointWebSocketsToken {
		tokenRest = rest.ForEndpoint(EndpointWebSocketsToken)
	}

	if err := tokenRest.Post(ctx, fiber.Map{}, &result); err != nil {
		return "", 0, err
	}

	if result.Token == "" {
		return "", 0, fmt.Errorf("kraken: empty websockets token")
	}

	expires = time.Duration(result.Expires) * time.Second

	if expires <= 0 {
		expires = 15 * time.Minute
	}

	return result.Token, expires, nil
}

func (rest *Rest) Error() error {
	return errnie.Error(rest.err)
}

func (rest *Rest) Close() error {
	rest.cancel()

	return errnie.Error(rest.ctx.Err())
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
