package kraken

import (
	"context"
	"errors"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestNewAuth(t *testing.T) {
	Convey("Setup", t, func() {
		Convey("When NewAuth is called", func() {
			auth := NewAuth()

			Convey("clock and fetch are wired for production", func() {
				So(auth, ShouldNotBeNil)
				So(auth.now, ShouldNotBeNil)
				So(auth.fetch, ShouldNotBeNil)
			})
		})
	})
}

func TestSeedToken(t *testing.T) {
	Convey("Setup", t, func() {
		auth := NewAuth()
		issued := time.Unix(1_700_000_000, 0)
		auth.now = func() time.Time { return issued }

		Convey("When SeedToken pins a young token", func() {
			auth.SeedToken("pinned-token")
			token, err := auth.Token(t.Context())

			Convey("Token returns the pinned value without fetching", func() {
				So(err, ShouldBeNil)
				So(token, ShouldEqual, "pinned-token")
			})
		})

		Convey("When SeedToken is called with an empty token", func() {
			auth.SeedToken("")

			Convey("the cache stays empty", func() {
				So(auth.token, ShouldBeEmpty)
			})
		})
	})
}

func TestToken(t *testing.T) {
	Convey("Setup", t, func() {
		clock := time.Unix(1_700_000_000, 0)
		fetches := 0
		auth := &Auth{
			now: func() time.Time { return clock },
			fetch: func() (string, error) {
				fetches++
				return "token-" + string(rune('A'+fetches-1)), nil
			},
		}

		Convey("When the cached token is younger than TokenReuse", func() {
			first, err := auth.Token(t.Context())
			So(err, ShouldBeNil)
			clock = clock.Add(TokenReuse - time.Second)
			second, err := auth.Token(t.Context())

			Convey("the same token is reused", func() {
				So(err, ShouldBeNil)
				So(second, ShouldEqual, first)
				So(fetches, ShouldEqual, 1)
			})
		})

		Convey("When TokenReuse has elapsed", func() {
			first, err := auth.Token(t.Context())
			So(err, ShouldBeNil)
			clock = clock.Add(TokenReuse)
			second, err := auth.Token(t.Context())

			Convey("a fresh token is fetched", func() {
				So(err, ShouldBeNil)
				So(second, ShouldNotEqual, first)
				So(fetches, ShouldEqual, 2)
			})
		})

		Convey("When fetch fails after a valid cache", func() {
			_, err := auth.Token(t.Context())
			So(err, ShouldBeNil)
			clock = clock.Add(TokenReuse)
			auth.fetch = func() (string, error) { return "", errors.New("rest down") }
			token, err := auth.Token(t.Context())

			Convey("the error is returned and nothing is cached", func() {
				So(err, ShouldNotBeNil)
				So(err.Error(), ShouldContainSubstring, "rest down")
				So(token, ShouldBeEmpty)
				So(fetches, ShouldEqual, 1)
			})
		})

		Convey("When a failed fetch is followed by a successful one", func() {
			_, err := auth.Token(t.Context())
			So(err, ShouldBeNil)
			clock = clock.Add(TokenReuse)
			auth.fetch = func() (string, error) { return "", errors.New("rest down") }
			_, _ = auth.Token(t.Context())
			auth.fetch = func() (string, error) { fetches++; return "token-B", nil }
			token, err := auth.Token(t.Context())

			Convey("the next call fetches again", func() {
				So(err, ShouldBeNil)
				So(token, ShouldEqual, "token-B")
			})
		})

		Convey("When fetch returns an empty token", func() {
			auth.fetch = func() (string, error) { return "", nil }
			_, err := auth.Token(t.Context())

			Convey("empty websockets token is an error", func() {
				So(err, ShouldNotBeNil)
				So(err.Error(), ShouldContainSubstring, "empty websockets token")
			})
		})

		Convey("When the context is already cancelled", func() {
			ctx, cancel := context.WithCancel(t.Context())
			cancel()
			_, err := auth.Token(ctx)

			Convey("no fetch runs", func() {
				So(err, ShouldNotBeNil)
				So(fetches, ShouldEqual, 0)
			})
		})

		Convey("When private REST is cooling down", func() {
			prev := Private
			gate := NewPrivateREST()
			gate.now = func() time.Time { return clock }
			Private = gate
			Reset(func() { Private = prev })
			So(gate.Hold(t.Context(), 1), ShouldBeNil)
			gate.ObserveError(errors.New("EAPI:Rate limit exceeded"))
			gate.Release()
			_, err := auth.Token(t.Context())

			Convey("Token refuses without fetching", func() {
				So(err, ShouldNotBeNil)
				So(err.Error(), ShouldContainSubstring, "cooling down")
				So(fetches, ShouldEqual, 0)
			})
		})
	})
}

func TestSignPrivateURL(t *testing.T) {
	Convey("Setup", t, func() {
		Convey("When the host is not api.kraken.com", func() {
			_, _, _, err := SignPrivateURL("https://example.com/0/private/Balance", map[string]any{})

			Convey("signing is rejected", func() {
				So(err, ShouldNotBeNil)
				So(err.Error(), ShouldContainSubstring, "private post host")
			})
		})
	})
}

func TestSignPrivate(t *testing.T) {
	Convey("Setup", t, func() {
		prev := processCredentials.Load()
		processCredentials.Store(nil)
		Reset(func() {
			if prev != nil {
				processCredentials.Store(prev)
			}
		})

		Convey("When no credential role is selected", func() {
			_, _, _, err := SignPrivate("/0/private/Balance", map[string]any{})

			Convey("signing fails", func() {
				So(err, ShouldNotBeNil)
			})
		})
	})
}

func BenchmarkNewAuth(b *testing.B) {
	b.ReportAllocs()

	for b.Loop() {
		_ = NewAuth()
	}
}

func BenchmarkSeedToken(b *testing.B) {
	auth := NewAuth()
	auth.now = func() time.Time { return time.Unix(1_700_000_000, 0) }

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		auth.SeedToken("pinned-token")
	}
}

func BenchmarkToken(b *testing.B) {
	clock := time.Unix(1_700_000_000, 0)
	auth := &Auth{
		now: func() time.Time { return clock },
		fetch: func() (string, error) {
			return "token-A", nil
		},
	}
	ctx := context.Background()

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		_, _ = auth.Token(ctx)
	}
}

func BenchmarkSignPrivateURL(b *testing.B) {
	params := map[string]any{}

	b.ReportAllocs()

	for b.Loop() {
		_, _, _, _ = SignPrivateURL("https://example.com/0/private/Balance", params)
	}
}

func BenchmarkSignPrivate(b *testing.B) {
	prev := processCredentials.Load()
	processCredentials.Store(nil)
	defer processCredentials.Store(prev)

	params := map[string]any{}

	b.ReportAllocs()

	for b.Loop() {
		_, _, _, _ = SignPrivate("/0/private/Balance", params)
	}
}
