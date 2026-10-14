// Package config lee la configuración de la aplicación desde variables de entorno.
package config

import (
	"errors"
	"os"
)

type Config struct {
	Port        string // puerto HTTP, por ejemplo "8080"
	DatabaseURL string // cadena de conexión de Neon/PostgreSQL
	Env         string // "development" o "production"
}

// SecureCookies indica si las cookies deben marcarse como Secure (solo HTTPS).
// En producción siempre debe ser true.
func (c Config) SecureCookies() bool {
	return c.Env == "production"
}

func Load() (Config, error) {
	cfg := Config{
		Port:        getEnv("PORT", "8080"),
		DatabaseURL: getEnv("DATABASE_URL", "dd"),
		Env:         getEnv("APP_ENV", "development"),
	}
	if cfg.DatabaseURL == "" {
		return cfg, errors.New("falta la variable de entorno DATABASE_URL")
	}
	return cfg, nil
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
