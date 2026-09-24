package main

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/fauzanebd/wheretowfc/backend/internal/auth"
	"github.com/fauzanebd/wheretowfc/backend/internal/catalogue"
	"github.com/fauzanebd/wheretowfc/backend/internal/config"
	"github.com/fauzanebd/wheretowfc/backend/internal/googleplaces"
	"github.com/fauzanebd/wheretowfc/backend/internal/httpapi"
	"github.com/fauzanebd/wheretowfc/backend/internal/ingestion"
	"github.com/fauzanebd/wheretowfc/backend/internal/recommendation"
	"github.com/hibiken/asynq"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/redis/go-redis/v9"
)

// adminMailer builds the mailer for the API and reports which transport it got.
func adminMailer(cfg config.Config) auth.Mailer {
	if cfg.SMTPHost == "" {
		log.Printf("SMTP_HOST is not set: catalogue admin sign-in links will be written to this log instead of emailed")
		return auth.LogMailer{}
	}
	mode := auth.SMTPTLSMode(cfg.SMTPPort, cfg.SMTPTLS)
	if mode == "none" {
		log.Printf("SMTP_TLS=none: sign-in mail goes to %s:%d unencrypted (only acceptable for a local relay)", cfg.SMTPHost, cfg.SMTPPort)
	} else {
		log.Printf("catalogue admin sign-in email via %s:%d (%s)", cfg.SMTPHost, cfg.SMTPPort, mode)
	}
	return auth.MailerFor(cfg.SMTPHost, cfg.SMTPPort, cfg.SMTPUsername, cfg.SMTPPassword, cfg.SMTPFrom, cfg.SMTPTLS)
}

type redisHealth struct{ client *redis.Client }

func (health redisHealth) Ping(ctx context.Context) error { return health.client.Ping(ctx).Err() }

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}
	redisOptions := &redis.Options{Addr: cfg.RedisAddr, Password: cfg.RedisPassword, DB: cfg.RedisDB}
	redisClient := redis.NewClient(redisOptions)
	defer redisClient.Close()
	startupContext, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := redisClient.Ping(startupContext).Err(); err != nil {
		log.Fatalf("redis at %s is required for ingestion: %v", cfg.RedisAddr, err)
	}
	database, err := sql.Open("pgx", cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("open Postgres: %v", err)
	}
	defer database.Close()
	if err := database.PingContext(startupContext); err != nil {
		log.Fatalf("Postgres is required for publishing and recommendations: %v", err)
	}
	catalogueStore := catalogue.NewStore(database)

	queueClient := asynq.NewClient(asynq.RedisClientOpt{Addr: cfg.RedisAddr, Password: cfg.RedisPassword, DB: cfg.RedisDB})
	defer queueClient.Close()
	store := ingestion.NewRedisStore(redisClient)
	ingestionService := ingestion.NewService(store, ingestion.NewAsynqQueue(queueClient, cfg.IngestionQueue), catalogueStore)
	interpreter, provider := recommendation.NewInterpreter(cfg.TypeSafeAPIKey, cfg.TypeSafeModel)
	recommendationService := recommendation.NewService(interpreter, catalogueStore)
	googlePlacesClient := googleplaces.NewClient(
		cfg.GooglePlacesAPIKey, cfg.GooglePlacesBaseURL, cfg.GooglePlacesLanguage, cfg.GooglePlacesRegion,
		cfg.GooglePlacesDefaultArea, cfg.GooglePlacesTimeout,
	)
	log.Printf("query interpreter: %s", provider)
	if googlePlacesClient.Enabled() {
		log.Printf("Google Place Details enabled for uncached admin review display")
	}

	authStore := auth.NewPostgresStore(database)
	authService := auth.NewService(authStore, adminMailer(cfg), cfg.AdminAppURL).WithSessionTTL(cfg.AdminSessionTTL)
	sameSite := httpapi.ParseSameSite(cfg.AdminCookieSameSite)
	log.Printf("catalogue admin sign-in: %s (session %s, cookie SameSite=%s)", cfg.AdminAppURL, cfg.AdminSessionTTL, cfg.AdminCookieSameSite)

	server := &http.Server{
		Addr: ":" + cfg.Port,
		Handler: httpapi.NewServer(httpapi.Options{
			Recommendations: recommendationService,
			Ingestion:       ingestionService,
			GooglePlaces:    googlePlacesClient,
			Health:          redisHealth{redisClient},
			CORSOrigins:     cfg.CORSOrigin,
			Auth:            authService,
			PhotoStore:      catalogueStore,
			Cookies: httpapi.CookiePolicy{
				Name:     "wfc_admin_session",
				Domain:   cfg.AdminCookieDomain,
				Secure:   strings.HasPrefix(cfg.AdminAppURL, "https://") || sameSite == http.SameSiteNoneMode,
				SameSite: sameSite,
				TTL:      cfg.AdminSessionTTL,
			},
		}),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	go func() {
		log.Printf("where to wfc api listening on http://localhost:%s", cfg.Port)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("serve api: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	shutdownContext, shutdownCancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer shutdownCancel()
	if err := server.Shutdown(shutdownContext); err != nil {
		log.Printf("api shutdown: %v", err)
	}
}
