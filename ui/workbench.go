package ui

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	neturl "net/url"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/bytedance/sonic"
	"github.com/gofiber/fiber/v3"
	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/catalog"
)

/*
arrowStream is the media type of an Apache Arrow IPC stream. The dashboard
hands the response body to the Perspective viewer unparsed, so the body is the
stream itself rather than a JSON envelope carrying it.
*/
const arrowStream = "application/vnd.apache.arrow.stream"

/*
WorkbenchSupervisor manages the out-of-process analytical workbench server.
DuckDB/Iceberg materializations and Go GC sweeps are physically isolated from
the critical trading loop by running in a separate process. The supervisor ensures
the server is booted and accessible, auto-spawning it when running locally if
not already running via `make workbench`.
*/
type WorkbenchSupervisor struct {
	mu       sync.Mutex
	cmd      *exec.Cmd
	queryURL string
}

func NewWorkbenchSupervisor(queryURL string) *WorkbenchSupervisor {
	return &WorkbenchSupervisor{
		queryURL: queryURL,
	}
}

func (supervisor *WorkbenchSupervisor) healthURL() string {
	parsed, err := neturl.Parse(supervisor.queryURL)

	if err != nil {
		return "http://127.0.0.1:8081/health"
	}

	parsed.Path = "/health"
	parsed.RawQuery = ""

	return parsed.String()
}

func (supervisor *WorkbenchSupervisor) isLocal() bool {
	parsed, err := neturl.Parse(supervisor.queryURL)

	if err != nil {
		return true
	}

	host := parsed.Hostname()

	return host == "127.0.0.1" || host == "localhost" || host == "::1" || host == "0.0.0.0" || host == ""
}

func (supervisor *WorkbenchSupervisor) isHealthy(ctx context.Context) bool {
	checkCtx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
	defer cancel()

	request, err := http.NewRequestWithContext(checkCtx, http.MethodGet, supervisor.healthURL(), nil)

	if err != nil {
		return false
	}

	response, err := http.DefaultClient.Do(request)

	if err != nil {
		return false
	}

	defer func() {
		if closeErr := response.Body.Close(); closeErr != nil {
			errnie.Error(closeErr)
		}
	}()

	return response.StatusCode == http.StatusOK
}

func (supervisor *WorkbenchSupervisor) Ensure(ctx context.Context) error {
	if supervisor.isHealthy(ctx) {
		return nil
	}

	supervisor.mu.Lock()
	defer supervisor.mu.Unlock()

	if supervisor.isHealthy(ctx) {
		return nil
	}

	if !supervisor.isLocal() {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"workbench service is remote and not healthy at "+supervisor.queryURL,
			nil,
		))
	}

	var command *exec.Cmd

	if _, statErr := os.Stat("bin/symm-workbench"); statErr == nil {
		command = exec.CommandContext(ctx, "./bin/symm-workbench")
	}

	if command == nil {
		command = exec.CommandContext(ctx, "go", "run", "./cmd/workbench")
	}

	command.Stdout = os.Stdout
	command.Stderr = os.Stderr

	if err := command.Start(); err != nil {
		return errnie.Error(errnie.Err(
			errnie.IO,
			"failed starting analytical workbench process",
			err,
		))
	}

	supervisor.cmd = command
	errnie.Info("[workbench] spawned standalone analytical workbench process")

	deadline := time.Now().Add(15 * time.Second)
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if supervisor.isHealthy(ctx) {
				return nil
			}

			if time.Now().After(deadline) {
				return errnie.Error(errnie.Err(
					errnie.Timeout,
					"timeout waiting for standalone analytical workbench to report healthy",
					nil,
				))
			}
		}
	}
}

func (supervisor *WorkbenchSupervisor) Close() error {
	supervisor.mu.Lock()
	defer supervisor.mu.Unlock()

	if supervisor.cmd != nil && supervisor.cmd.Process != nil {
		if err := supervisor.cmd.Process.Kill(); err != nil {
			errnie.Warn(fmt.Sprintf("[workbench] failed to kill process: %v", err))
		}

		_ = supervisor.cmd.Wait()
		supervisor.cmd = nil
	}

	return nil
}

/*
registerWorkbench mounts the Analytical Workbench's surface.

Analytical queries are executed out-of-process by the standalone symm-workbench service
to physically isolate DuckDB materializations and Go GC sweeps from the critical trading loop.
The hub acts as a gateway reverse-proxying statements to the workbench service.
*/
func (hub *Hub) registerWorkbench() {
	if hub.workbenchSupervisor != nil {
		go func() {
			if err := hub.workbenchSupervisor.Ensure(hub.Context()); err != nil {
				errnie.Warn("[workbench] supervisor auto-boot: " + err.Error())
			}
		}()
	}

	/*
		/workbench/primitives is the palette the pipeline editor draws from:
		every nomagique primitive, what each is for, and which of its arguments
		are streams to be wired rather than settings to be typed. It is derived
		from nomagique's own declarations, so the editor can only offer what the
		library actually has.
	*/
	hub.app.Get("/workbench/primitives", func(fiberCtx fiber.Ctx) error {
		primitives, err := catalog.Primitives()

		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}

		return fiberCtx.JSON(primitives)
	})

	// /workbench/query proxies one analytical statement to the standalone
	// workbench service, returning its result as an Arrow IPC stream.
	hub.app.Post("/workbench/query", func(fiberCtx fiber.Ctx) error {
		var request struct {
			SQL string `json:"sql"`
		}

		if err := fiberCtx.Bind().Body(&request); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}

		if request.SQL == "" {
			return fiber.NewError(fiber.StatusBadRequest, "query is empty")
		}

		workbenchURL := viper.GetString("workbench.url")

		if workbenchURL == "" {
			workbenchURL = "http://127.0.0.1:8081/workbench/query"
		}

		if hub.workbenchSupervisor != nil {
			if err := hub.workbenchSupervisor.Ensure(hub.Context()); err != nil {
				return fiber.NewError(
					fiber.StatusServiceUnavailable,
					"workbench service unavailable at "+workbenchURL+": "+err.Error(),
				)
			}
		}

		payload, err := sonic.Marshal(request)

		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}

		proxyRequest, err := http.NewRequestWithContext(
			hub.Context(),
			http.MethodPost,
			workbenchURL,
			bytes.NewReader(payload),
		)

		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}

		proxyRequest.Header.Set("Content-Type", "application/json")

		response, err := http.DefaultClient.Do(proxyRequest)

		if err != nil {
			return fiber.NewError(
				fiber.StatusServiceUnavailable,
				"workbench service unavailable at "+workbenchURL+": "+err.Error(),
			)
		}

		defer func() {
			if closeErr := response.Body.Close(); closeErr != nil {
				errnie.Error(closeErr)
			}
		}()

		if response.StatusCode != http.StatusOK {
			messageBytes, readErr := io.ReadAll(response.Body)

			if readErr != nil {
				return fiber.NewError(response.StatusCode, "failed reading workbench error")
			}

			return fiber.NewError(response.StatusCode, string(messageBytes))
		}

		stream, err := io.ReadAll(response.Body)

		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}

		fiberCtx.Set(fiber.HeaderContentType, arrowStream)

		if err := fiberCtx.Send(stream); err != nil {
			return errnie.Error(errnie.Err(
				errnie.IO,
				"hub: send arrow ipc stream",
				err,
			))
		}

		return nil
	})
}

