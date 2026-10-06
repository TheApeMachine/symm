package kraken

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
	path          string
	highWater     atomic.Int64
	isPersisting  atomic.Bool
	lastPersistNs atomic.Int64
	// persistErr holds the first high-water persist failure. It is sticky:
	// once the on-disk high-water cannot be trusted, NewAuthenticatedREST
	// refuses to sign new clients instead of continuing on unpersisted state.
	persistErr atomic.Pointer[error]
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
	next := nonce.highWater.Add(1)
	nonce.persistValue(next, false)

	return strconv.FormatInt(next, 10)
}

/*
Err reports the first high-water persist failure, or nil.
*/
func (nonce *AuthNonce) Err() error {
	if nonce == nil {
		return nil
	}

	if ptr := nonce.persistErr.Load(); ptr != nil {
		return *ptr
	}

	return nil
}

/*
processAuthNonce returns the process-wide generator shared by authenticated
Live transports. Construction errors prevent REST Nonce wiring and auth.
*/
func ProcessAuthNonce() (*AuthNonce, error) {
	processNonceOnce.Do(func() {
		dir, err := nonceDir()

		if err != nil {
			processNonceErr = err
			return
		}

		processNonce, processNonceErr = NewAuthNonce(dir)
	})

	return processNonce, processNonceErr
}

/*
nonceDir resolves the directory holding the persisted nonce high-water. An
unresolvable home directory is an error, never a fall back to the temp dir:
a high-water written somewhere that does not survive restarts is not one.
*/
func nonceDir() (string, error) {
	dataPath := strings.TrimSpace(viper.GetString("system.data_path"))

	if dataPath != "" && !strings.HasPrefix(dataPath, "~/") {
		return dataPath, nil
	}

	home, err := os.UserHomeDir()

	if err != nil {
		return "", errnie.Error(errnie.Err(
			errnie.IO,
			"[kraken.nonce] home directory unavailable for auth nonce",
			err,
		))
	}

	if dataPath == "" {
		return filepath.Join(home, ".symm", "data"), nil
	}

	return filepath.Join(home, strings.TrimPrefix(dataPath, "~/")), nil
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
	now := time.Now().UnixNano()

	if !force && now-nonce.lastPersistNs.Load() < int64(50*time.Millisecond) && value%128 != 0 {
		return
	}

	if !nonce.isPersisting.CompareAndSwap(false, true) {
		if !force {
			return
		}

		for !nonce.isPersisting.CompareAndSwap(false, true) {
		}
	}
	defer nonce.isPersisting.Store(false)

	nonce.lastPersistNs.Store(now)

	if err := nonce.writeAtomic(value); err != nil {
		nonce.persistErr.CompareAndSwap(nil, &err)
	}
}

/*
writeAtomic writes and syncs the nonce to a temporary file, then renames it into
place so a crash cannot leave a truncated high-water file.
*/
func (nonce *AuthNonce) writeAtomic(value int64) error {
	dir := filepath.Dir(nonce.path)

	if err := os.MkdirAll(dir, 0o700); err != nil {
		return errnie.Error(errnie.Err(
			errnie.IO,
			"[kraken.nonce] failed to create nonce directory",
			err,
		))
	}

	temporary, err := os.CreateTemp(dir, "kraken-auth-nonce-*.tmp")

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.IO,
			"[kraken.nonce] failed to create nonce temp file",
			err,
		))
	}

	temporaryPath := temporary.Name()
	payload := []byte(strconv.FormatInt(value, 10) + "\n")

	if _, err := temporary.Write(payload); err != nil {
		temporary.Close()
		os.Remove(temporaryPath)

		return errnie.Error(errnie.Err(
			errnie.IO,
			"[kraken.nonce] failed to write auth nonce",
			err,
		))
	}

	if err := temporary.Sync(); err != nil {
		temporary.Close()
		os.Remove(temporaryPath)

		return errnie.Error(errnie.Err(
			errnie.IO,
			"[kraken.nonce] failed to sync auth nonce",
			err,
		))
	}

	if err := temporary.Close(); err != nil {
		os.Remove(temporaryPath)

		return errnie.Error(errnie.Err(
			errnie.IO,
			"[kraken.nonce] failed to close auth nonce temp file",
			err,
		))
	}

	if err := os.Rename(temporaryPath, nonce.path); err != nil {
		os.Remove(temporaryPath)

		return errnie.Error(errnie.Err(
			errnie.IO,
			"[kraken.nonce] failed to persist auth nonce",
			err,
		))
	}

	return nil
}
