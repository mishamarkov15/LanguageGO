package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"hw-9/gateway/internal/api"
	"hw-9/ledger"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	timeout := 2 * time.Second
	if raw, ok := os.LookupEnv("REQUEST_TIMEOUT"); ok {
		parsed, err := time.ParseDuration(raw)
		if err != nil || parsed <= 0 {
			return fmt.Errorf("REQUEST_TIMEOUT: ожидается положительная длительность, например 2s или 50ms")
		}
		timeout = parsed
	}
	addr := os.Getenv("HTTP_ADDR")
	if addr == "" {
		addr = ":8080"
	}
	svc, closeLedger, err := ledger.New(ctx)
	if err != nil {
		return err
	}
	defer closeLedger()
	log.Println("Ledger started")

	server := &http.Server{
		Addr:              addr,
		Handler:           api.NewRouter(svc, timeout),
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			log.Printf("остановка сервера: %v", err)
		}
	}()

	log.Printf("Gateway listening on %s, request timeout %s", server.Addr, timeout)
	if err := server.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	log.Println("Gateway stopped")
	return nil
}
