package websocket

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

/*
TestAuthNonceIsMonotonic ensures issued nonces never decrease on a fresh
generator instance.
*/
func TestAuthNonceIsMonotonic(t *testing.T) {
	Convey("Given an isolated auth nonce generator", t, func() {
		nonce, err := NewAuthNonce(t.TempDir())
		So(err, ShouldBeNil)

		prior := int64(0)

		for range 64 {
			raw := nonce.Next()
			value, parseErr := strconv.ParseInt(raw, 10, 64)
			So(parseErr, ShouldBeNil)
			So(value, ShouldBeGreaterThan, prior)
			prior = value
		}
	})
}

/*
TestAuthNoncePersistsAcrossRestart seeds the next process from the on-disk
high-water so Kraken never sees a reused nonce after a crash or reboot.
*/
func TestAuthNoncePersistsAcrossRestart(t *testing.T) {
	Convey("Given a persisted high-water ahead of wall clock", t, func() {
		dir := t.TempDir()
		path := filepath.Join(dir, "kraken-auth-nonce")
		ahead := int64(9_000_000_000_000_000_000)
		So(os.WriteFile(path, []byte(strconv.FormatInt(ahead, 10)+"\n"), 0o600), ShouldBeNil)

		loaded, err := loadNonce(path)
		So(err, ShouldBeNil)
		So(loaded, ShouldEqual, ahead)

		nonce, err := NewAuthNonce(dir)
		So(err, ShouldBeNil)

		next, err := strconv.ParseInt(nonce.Next(), 10, 64)
		So(err, ShouldBeNil)
		So(next, ShouldEqual, ahead+1)
	})
}

/*
TestBumpAuthNonceAdvancesPersistedHighWater jumps the on-disk mark so a retry
after Invalid nonce clears Kraken's last accepted value.
*/
func TestBumpAuthNonceAdvancesPersistedHighWater(t *testing.T) {
	Convey("Given an initialized auth nonce", t, func() {
		dir := t.TempDir()
		nonce, err := NewAuthNonce(dir)
		So(err, ShouldBeNil)

		before, err := strconv.ParseInt(nonce.Next(), 10, 64)
		So(err, ShouldBeNil)

		nonce.Bump()

		after, err := loadNonce(filepath.Join(dir, "kraken-auth-nonce"))
		So(err, ShouldBeNil)
		So(after, ShouldBeGreaterThan, before)
	})
}

/*
TestLoadNonceRejectsCorruptValues ensures invalid persisted high-water cannot
silently reset authentication to zero.
*/
func TestLoadNonceRejectsCorruptValues(t *testing.T) {
	Convey("Given a non-numeric nonce file", t, func() {
		dir := t.TempDir()
		path := filepath.Join(dir, "kraken-auth-nonce")
		So(os.WriteFile(path, []byte("not-a-nonce\n"), 0o600), ShouldBeNil)

		_, err := loadNonce(path)
		So(err, ShouldNotBeNil)

		_, err = NewAuthNonce(dir)
		So(err, ShouldNotBeNil)
	})
}
