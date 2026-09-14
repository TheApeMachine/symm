package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/bytedance/sonic"
	"github.com/gofiber/fiber/v3"
	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/workbench"
)

const arrowStream = "application/vnd.apache.arrow.stream"

func main() {
	viper.SetDefault("workbench.addr", ":8081")
	viper.SetDefault("storage.s3.endpoint", "http://seaweed.home.arpa:8333")
	viper.SetDefault("storage.iceberg.uri", "http://iceberg.seaweed.home.arpa")
	viper.SetDefault("storage.iceberg.warehouse", "s3://symmtables/")
	viper.SetDefault("workbench.memory_limit", "16GB")
	viper.SetDefault("workbench.max_temp_directory_size", "10GB")
	viper.SetDefault("workbench.threads", 4)

	viper.SetConfigName("config")
	viper.SetConfigType("yml")
	viper.AddConfigPath("cmd/cfg")
	viper.AddConfigPath("$HOME/.symm")
	viper.AddConfigPath(".")

	if err := viper.ReadInConfig(); err != nil {
		errnie.Info(fmt.Sprintf("workbench: config file notice: %v", err))
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	warehouse := workbench.New()
	defer func() {
		if err := warehouse.Close(); err != nil {
			errnie.Error(err)
		}
	}()

	app := fiber.New(fiber.Config{
		JSONEncoder: sonic.Marshal,
		JSONDecoder: sonic.Unmarshal,
	})

	app.Get("/health", func(c fiber.Ctx) error {
		return c.JSON(fiber.Map{
			"status":  "ok",
			"service": "symm-workbench",
		})
	})

	app.Post("/workbench/query", func(c fiber.Ctx) error {
		var request struct {
			SQL string `json:"sql"`
		}

		if err := c.Bind().Body(&request); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}

		if request.SQL == "" {
			return fiber.NewError(fiber.StatusBadRequest, "query is empty")
		}

		stream, err := warehouse.Execute(ctx, request.SQL)
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}

		c.Set(fiber.HeaderContentType, arrowStream)
		return c.Send(stream)
	})

	addr := viper.GetString("workbench.addr")
	errnie.Info(fmt.Sprintf("starting standalone analytical workbench on %s", addr))

	stopSignals := make(chan os.Signal, 1)
	signal.Notify(stopSignals, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-stopSignals
		errnie.Info("shutting down analytical workbench...")
		cancel()

		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownCancel()

		if err := app.ShutdownWithContext(shutdownCtx); err != nil {
			errnie.Error(err)
		}

		if err := warehouse.Close(); err != nil {
			errnie.Error(err)
		}
	}()

	if err := app.Listen(addr); err != nil {
		errnie.Error(err)
	}
}
