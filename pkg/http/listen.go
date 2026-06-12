package http

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func ListenAndServe(server *http.Server, shutdownTimeout time.Duration) error {
	shutdownError := make(chan error, 1)
	// We run the server in a separate goroutine because `ListenAndServe()`
	// blocks, and so does the code that listens for the termination signal.
	go func(server *http.Server) {
		quit := make(chan os.Signal, 1)
		signal.Notify(quit, syscall.SIGINT)
		<-quit

		ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		shutdownError <- server.Shutdown(ctx)
	}(server)

	err := server.ListenAndServe()
	// We ignore `http.ErrServerClosed` because it is the error that's
	// expected to be returned when the server is shutdown using
	// `server.Shutdown(ctx)`.
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("unexpected error when shutting down server: %w", err)
	}

	err = <-shutdownError
	if err != nil {
		return fmt.Errorf("shutdown returned unexpected error: %w", err)
	}

	return nil
}
