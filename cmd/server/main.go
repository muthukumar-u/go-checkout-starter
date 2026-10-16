// Command server starts the checkout API and its embedded web page.
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

	"checkout-api/internal/httpapi"
)

func main() {
	address := os.Getenv("ADDR")
	if address == "" {
		address = "127.0.0.1:8080"
	}
	server := &http.Server{
		Addr: address, Handler: httpapi.NewHandler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    16 * 1024,
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	shutdownDone := make(chan struct{})
	go func() {
		defer close(shutdownDone)
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			log.Printf("Graceful shutdown: %v", err)
		}
	}()
	log.Printf("Checkout page and API: http://%s", address)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
	// Shutdown stops accepting connections first, then waits for active requests.
	// Keep main alive until those requests finish (or the deadline expires).
	<-shutdownDone
}
