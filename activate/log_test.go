package activate

import (
	"bytes"
	"io"
	"os"
	"sync"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestOnce(t *testing.T) {
	Convey("Given activate.Once", t, func() {
		seen = sync.Map{}

		Convey("It should print only the first call for a key", func() {
			stdout := captureStdout(func() {
				Once("test-once")
				Once("test-once")
				Once("other-key")
			})

			So(stdout, ShouldContainSubstring, "test-once")
			So(stdout, ShouldContainSubstring, "other-key")
			So(bytes.Count([]byte(stdout), []byte("test-once")), ShouldEqual, 1)
		})
	})
}

func captureStdout(run func()) string {
	reader, writer, err := os.Pipe()

	if err != nil {
		panic(err)
	}

	oldStdout := os.Stdout
	os.Stdout = writer

	var captured bytes.Buffer
	var wait sync.WaitGroup

	wait.Add(1)

	go func() {
		defer wait.Done()
		_, _ = io.Copy(&captured, reader)
	}()

	run()

	writer.Close()
	os.Stdout = oldStdout
	wait.Wait()

	return captured.String()
}
