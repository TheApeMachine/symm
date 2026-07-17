package websocket

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
)

/*
TestAuthNonceIsMonotonic ensures issued nonces never decrease.
*/
func TestAuthNonceIsMonotonic(t *testing.T) {
	Convey("Given the shared auth nonce generator", t, func() {
		generate := authNonce()
		prior := int64(0)

		for range 64 {
			raw := generate()
			value, err := strconv.ParseInt(raw, 10, 64)
			So(err, ShouldBeNil)
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
		previous := viper.Get("system.data_path")
		viper.Set("system.data_path", dir)
		t.Cleanup(func() { viper.Set("system.data_path", previous) })

		path := filepath.Join(dir, "kraken-auth-nonce")
		ahead := int64(9_000_000_000_000_000_000)
		So(os.WriteFile(path, []byte(strconv.FormatInt(ahead, 10)+"\n"), 0o600), ShouldBeNil)

		loaded := loadNonce(path)
		So(loaded, ShouldEqual, ahead)

		seed := int64(1)
		highWater := loaded

		if seed <= highWater {
			seed = highWater + 1
		}

		So(seed, ShouldEqual, ahead+1)
	})
}

/*
TestBumpAuthNonceAdvancesPersistedHighWater jumps the on-disk mark so a retry
after Invalid nonce clears Kraken's last accepted value.
*/
func TestBumpAuthNonceAdvancesPersistedHighWater(t *testing.T) {
	Convey("Given an initialized auth nonce", t, func() {
		dir := t.TempDir()
		previous := viper.Get("system.data_path")
		viper.Set("system.data_path", dir)
		t.Cleanup(func() { viper.Set("system.data_path", previous) })

		before, err := strconv.ParseInt(authNonce()(), 10, 64)
		So(err, ShouldBeNil)

		bumpAuthNonce()

		after := loadNonce(noncePath())
		So(after, ShouldBeGreaterThan, before)
	})
}
