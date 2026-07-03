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
	"strings"
	"time"

	"github.com/kardianos/service"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/spf13/pflag"

	"github.com/t0mer/fulcrum/internal/api"
	"github.com/t0mer/fulcrum/internal/config"
	"github.com/t0mer/fulcrum/internal/enroll"
	"github.com/t0mer/fulcrum/internal/intake"
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
	serviceAction := fs.String("service", "", "OS service control: install|uninstall|start|stop|restart|status")
	config.BindFlags(fs)
	if err := fs.Parse(os.Args[1:]); err != nil {
		return err
	}
	if *showVersion {
		fmt.Println(version.Version)
		return nil
	}

	svcConfig := &service.Config{
		Name:        "fulcrum",
		DisplayName: "Fulcrum",
		Description: "Watches WhatsApp groups and forwards photos of enrolled subjects.",
		// Re-run with the same flags (minus --service) when the manager starts us.
		Arguments: withoutServiceFlag(os.Args[1:]),
	}
	prg := &program{flags: fs}
	svc, err := service.New(prg, svcConfig)
	if err != nil {
		return fmt.Errorf("creating service: %w", err)
	}

	if *serviceAction != "" {
		return controlService(svc, *serviceAction)
	}
	// service.Run handles both foreground (interactive) and managed execution;
	// it calls prg.Start then blocks, and prg.Stop on shutdown signals.
	return svc.Run()
}

func controlService(svc service.Service, action string) error {
	if action == "status" {
		status, err := svc.Status()
		if err != nil {
			return err
		}
		fmt.Println(statusString(status))
		return nil
	}
	return service.Control(svc, action)
}

func statusString(s service.Status) string {
	switch s {
	case service.StatusRunning:
		return "running"
	case service.StatusStopped:
		return "stopped"
	default:
		return "unknown"
	}
}

// program adapts the daemon to the kardianos service lifecycle.
type program struct {
	flags  *pflag.FlagSet
	cancel context.CancelFunc
	done   chan struct{}
	err    error
}

func (p *program) Start(service.Service) error {
	ctx, cancel := context.WithCancel(context.Background())
	p.cancel = cancel
	p.done = make(chan struct{})
	go func() {
		defer close(p.done)
		p.err = p.daemon(ctx)
	}()
	return nil
}

func (p *program) Stop(service.Service) error {
	if p.cancel != nil {
		p.cancel()
	}
	if p.done != nil {
		<-p.done
	}
	return p.err
}

// daemon builds and runs the full stack until ctx is cancelled.
func (p *program) daemon(ctx context.Context) error {
	cfg, err := config.Load(p.flags)
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

	proc := pipeline.New(st, provider, mlClient,
		&sink.Forward{Sender: provider, DestinationGroupID: cfg.Sink.DestinationGroupID},
		pipeline.Config{
			DefaultThreshold: cfg.Match.DefaultThreshold,
			SinkMode:         cfg.Sink.Mode,
			StoragePath:      cfg.Sink.StoragePath,
			NearDupDistance:  cfg.Match.NearDupDistance,
		}, m, logger)
	pool := queue.New(st, proc, queue.Options{
		Workers:     cfg.Queue.Workers,
		MaxAttempts: cfg.Queue.MaxAttempts,
		Logger:      logger,
		OnDepth:     func(d int) { m.QueueDepth.Set(float64(d)) },
	})

	// Single admission path shared by the webhook route and the polling receiver.
	intakeSvc := intake.New(st, m, pool, cfg.Provider.Name, logger)

	apiSvc := api.New(api.Deps{
		Store:            st,
		Enroll:           enrollSvc,
		Provider:         provider,
		ProviderName:     cfg.Provider.Name,
		Intake:           intakeSvc,
		WebhookSecret:    cfg.Server.WebhookSecret,
		AuthToken:        cfg.Server.AuthToken,
		Logger:           logger,
		DefaultThreshold: cfg.Match.DefaultThreshold,
		DefaultSinkMode:  cfg.Sink.Mode,
		MatchesPath:      cfg.Sink.StoragePath,
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

	// Derive a cancellable context so a server startup/serve failure can stop
	// the worker pool and receiver before the deferred store Close — otherwise
	// they keep running against a closed DB (repeated "database is closed").
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	poolDone := make(chan struct{})
	go func() {
		pool.Run(ctx)
		close(poolDone)
	}()

	// Providers that pull messages themselves (e.g. green-api's bot library)
	// run a background receive loop instead of an inbound HTTP webhook. It
	// admits messages through the same intake path as the webhook route and
	// stops when ctx is cancelled.
	if rcv, ok := provider.(whatsapp.Receiver); ok {
		go func() {
			handle := func(m whatsapp.InboundMessage) { intakeSvc.Accept(ctx, m) }
			if err := rcv.Receive(ctx, handle); err != nil && ctx.Err() == nil {
				logger.Error("message receiver stopped", "err", err)
			}
		}()
	}

	errCh := make(chan error, 1)
	go func() {
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case err := <-errCh:
		// Server failed: stop the workers and receiver, then drain before the
		// deferred store Close so they don't race a closed DB.
		cancel()
		<-poolDone
		return err
	case <-ctx.Done():
		logger.Info("shutting down")
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer shutdownCancel()
		err := srv.Shutdown(shutdownCtx)
		<-poolDone // wait for workers to finish the in-flight job
		return err
	}
}

// withoutServiceFlag returns args with the --service flag (and its value)
// removed, so the installed service doesn't recurse into control mode.
func withoutServiceFlag(args []string) []string {
	out := make([]string, 0, len(args))
	skip := false
	for _, a := range args {
		if skip {
			skip = false
			continue
		}
		if a == "--service" || a == "-service" {
			skip = true // value follows as the next token
			continue
		}
		if strings.HasPrefix(a, "--service=") || strings.HasPrefix(a, "-service=") {
			continue
		}
		out = append(out, a)
	}
	return out
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
