package private

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/gofiber/fiber/v3/client"
	"github.com/theapemachine/symm/replay"
)

const baseURL = "https://api.kraken.com"

/*
Rest is the Kraken private REST API client for authenticated endpoints.
*/
type Rest struct {
	apiKey     string
	secret     []byte
	httpClient *client.Client
	nonce      atomic.Uint64
}

/*
NewRest builds a private REST client from API key and base64-encoded secret.
*/
func NewRest(apiKey, apiSecret string) (*Rest, error) {
	if strings.TrimSpace(apiKey) == "" || strings.TrimSpace(apiSecret) == "" {
		return nil, fmt.Errorf("kraken api key and secret are required")
	}

	secret, err := base64.StdEncoding.DecodeString(apiSecret)

	if err != nil {
		return nil, fmt.Errorf("decode kraken api secret: %w", err)
	}

	return &Rest{
		apiKey:     apiKey,
		secret:     secret,
		httpClient: client.New(),
	}, nil
}

/*
WebSocketToken returns a short-lived token for the authenticated WebSocket v2 API.
*/
func (rest *Rest) WebSocketToken(ctx context.Context) (token string, expires time.Duration, err error) {
	var response struct {
		Error  []string `json:"error"`
		Result struct {
			Token   string `json:"token"`
			Expires int    `json:"expires"`
		} `json:"result"`
	}

	const path = "/0/private/GetWebSocketsToken"

	if err := rest.post(ctx, path, url.Values{}, &response); err != nil {
		return "", 0, err
	}

	if len(response.Error) > 0 {
		return "", 0, fmt.Errorf("kraken: %s", strings.Join(response.Error, ", "))
	}

	if response.Result.Token == "" {
		return "", 0, fmt.Errorf("kraken: empty websockets token")
	}

	expires = time.Duration(response.Result.Expires) * time.Second

	if expires <= 0 {
		expires = 15 * time.Minute
	}

	return response.Result.Token, expires, nil
}

func (rest *Rest) post(ctx context.Context, path string, form url.Values, model any) error {
	if form == nil {
		form = url.Values{}
	}

	nonce := rest.nextNonce()
	form.Set("nonce", nonce)
	body := form.Encode()
	signature, err := rest.sign(path, nonce, body)

	if err != nil {
		return err
	}

	response, err := rest.httpClient.Post(
		baseURL+path,
		client.Config{
			Ctx:     ctx,
			Timeout: 10 * time.Second,
			Header: map[string]string{
				"API-Key":      rest.apiKey,
				"API-Sign":     signature,
				"Content-Type": "application/x-www-form-urlencoded",
			},
			Body: []byte(body),
		},
	)

	if err != nil {
		return fmt.Errorf("kraken private post %s: %w", path, err)
	}

	defer response.Close()

	responseBody := response.Body()
	_ = replay.WriteREST(strings.TrimPrefix(path, "/0/private/"), responseBody)

	if err := json.Unmarshal(responseBody, model); err != nil {
		return fmt.Errorf("kraken private decode %s: %w", path, err)
	}

	return nil
}

func (rest *Rest) nextNonce() string {
	sequence := rest.nonce.Add(1)

	return strconv.FormatInt(time.Now().UnixNano()+int64(sequence), 10)
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
