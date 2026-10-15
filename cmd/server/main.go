package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/diegovhdev/yortale/internal/auth"
	"github.com/diegovhdev/yortale/internal/platform/config"
	"github.com/diegovhdev/yortale/internal/platform/database"
	"github.com/diegovhdev/yortale/internal/server"
	"github.com/diegovhdev/yortale/internal/stories"
	"github.com/joho/godotenv"
)

func main() {
	if err := run(); err != nil {
		slog.Error("el servidor se detuvo", "err", err)
		os.Exit(1)
	}
}

func run() error {

	_ = godotenv.Load()

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	// Se cancela con Ctrl+C o cuando el hosting envía SIGTERM.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := database.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	// Repositorios -> servicios -> router
	users := auth.NewUserRepository(pool)
	sessions := auth.NewSessionRepository(pool)
	authService := auth.NewService(users, sessions)
	storyService := stories.NewService(stories.NewStoryRepository(pool), stories.NewChapterRepository(pool))

	go cleanupSessions(ctx, authService)

	srv := &http.Server{
		Addr: ":" + cfg.Port,
		Handler: server.NewRouter(server.Deps{
			Auth:          authService,
			Stories:       storyService,
			SecureCookies: cfg.SecureCookies(),
		}),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		slog.Info("servidor escuchando", "url", "http://localhost:"+cfg.Port, "env", cfg.Env)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
		close(errCh)
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
	}

	// Apagado ordenado: deja terminar las peticiones en curso.
	slog.Info("apagando servidor...")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return srv.Shutdown(shutdownCtx)
}

// cleanupSessions borra cada hora las sesiones expiradas.
func cleanupSessions(ctx context.Context, sessions *auth.Service) {
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if n, err := sessions.CleanupExpiredSessions(ctx); err != nil {
				slog.Error("limpiar sesiones", "err", err)
			} else if n > 0 {
				slog.Info("sesiones expiradas borradas", "cantidad", n)
			}
		}
	}
}
