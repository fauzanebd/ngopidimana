package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	Port                    string
	CORSOrigin              string
	DatabaseURL             string
	TypeSafeAPIKey          string
	TypeSafeModel           string
	RedisAddr               string
	RedisPassword           string
	RedisDB                 int
	IngestionQueue          string
	IngestionWorkers        int
	ExaAPIKey               string
	ExaBaseURL              string
	ExaMaxResults           int
	OpenRouterAPIKey        string
	OpenRouterModel         string
	OpenRouterBaseURL       string
	OpenRouterSiteURL       string
	OpenRouterAppName       string
	OpenRouterTimeout       time.Duration
	OpenRouterMaxInputChars int
	OpenRouterMaxOutput     int64
	GooglePlacesAPIKey      string
	GooglePlacesBaseURL     string
	GooglePlacesLanguage    string
	GooglePlacesRegion      string
	GooglePlacesDefaultArea string
	GooglePlacesTimeout     time.Duration
}

func Load() (Config, error) {
	if _, err := LoadEnvFile(".env", "../.env", "../../.env"); err != nil {
		return Config{}, err
	}
	redisDB, err := envInt("REDIS_DB", 0)
	if err != nil {
		return Config{}, err
	}
	workers, err := envInt("INGESTION_WORKERS", 4)
	if err != nil {
		return Config{}, err
	}
	if workers < 1 {
		return Config{}, errors.New("INGESTION_WORKERS must be at least 1")
	}
	exaMaxResults, err := envInt("EXA_MAX_RESULTS", 5)
	if err != nil {
		return Config{}, err
	}
	if exaMaxResults < 1 || exaMaxResults > 10 {
		return Config{}, errors.New("EXA_MAX_RESULTS must be between 1 and 10")
	}
	openRouterTimeoutSeconds, err := envInt("OPENROUTER_TIMEOUT_SECONDS", 45)
	if err != nil {
		return Config{}, err
	}
	if openRouterTimeoutSeconds < 5 || openRouterTimeoutSeconds > 120 {
		return Config{}, errors.New("OPENROUTER_TIMEOUT_SECONDS must be between 5 and 120")
	}
	openRouterMaxInputChars, err := envInt("OPENROUTER_MAX_INPUT_CHARS", 24000)
	if err != nil {
		return Config{}, err
	}
	if openRouterMaxInputChars < 4000 || openRouterMaxInputChars > 100000 {
		return Config{}, errors.New("OPENROUTER_MAX_INPUT_CHARS must be between 4000 and 100000")
	}
	openRouterMaxOutput, err := envInt("OPENROUTER_MAX_OUTPUT_TOKENS", 5000)
	if err != nil {
		return Config{}, err
	}
	if openRouterMaxOutput < 500 || openRouterMaxOutput > 20000 {
		return Config{}, errors.New("OPENROUTER_MAX_OUTPUT_TOKENS must be between 500 and 20000")
	}
	googlePlacesTimeoutSeconds, err := envInt("GOOGLE_PLACES_TIMEOUT_SECONDS", 12)
	if err != nil {
		return Config{}, err
	}
	if googlePlacesTimeoutSeconds < 5 || googlePlacesTimeoutSeconds > 60 {
		return Config{}, errors.New("GOOGLE_PLACES_TIMEOUT_SECONDS must be between 5 and 60")
	}
	return Config{
		Port:                    envOr("PORT", "8080"),
		CORSOrigin:              envOr("CORS_ORIGIN", "http://localhost:5173,http://localhost:5174"),
		DatabaseURL:             envOr("DATABASE_URL", "postgres://wheretowfc:local-development-only@127.0.0.1:5432/wheretowfc?sslmode=disable"),
		TypeSafeAPIKey:          os.Getenv("TYPESAFE_API_KEY"),
		TypeSafeModel:           envOr("TYPESAFE_DEFAULT_MODEL", "jev-latest"),
		RedisAddr:               envOr("REDIS_ADDR", "127.0.0.1:6380"),
		RedisPassword:           os.Getenv("REDIS_PASSWORD"),
		RedisDB:                 redisDB,
		IngestionQueue:          envOr("INGESTION_QUEUE", "ingestion"),
		IngestionWorkers:        workers,
		ExaAPIKey:               os.Getenv("EXA_API_KEY"),
		ExaBaseURL:              envOr("EXA_BASE_URL", "https://api.exa.ai"),
		ExaMaxResults:           exaMaxResults,
		OpenRouterAPIKey:        os.Getenv("OPENROUTER_API_KEY"),
		OpenRouterModel:         os.Getenv("OPENROUTER_MODEL"),
		OpenRouterBaseURL:       envOr("OPENROUTER_BASE_URL", "https://openrouter.ai/api/v1"),
		OpenRouterSiteURL:       os.Getenv("OPENROUTER_SITE_URL"),
		OpenRouterAppName:       envOr("OPENROUTER_APP_NAME", "Where to WFC ingestion"),
		OpenRouterTimeout:       time.Duration(openRouterTimeoutSeconds) * time.Second,
		OpenRouterMaxInputChars: openRouterMaxInputChars,
		OpenRouterMaxOutput:     int64(openRouterMaxOutput),
		GooglePlacesAPIKey:      os.Getenv("GOOGLE_PLACES_API_KEY"),
		GooglePlacesBaseURL:     envOr("GOOGLE_PLACES_BASE_URL", "https://places.googleapis.com"),
		GooglePlacesLanguage:    envOr("GOOGLE_PLACES_LANGUAGE_CODE", "id"),
		GooglePlacesRegion:      envOr("GOOGLE_PLACES_REGION_CODE", "ID"),
		GooglePlacesDefaultArea: envOr("GOOGLE_PLACES_DEFAULT_AREA", "Jakarta, Indonesia"),
		GooglePlacesTimeout:     time.Duration(googlePlacesTimeoutSeconds) * time.Second,
	}, nil
}

// LoadEnvFile loads the first file found without overriding process variables.
func LoadEnvFile(paths ...string) (string, error) {
	for _, path := range paths {
		if err := godotenv.Load(path); err == nil {
			return path, nil
		} else if !errors.Is(err, os.ErrNotExist) {
			return "", fmt.Errorf("load %s: %w", path, err)
		}
	}
	return "", nil
}

func envOr(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func envInt(key string, fallback int) (int, error) {
	value := os.Getenv(key)
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("%s must be an integer: %w", key, err)
	}
	return parsed, nil
}
