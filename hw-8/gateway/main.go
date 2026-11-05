package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"hw-8/gateway/internal/api"
	"hw-8/ledger"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	svc, closeLedger, err := ledger.New(ctx)
	if err != nil {
		return err
	}
	defer closeLedger()
	log.Println("Ledger started")

	server := &http.Server{
		Addr:              ":8080",
		Handler:           api.NewRouter(svc),
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

	log.Printf("Gateway listening on %s", server.Addr)
	if err := server.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	log.Println("Gateway stopped")
	return nil
}
