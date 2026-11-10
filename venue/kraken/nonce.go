package kraken

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"golang.org/x/sys/unix"
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
	lock          *os.File
	highWater     atomic.Int64
	isPersisting  atomic.Bool
	lastPersistNs atomic.Int64
	// persistErr holds the first high-water persist failure. It is sticky:
	// once the on-disk high-water cannot be trusted, NewAuthenticatedREST
	// refuses to sign new clients instead of continuing on unpersisted state.
	persistErr atomic.Pointer[error]
	// clock is the wall clock in nanoseconds; tests replace it.
	clock func() int64
}

/*
NewAuthNonce seeds a generator from pathDir/kraken-auth-nonce. A missing file
is the valid zero state; other read or parse failures abort construction so
authentication cannot proceed on corrupt high-water state. An exclusive lock
keeps a second process from racing the same key's nonce stream.
*/
func NewAuthNonce(pathDir string) (*AuthNonce, error) {
	if err := os.MkdirAll(pathDir, 0o700); err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.IO,
			"[kraken.nonce] failed to create nonce directory",
			err,
		))
	}

	path := filepath.Join(pathDir, "kraken-auth-nonce")
	lock, err := lockNonce(pathDir)

	if err != nil {
		return nil, err
	}

	highWater, err := loadNonce(path)

	if err != nil {
		_ = lock.Close()
		return nil, err
	}

	// Legacy path (~/.symm/kraken-auth-nonce) may hold a higher water mark
	// from older builds; take the max so a migrate cannot rewind Kraken.
	if legacy, legacyErr := loadLegacyNonce(); legacyErr == nil && legacy > highWater {
		highWater = legacy
	}

	seed := time.Now().UnixNano()

	if seed <= highWater {
		seed = highWater + 1
	}

	nonce := &AuthNonce{
		path:  path,
		lock:  lock,
		clock: func() int64 { return time.Now().UnixNano() },
	}
	nonce.highWater.Store(seed - 1)

	// Persist the seed immediately so a crash before the first Next cannot
	// restart behind a token fetch that already left the process.
	nonce.persistValue(seed-1, true)

	return nonce, nil
}

/*
Next returns the next nonce, max(now, last+1) in nanoseconds, and persists the
high-water mark. Tracking the clock, not only counting up from the seed, keeps
a long-running process ahead of any other process that signed with the same
API key since it started: Kraken accepts a nonce only above the last one it
saw for the key, whichever process sent it.
*/
func (nonce *AuthNonce) Next() string {
	for {
		last := nonce.highWater.Load()
		next := max(nonce.clock(), last+1)

		if nonce.highWater.CompareAndSwap(last, next) {
			nonce.persistValue(next, true)

			return strconv.FormatInt(next, 10)
		}
	}
}

/*
Bump raises the high-water above floor (and the clock) after Kraken rejects a
nonce, then persists. Safe under concurrent Next callers.
*/
func (nonce *AuthNonce) Bump(floor int64) {
	if nonce == nil {
		return
	}

	for {
		last := nonce.highWater.Load()
		next := max(nonce.clock(), max(last, floor)+1)

		if next <= last {
			return
		}

		if nonce.highWater.CompareAndSwap(last, next) {
			nonce.persistValue(next, true)
			return
		}
	}
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
Close releases the exclusive nonce lock. The process-wide generator keeps the
lock for the life of the process; tests call Close before constructing a
successor in the same directory.
*/
func (nonce *AuthNonce) Close() error {
	if nonce == nil || nonce.lock == nil {
		return nil
	}

	err := nonce.lock.Close()
	nonce.lock = nil
	return err
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
BumpAuthNonce advances the process generator after an Invalid nonce rejection.
*/
func BumpAuthNonce() {
	nonce, err := ProcessAuthNonce()

	if err != nil || nonce == nil {
		return
	}

	// Jump a full second of nanoseconds past both clock and high-water so a
	// foreign signer that raced ahead cannot keep rejecting us on every tick.
	nonce.Bump(nonce.clock() + int64(time.Second))
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

func loadLegacyNonce() (int64, error) {
	home, err := os.UserHomeDir()

	if err != nil {
		return 0, err
	}

	return loadNonce(filepath.Join(home, ".symm", "kraken-auth-nonce"))
}

/*
lockNonce takes an exclusive non-blocking flock so two symm processes cannot
share one API key's nonce stream (the usual Invalid nonce cause).
*/
func lockNonce(pathDir string) (*os.File, error) {
	path := filepath.Join(pathDir, "kraken-auth-nonce.lock")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.IO,
			"[kraken.nonce] failed to open auth nonce lock",
			err,
		))
	}

	if err := unix.Flock(int(file.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		_ = file.Close()
		return nil, errnie.Error(errnie.Err(
			errnie.Conflict,
			"[kraken.nonce] another process holds the auth nonce lock — only one live signer may use this key",
			err,
		))
	}

	return file, nil
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
		return errnie.Error(errnie.Err(
			errnie.IO,
			"[kraken.nonce] failed to write auth nonce",
			errors.Join(err, temporary.Close(), os.Remove(temporaryPath)),
		))
	}

	if err := temporary.Sync(); err != nil {
		return errnie.Error(errnie.Err(
			errnie.IO,
			"[kraken.nonce] failed to sync auth nonce",
			errors.Join(err, temporary.Close(), os.Remove(temporaryPath)),
		))
	}

	if err := temporary.Close(); err != nil {
		return errnie.Error(errnie.Err(
			errnie.IO,
			"[kraken.nonce] failed to close auth nonce temp file",
			errors.Join(err, os.Remove(temporaryPath)),
		))
	}

	if err := os.Rename(temporaryPath, nonce.path); err != nil {
		return errnie.Error(errnie.Err(
			errnie.IO,
			"[kraken.nonce] failed to persist auth nonce",
			errors.Join(err, os.Remove(temporaryPath)),
		))
	}

	return nil
}
