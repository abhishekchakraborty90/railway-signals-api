// Command railway-signals-api serves a read-only REST API over the railway
// signals/tracks dataset. Data is loaded into memory once at startup.
package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"

	"github.com/crosstech/railway-signals-api/internal/config"
	"github.com/crosstech/railway-signals-api/internal/handlers"
	"github.com/crosstech/railway-signals-api/internal/store"
)

func main() {
	dataFile := envOr(config.EnvDataFile, config.DefaultDataFile)

	// Fail fast: without data the API cannot serve anything meaningful.
	db, err := store.Load(dataFile)
	if err != nil {
		log.Fatalf("startup: could not load data: %v", err)
	}
	log.Printf("loaded data from %q", dataFile)

	e := echo.New()
	e.HideBanner = true
	e.HTTPErrorHandler = handlers.ErrorHandler // uniform JSON errors everywhere
	e.Use(middleware.Recover())
	e.Use(middleware.Logger())
	e.Use(middleware.CORS()) // permissive CORS so a separate frontend can call it

	handlers.New(db).Register(e)

	port := envOr(config.EnvPort, config.DefaultPort)
	go func() {
		if err := e.Start(":" + port); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("server error: %v", err)
		}
	}()
	log.Printf("listening on :%s", port)

	// Graceful shutdown on SIGINT/SIGTERM.
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	ctx, cancel := context.WithTimeout(context.Background(), config.ShutdownTimeout)
	defer cancel()
	if err := e.Shutdown(ctx); err != nil {
		log.Fatalf("shutdown error: %v", err)
	}
}

// envOr returns the env var value or a fallback when unset/empty.
func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
