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
	processNonce     *AuthNonce
	processNonceOnce sync.Once
	processNonceErr  error
)

/*
AuthNonce issues strictly increasing Kraken REST nonces backed by a storage
path. Private and Level3 transports share one process instance so concurrent
token fetches cannot collide; tests construct isolated generators per TempDir.
*/
type AuthNonce struct {
	path      string
	highWater atomic.Int64
	persistMu sync.Mutex
	lastWrite time.Time
}

/*
NewAuthNonce seeds a generator from pathDir/kraken-auth-nonce. A missing file
is the valid zero state; other read or parse failures abort construction so
authentication cannot proceed on corrupt high-water state.
*/
func NewAuthNonce(pathDir string) (*AuthNonce, error) {
	path := filepath.Join(pathDir, "kraken-auth-nonce")
	highWater, err := loadNonce(path)

	if err != nil {
		return nil, err
	}

	seed := time.Now().UnixNano()

	if seed <= highWater {
		seed = highWater + 1
	}

	nonce := &AuthNonce{path: path}
	nonce.highWater.Store(seed - 1)

	return nonce, nil
}

/*
Next returns the next monotonic nonce string and persists the high-water mark.
*/
func (nonce *AuthNonce) Next() string {
	nonce.persistMu.Lock()
	defer nonce.persistMu.Unlock()

	next := nonce.highWater.Load() + 1
	nonce.highWater.Store(next)
	nonce.persistValue(next, false)

	return strconv.FormatInt(next, 10)
}

/*
Bump jumps the high-water by one second of nanoseconds after an Invalid nonce
rejection so the retry clears anything Kraken still holds, then persists.
*/
func (nonce *AuthNonce) Bump() {
	nonce.persistMu.Lock()
	defer nonce.persistMu.Unlock()

	next := nonce.highWater.Load() + int64(time.Second)
	nonce.highWater.Store(next)
	nonce.persistValue(next, true)
}

/*
processAuthNonce returns the process-wide generator shared by authenticated
Live transports. Construction errors prevent REST Nonce wiring and auth.
*/
func processAuthNonce() (*AuthNonce, error) {
	processNonceOnce.Do(func() {
		processNonce, processNonceErr = NewAuthNonce(nonceDir())
	})

	return processNonce, processNonceErr
}

func nonceDir() string {
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
			return os.TempDir()
		}

		dataPath = filepath.Join(home, ".symm", "data")
	}

	return dataPath
}

/*
loadNonce reads a persisted high-water. Missing files yield zero; other IO,
parse, negative, or overflow failures return a descriptive error.
*/
func loadNonce(path string) (int64, error) {
	body, err := os.ReadFile(path)

	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}

		return 0, errnie.Error(errnie.Err(
			errnie.IO,
			"websocket: failed to read auth nonce",
			err,
		))
	}

	value, err := strconv.ParseInt(strings.TrimSpace(string(body)), 10, 64)

	if err != nil {
		return 0, errnie.Error(errnie.Err(
			errnie.Validation,
			"websocket: invalid auth nonce value",
			err,
		))
	}

	if value < 0 {
		return 0, errnie.Error(errnie.Err(
			errnie.Validation,
			"websocket: auth nonce must not be negative",
			nil,
		))
	}

	return value, nil
}

func (nonce *AuthNonce) persistValue(value int64, force bool) {
	if !force && time.Since(nonce.lastWrite) < 50*time.Millisecond && value%128 != 0 {
		return
	}

	nonce.lastWrite = time.Now()
	nonce.writeAtomic(value)
}

/*
writeAtomic writes and syncs the nonce to a temporary file, then renames it into
place so a crash cannot leave a truncated high-water file.
*/
func (nonce *AuthNonce) writeAtomic(value int64) {
	dir := filepath.Dir(nonce.path)

	if err := os.MkdirAll(dir, 0o700); err != nil {
		errnie.Error(errnie.Err(
			errnie.IO,
			"websocket: failed to create nonce directory",
			err,
		))

		return
	}

	temporary, err := os.CreateTemp(dir, "kraken-auth-nonce-*.tmp")

	if err != nil {
		errnie.Error(errnie.Err(
			errnie.IO,
			"websocket: failed to create nonce temp file",
			err,
		))

		return
	}

	temporaryPath := temporary.Name()
	payload := []byte(strconv.FormatInt(value, 10) + "\n")

	if _, err := temporary.Write(payload); err != nil {
		temporary.Close()
		os.Remove(temporaryPath)
		errnie.Error(errnie.Err(
			errnie.IO,
			"websocket: failed to write auth nonce",
			err,
		))

		return
	}

	if err := temporary.Sync(); err != nil {
		temporary.Close()
		os.Remove(temporaryPath)
		errnie.Error(errnie.Err(
			errnie.IO,
			"websocket: failed to sync auth nonce",
			err,
		))

		return
	}

	if err := temporary.Close(); err != nil {
		os.Remove(temporaryPath)
		errnie.Error(errnie.Err(
			errnie.IO,
			"websocket: failed to close auth nonce temp file",
			err,
		))

		return
	}

	if err := os.Rename(temporaryPath, nonce.path); err != nil {
		os.Remove(temporaryPath)
		errnie.Error(errnie.Err(
			errnie.IO,
			"websocket: failed to persist auth nonce",
			err,
		))
	}
}
