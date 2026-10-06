package kraken

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestAuthNonce(t *testing.T) {
	Convey("Given an AuthNonce in a temp dir", t, func() {
		dir := t.TempDir()
		nonce, err := NewAuthNonce(dir)
		So(err, ShouldBeNil)

		Convey("nonces are strictly increasing and persisted", func() {
			first, _ := strconv.ParseInt(nonce.Next(), 10, 64)
			second, _ := strconv.ParseInt(nonce.Next(), 10, 64)
			So(second, ShouldBeGreaterThan, first)
			So(nonce.Err(), ShouldBeNil)

			body, err := os.ReadFile(filepath.Join(dir, "kraken-auth-nonce"))
			So(err, ShouldBeNil)
			persisted, err := strconv.ParseInt(strings.TrimSpace(string(body)), 10, 64)
			So(err, ShouldBeNil)
			So(persisted, ShouldBeGreaterThanOrEqualTo, first)

			Convey("and a restart seeds above the persisted high-water", func() {
				restarted, err := NewAuthNonce(dir)
				So(err, ShouldBeNil)
				next, _ := strconv.ParseInt(restarted.Next(), 10, 64)
				So(next, ShouldBeGreaterThan, persisted)
			})
		})

		Convey("a persist failure is recorded and sticky", func() {
			blocker := filepath.Join(dir, "blocker")
			So(os.WriteFile(blocker, []byte("x"), 0o600), ShouldBeNil)
			nonce.path = filepath.Join(blocker, "kraken-auth-nonce")
			nonce.lastPersistNs.Store(0)

			nonce.Next()
			So(nonce.Err(), ShouldNotBeNil)

			nonce.path = filepath.Join(dir, "kraken-auth-nonce")
			nonce.lastPersistNs.Store(0)
			nonce.Next()
			So(nonce.Err(), ShouldNotBeNil)
		})
	})

	Convey("Given a corrupt high-water file", t, func() {
		dir := t.TempDir()
		So(os.WriteFile(filepath.Join(dir, "kraken-auth-nonce"), []byte("nope"), 0o600), ShouldBeNil)

		Convey("construction fails", func() {
			_, err := NewAuthNonce(dir)
			So(err, ShouldNotBeNil)
		})
	})
}
