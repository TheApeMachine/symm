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

func TestNewAuthNonce(t *testing.T) {
	Convey("Setup", t, func() {
		dir := t.TempDir()

		Convey("When NewAuthNonce constructs in a fresh directory", func() {
			nonce, err := NewAuthNonce(dir)

			Convey("construction succeeds", func() {
				So(err, ShouldBeNil)
				So(nonce, ShouldNotBeNil)
				So(nonce.Close(), ShouldBeNil)
			})
		})

		Convey("When the high-water file is corrupt", func() {
			So(os.WriteFile(filepath.Join(dir, "kraken-auth-nonce"), []byte("nope"), 0o600), ShouldBeNil)

			Convey("construction fails", func() {
				_, err := NewAuthNonce(dir)
				So(err, ShouldNotBeNil)
			})
		})

		Convey("When a lock is already held in the directory", func() {
			first, err := NewAuthNonce(dir)
			So(err, ShouldBeNil)

			Convey("a second generator is refused", func() {
				_, err := NewAuthNonce(dir)
				So(err, ShouldNotBeNil)
				So(first.Close(), ShouldBeNil)
			})
		})
	})
}

func TestNext(t *testing.T) {
	Convey("Setup", t, func() {
		dir := t.TempDir()
		nonce, err := NewAuthNonce(dir)
		So(err, ShouldBeNil)
		defer nonce.Close()

		Convey("When Next is called twice under a frozen clock", func() {
			clock := nonce.highWater.Load() + 1
			nonce.clock = func() int64 { return clock }
			first, _ := strconv.ParseInt(nonce.Next(), 10, 64)
			second, _ := strconv.ParseInt(nonce.Next(), 10, 64)

			Convey("second is first+1 and both are persisted", func() {
				So(first, ShouldEqual, clock)
				So(second, ShouldEqual, first+1)
				So(nonce.Err(), ShouldBeNil)

				body, err := os.ReadFile(filepath.Join(dir, "kraken-auth-nonce"))
				So(err, ShouldBeNil)
				persisted, err := strconv.ParseInt(strings.TrimSpace(string(body)), 10, 64)
				So(err, ShouldBeNil)
				So(persisted, ShouldEqual, second)
			})

			Convey("and a restart reloads the persisted high-water", func() {
				So(nonce.Close(), ShouldBeNil)

				restarted, err := NewAuthNonce(dir)
				So(err, ShouldBeNil)
				restarted.highWater.Store(second)
				restarted.clock = func() int64 { return second }
				next, _ := strconv.ParseInt(restarted.Next(), 10, 64)
				So(next, ShouldEqual, second+1)
				So(restarted.Close(), ShouldBeNil)
			})
		})

		Convey("When the clock advances an hour", func() {
			clock := nonce.highWater.Load() + 1
			nonce.clock = func() int64 { return clock }
			first, _ := strconv.ParseInt(nonce.Next(), 10, 64)
			clock += int64(time.Hour)
			later, _ := strconv.ParseInt(nonce.Next(), 10, 64)

			Convey("later nonce follows the clock", func() {
				So(later, ShouldEqual, clock)
				So(first, ShouldEqual, clock-int64(time.Hour))
			})

			Convey("and stays increasing when the clock steps back", func() {
				clock -= int64(time.Minute)
				stalled, _ := strconv.ParseInt(nonce.Next(), 10, 64)
				So(stalled, ShouldEqual, later+1)
			})
		})

		Convey("When eight goroutines call Next concurrently", func() {
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

			Convey("no duplicate nonces appear", func() {
				So(shared.Load(), ShouldEqual, 0)
			})
		})
	})
}

func TestBump(t *testing.T) {
	Convey("Setup", t, func() {
		dir := t.TempDir()
		nonce, err := NewAuthNonce(dir)
		So(err, ShouldBeNil)
		defer nonce.Close()

		hw := nonce.highWater.Load()
		clock := hw
		nonce.clock = func() int64 { return clock }
		first, _ := strconv.ParseInt(nonce.Next(), 10, 64)
		floor := first + 1000
		nonce.Bump(floor)
		after, _ := strconv.ParseInt(nonce.Next(), 10, 64)

		Convey("When Bump raises the floor above the last nonce", func() {
			Convey("Next is floor+2 after Bump stores floor+1", func() {
				So(first, ShouldEqual, hw+1)
				So(after, ShouldEqual, floor+2)
			})
		})
	})
}

func TestErr(t *testing.T) {
	Convey("Setup", t, func() {
		dir := t.TempDir()
		nonce, err := NewAuthNonce(dir)
		So(err, ShouldBeNil)
		defer nonce.Close()

		Convey("When persist fails on an invalid path", func() {
			blocker := filepath.Join(dir, "blocker")
			So(os.WriteFile(blocker, []byte("x"), 0o600), ShouldBeNil)
			nonce.path = filepath.Join(blocker, "kraken-auth-nonce")
			nonce.lastPersistNs.Store(0)
			nonce.Next()

			Convey("Err is sticky", func() {
				So(nonce.Err(), ShouldNotBeNil)
				nonce.path = filepath.Join(dir, "kraken-auth-nonce")
				nonce.lastPersistNs.Store(0)
				nonce.Next()
				So(nonce.Err(), ShouldNotBeNil)
			})
		})
	})
}

func BenchmarkNewAuthNonce(b *testing.B) {
	base := b.TempDir()

	b.ReportAllocs()
	b.ResetTimer()

	n := 0
	for b.Loop() {
		dir := filepath.Join(base, strconv.Itoa(n))
		n++
		_ = os.Mkdir(dir, 0o755)
		nonce, err := NewAuthNonce(dir)
		if err == nil {
			_ = nonce.Close()
		}
	}
}

func BenchmarkNext(b *testing.B) {
	dir := b.TempDir()
	nonce, err := NewAuthNonce(dir)
	if err != nil {
		b.Fatal(err)
	}
	defer nonce.Close()

	clock := nonce.highWater.Load() + 1
	nonce.clock = func() int64 { return clock }

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		_ = nonce.Next()
	}
}

func BenchmarkBump(b *testing.B) {
	dir := b.TempDir()
	nonce, err := NewAuthNonce(dir)
	if err != nil {
		b.Fatal(err)
	}
	defer nonce.Close()

	hw := nonce.highWater.Load()
	nonce.clock = func() int64 { return hw }

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		nonce.Bump(hw + 1000)
		_ = nonce.Next()
		hw += 1002
	}
}

func BenchmarkErr(b *testing.B) {
	dir := b.TempDir()
	nonce, err := NewAuthNonce(dir)
	if err != nil {
		b.Fatal(err)
	}
	defer nonce.Close()

	blocker := filepath.Join(dir, "blocker")
	_ = os.WriteFile(blocker, []byte("x"), 0o600)
	nonce.path = filepath.Join(blocker, "kraken-auth-nonce")
	nonce.lastPersistNs.Store(0)
	_ = nonce.Next()

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		_ = nonce.Err()
	}
}
