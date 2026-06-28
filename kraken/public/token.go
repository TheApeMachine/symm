package public

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"net/url"
	"os"
	"strconv"
	"time"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken/types"
)

type Token struct {
	active    bool
	current   string
	apiKey    string
	apiSecret string
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
			return types.NewToken(ctx)
		}).Value()
	}

	return out
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
