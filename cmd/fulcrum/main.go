// Command fulcrum is the Go core: webhook intake, job queue, matcher, sinks,
// HTTP API, and embedded SPA. See CLAUDE.md for the full design contract.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/spf13/pflag"

	"github.com/t0mer/fulcrum/internal/config"
	"github.com/t0mer/fulcrum/internal/metrics"
	"github.com/t0mer/fulcrum/internal/server"
	"github.com/t0mer/fulcrum/internal/store"
	"github.com/t0mer/fulcrum/internal/version"
	"github.com/t0mer/fulcrum/web"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "fulcrum:", err)
		os.Exit(1)
	}
}

func run() error {
	fs := pflag.NewFlagSet("fulcrum", pflag.ContinueOnError)
	showVersion := fs.Bool("version", false, "print version and exit")
	config.BindFlags(fs)
	if err := fs.Parse(os.Args[1:]); err != nil {
		return err
	}
	if *showVersion {
		fmt.Println(version.Version)
		return nil
	}

	cfg, err := config.Load(fs)
	if err != nil {
		return err
	}

	logger := newLogger(cfg.Server.LogLevel)
	logger.Info("starting fulcrum", "version", version.Version, "port", cfg.Server.Port)

	st, err := store.Open(cfg.DBPath)
	if err != nil {
		return err
	}
	defer st.Close()

	reg := prometheus.NewRegistry()
	reg.MustRegister(prometheus.NewGoCollector())
	_ = metrics.New(reg)

	spa, err := web.Dist()
	if err != nil {
		return fmt.Errorf("loading embedded SPA: %w", err)
	}

	handler := server.New(server.Options{
		Logger:   logger,
		Registry: reg,
		SPA:      spa,
		Ready:    st.Ping,
	})

	srv := &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.Server.Port),
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)
	go func() {
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		logger.Info("shutting down")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return srv.Shutdown(shutdownCtx)
	}
}

func newLogger(level string) *slog.Logger {
	var lvl slog.Level
	switch level {
	case "debug":
		lvl = slog.LevelDebug
	case "warning", "warn":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		lvl = slog.LevelInfo
	}
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: lvl}))
}
