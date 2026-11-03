package kraken

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

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

		Convey("nonces follow the clock so a long-running process stays ahead of a later one", func() {
			clock := time.Now().UnixNano()
			nonce.clock = func() int64 { return clock }
			first, _ := strconv.ParseInt(nonce.Next(), 10, 64)

			// Another process with the same key signs an hour later.
			clock += int64(time.Hour)
			later, _ := strconv.ParseInt(nonce.Next(), 10, 64)
			So(later, ShouldEqual, clock)
			So(later, ShouldBeGreaterThan, first)

			Convey("and stay strictly increasing when the clock stalls or steps back", func() {
				clock -= int64(time.Minute)
				stalled, _ := strconv.ParseInt(nonce.Next(), 10, 64)
				So(stalled, ShouldEqual, later+1)
			})
		})

		Convey("concurrent callers never share a nonce", func() {
			seen := sync.Map{}
			var group sync.WaitGroup
			var shared atomic.Int32

			for range 8 {

				group.Go(func() {

					for range 500 {
						if _, loaded := seen.LoadOrStore(nonce.Next(), struct{}{}); loaded {
							shared.Add(1)
						}
					}
				})
			}

			group.Wait()
			So(shared.Load(), ShouldEqual, 0)
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
