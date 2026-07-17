package websocket

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
)

var (
	authNonceOnce sync.Once
	authNonceFn   func() string
	authHighWater atomic.Int64
	authPersistMu sync.Mutex
	authLastWrite time.Time
)

/*
authNonce returns the process-wide Kraken REST nonce generator.

Private and Level3 Live transports share one API key. Nonces must be strictly
increasing for that key across process restarts: boot-time concurrent token
fetches can run an in-memory counter ahead of wall clock, so the next start
would otherwise reuse a lower value and Kraken rejects it with
EAPI:Invalid nonce. A persisted high-water plus an atomic increment keeps
every authenticated REST call monotonic for the key.
*/
func authNonce() func() string {
	authNonceOnce.Do(func() {
		path := noncePath()
		highWater := loadNonce(path)
		seed := time.Now().UnixNano()

		if seed <= highWater {
			seed = highWater + 1
		}

		authHighWater.Store(seed - 1)

		authNonceFn = func() string {
			next := authHighWater.Add(1)
			persistNonce(path, next)

			return strconv.FormatInt(next, 10)
		}
	})

	return authNonceFn
}

/*
bumpAuthNonce jumps the high-water mark by one second of nanoseconds after an
Invalid nonce rejection so the retry clears anything Kraken still holds, then
persists so the next process start does not repeat the collision.
*/
func bumpAuthNonce() {
	authNonce()
	next := authHighWater.Add(int64(time.Second))
	persistNonceAbsolute(noncePath(), next)
}

func noncePath() string {
	dataPath := strings.TrimSpace(viper.GetString("system.data_path"))

	if strings.HasPrefix(dataPath, "~/") {
		home, err := os.UserHomeDir()

		if err == nil {
			dataPath = filepath.Join(home, strings.TrimPrefix(dataPath, "~/"))
		}
	}

	if dataPath == "" {
		home, err := os.UserHomeDir()

		if err != nil {
			return filepath.Join(os.TempDir(), "symm-kraken-auth-nonce")
		}

		dataPath = filepath.Join(home, ".symm", "data")
	}

	return filepath.Join(dataPath, "kraken-auth-nonce")
}

func loadNonce(path string) int64 {
	body, err := os.ReadFile(path)

	if err != nil {
		return 0
	}

	value, err := strconv.ParseInt(strings.TrimSpace(string(body)), 10, 64)

	if err != nil || value < 0 {
		return 0
	}

	return value
}

func persistNonce(path string, value int64) {
	authPersistMu.Lock()
	defer authPersistMu.Unlock()

	if time.Since(authLastWrite) < 50*time.Millisecond && value%128 != 0 {
		return
	}

	authLastWrite = time.Now()
	persistNonceAbsolute(path, value)
}

func persistNonceAbsolute(path string, value int64) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		errnie.Error(errnie.Err(
			errnie.IO,
			"websocket: failed to create nonce directory",
			err,
		))

		return
	}

	if err := os.WriteFile(
		path,
		[]byte(strconv.FormatInt(value, 10)+"\n"),
		0o600,
	); err != nil {
		errnie.Error(errnie.Err(
			errnie.IO,
			"websocket: failed to persist auth nonce",
			err,
		))
	}
}
