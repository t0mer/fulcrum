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

	"github.com/t0mer/fulcrum/internal/api"
	"github.com/t0mer/fulcrum/internal/config"
	"github.com/t0mer/fulcrum/internal/enroll"
	"github.com/t0mer/fulcrum/internal/metrics"
	"github.com/t0mer/fulcrum/internal/ml"
	"github.com/t0mer/fulcrum/internal/pipeline"
	"github.com/t0mer/fulcrum/internal/queue"
	"github.com/t0mer/fulcrum/internal/server"
	"github.com/t0mer/fulcrum/internal/sink"
	"github.com/t0mer/fulcrum/internal/store"
	"github.com/t0mer/fulcrum/internal/version"
	"github.com/t0mer/fulcrum/internal/whatsapp"
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
	m := metrics.New(reg)

	spa, err := web.Dist()
	if err != nil {
		return fmt.Errorf("loading embedded SPA: %w", err)
	}

	mlClient := ml.New(cfg.ML.URL)
	enrollSvc := enroll.New(st, mlClient, cfg.Enroll.FacesPath)

	provider, err := whatsapp.New(cfg.Provider.Name, whatsapp.Config{
		BaseURL: cfg.Provider.BaseURL,
		Token:   cfg.Provider.Token,
	})
	if err != nil {
		return err
	}

	// Worker pool + processing pipeline.
	proc := pipeline.New(st, provider, mlClient,
		&sink.Forward{Sender: provider, DestinationGroupID: cfg.Sink.DestinationGroupID},
		pipeline.Config{
			DefaultThreshold: cfg.Match.DefaultThreshold,
			SinkMode:         cfg.Sink.Mode,
			StoragePath:      cfg.Sink.StoragePath,
		}, m, logger)
	pool := queue.New(st, proc, queue.Options{
		Workers:     cfg.Queue.Workers,
		MaxAttempts: cfg.Queue.MaxAttempts,
		Logger:      logger,
		OnDepth:     func(d int) { m.QueueDepth.Set(float64(d)) },
	})

	apiSvc := api.New(api.Deps{
		Store:         st,
		Enroll:        enrollSvc,
		Provider:      provider,
		ProviderName:  cfg.Provider.Name,
		Notifier:      pool,
		WebhookSecret: cfg.Server.WebhookSecret,
		Metrics:       m,
		Logger:        logger,
	})

	handler := server.New(server.Options{
		Logger:   logger,
		Registry: reg,
		SPA:      spa,
		API:      apiSvc.Routes(),
		Webhook:  apiSvc.WebhookHandler(),
		Ready:    st.Ping,
	})

	srv := &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.Server.Port),
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Start the worker pool alongside the HTTP server.
	poolDone := make(chan struct{})
	go func() {
		pool.Run(ctx)
		close(poolDone)
	}()

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
		err := srv.Shutdown(shutdownCtx)
		<-poolDone // wait for workers to finish the in-flight job
		return err
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
