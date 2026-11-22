package main

import (
	"context"
	"errors"
	"flag"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/cusconf/cusconf/internal/api"
	"github.com/cusconf/cusconf/internal/source"
	"github.com/cusconf/cusconf/internal/store"
	"github.com/cusconf/cusconf/internal/tester"
	"github.com/cusconf/cusconf/web"
)

func main() {
	addr := flag.String("addr", envOr("CUSCONF_ADDR", ":8080"), "listen address")
	dataDir := flag.String("data", envOr("CUSCONF_DATA", "./data"), "data directory")
	flag.Parse()

	if err := os.MkdirAll(*dataDir, 0o700); err != nil {
		fatal("data dir", err)
	}
	st, err := store.Open(filepath.Join(*dataDir, "cusconf.db"))
	if err != nil {
		fatal("open store", err)
	}
	defer st.Close()

	mgr := source.NewManager(st)
	dist, err := fs.Sub(web.Dist, "dist")
	if err != nil {
		fatal("web assets", err)
	}
	tests := tester.NewManager(st)
	srv, err := api.New(st, mgr, tests, dist)
	if err != nil {
		fatal("init api", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go mgr.Run(ctx)
	go tests.RunSchedule(ctx, st.AllNodes)

	hs := &http.Server{Addr: *addr, Handler: srv.Handler(), ReadHeaderTimeout: 10 * time.Second}
	go func() {
		<-ctx.Done()
		sctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		hs.Shutdown(sctx)
	}()
	slog.Info("cusconf listening", "addr", *addr, "data", *dataDir)
	if err := hs.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		fatal("serve", err)
	}
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func fatal(msg string, err error) {
	slog.Error(msg, "err", err)
	os.Exit(1)
}
