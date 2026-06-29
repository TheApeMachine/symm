package public

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"errors"
	"net/url"
	"os"
	"strconv"
	"time"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
)

type Token struct {
	active    bool
	current   string
	apiKey    string
	apiSecret string
}

type TokenRest interface {
	WebSocketToken(context.Context, *WebSocketToken) error
}

type WebSocketToken struct {
	Token   string `json:"token"`
	Expires int    `json:"expires"`
}

var tokenRest TokenRest

func BindTokenRest(rest TokenRest) {
	tokenRest = rest
}

func NewToken(
	ctx context.Context, destination string,
) *Token {
	out := &Token{
		active:    destination == "kraken:private",
		apiKey:    os.Getenv("SYMM_KRAKEN_API_KEY"),
		apiSecret: os.Getenv("SYMM_KRAKEN_API_SECRET"),
	}

	if out.active {
		out.current = errnie.Does(func() (string, error) {
			return newWebSocketToken(ctx)
		}).Value()
	}

	return out
}

func newWebSocketToken(ctx context.Context) (string, error) {
	if tokenRest == nil {
		return "", errnie.Error(errnie.Err(
			errnie.Validation,
			"kraken/public: websocket token rest not bound",
			errors.New("token rest not bound"),
		))
	}

	token := &WebSocketToken{}

	if err := tokenRest.WebSocketToken(ctx, token); err != nil {
		return "", errnie.Error(errnie.Err(
			errnie.IO,
			"kraken/public: websocket token request failed",
			err,
		))
	}

	if token.Token == "" {
		return "", errnie.Error(errnie.Err(
			errnie.Validation,
			"kraken/public: empty websocket token",
			errors.New("empty websocket token"),
		))
	}

	return token.Token, nil
}

func (token *Token) Wrap(artifact *datura.Artifact) []byte {
	if token.active {
		artifact.PokePayload(token.current, "params", "token")
	}

	return artifact.DecryptPayload()
}

func (token *Token) Header(
	artifact *datura.Artifact,
) *datura.Artifact {
	if !token.active {
		return artifact
	}

	nonce := strconv.FormatInt(time.Now().UnixNano(), 10)
	body := url.Values{"nonce": []string{nonce}}.Encode()

	signature := errnie.Does(func() (string, error) {
		return token.sign("/0/private", nonce, body)
	}).Or(func(err error) {
		errnie.Error(errnie.Err(
			errnie.Unauthorized,
			err.Error(),
			err,
		))
	}).Value()

	return artifact.Poke(
		token.apiKey, "headers", "API-Key",
	).Poke(
		signature, "headers", "API-Sign",
	).Poke(
		"application/x-www-form-urlencoded",
		"headers", "Content-Type",
	)
}

func (token *Token) sign(
	path string, nonce string, body string,
) (string, error) {
	secret := errnie.Does(func() ([]byte, error) {
		return base64.StdEncoding.DecodeString(
			token.apiSecret,
		)
	}).Or(func(err error) {
		errnie.Error(errnie.Err(
			errnie.Unauthorized,
			err.Error(),
			err,
		))
	}).Value()

	hash := sha256.Sum256([]byte(nonce + body))
	message := append([]byte(path), hash[:]...)
	signature := hmac.New(sha512.New, secret)
	signature.Write(message)

	return base64.StdEncoding.EncodeToString(
		signature.Sum(nil),
	), nil
}
