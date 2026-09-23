package main

import (
	"context"
	"log"
	"time"

	"github.com/fauzanebd/wheretowfc/backend/internal/config"
	"github.com/fauzanebd/wheretowfc/backend/internal/googleplaces"
	"github.com/fauzanebd/wheretowfc/backend/internal/ingestion"
	"github.com/hibiken/asynq"
	"github.com/redis/go-redis/v9"
)

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
		log.Fatalf("redis at %s is required for the worker: %v", cfg.RedisAddr, err)
	}

	exaDiscoverer := ingestion.NewExaDiscoverer(cfg.ExaAPIKey, cfg.ExaBaseURL, cfg.ExaMaxResults)
	var discoverer ingestion.Discoverer
	if exaDiscoverer.Enabled() {
		discoverer = exaDiscoverer
	}
	openRouterExtractor := ingestion.NewOpenRouterEvidenceExtractor(
		cfg.OpenRouterAPIKey, cfg.OpenRouterModel, cfg.OpenRouterBaseURL, cfg.OpenRouterSiteURL, cfg.OpenRouterAppName,
		cfg.OpenRouterTimeout, cfg.OpenRouterMaxInputChars, cfg.OpenRouterMaxOutput,
	)
	var evidenceExtractor ingestion.EvidenceExtractor
	if openRouterExtractor.Enabled() {
		evidenceExtractor = openRouterExtractor
	}
	googlePlacesClient := googleplaces.NewClient(
		cfg.GooglePlacesAPIKey, cfg.GooglePlacesBaseURL, cfg.GooglePlacesLanguage, cfg.GooglePlacesRegion,
		cfg.GooglePlacesDefaultArea, cfg.GooglePlacesTimeout,
	)
	var placeResolver ingestion.PlaceResolver
	if googlePlacesClient.Enabled() {
		placeResolver = googlePlacesClient
	}
	processor := ingestion.NewProcessor(ingestion.NewRedisStore(redisClient), ingestion.NewWebExtractor(), placeResolver, discoverer, evidenceExtractor)
	redisConnection := asynq.RedisClientOpt{Addr: cfg.RedisAddr, Password: cfg.RedisPassword, DB: cfg.RedisDB}
	server := asynq.NewServer(redisConnection, asynq.Config{
		Concurrency: cfg.IngestionWorkers,
		Queues:      map[string]int{cfg.IngestionQueue: 1},
	})
	mux := asynq.NewServeMux()
	mux.HandleFunc(ingestion.TaskTypeEnrich, processor.Handle)
	log.Printf("ingestion worker listening on queue %q with %d workers", cfg.IngestionQueue, cfg.IngestionWorkers)
	if exaDiscoverer.Enabled() {
		log.Printf("Exa source discovery enabled with up to %d results per run", cfg.ExaMaxResults)
	} else {
		log.Printf("Exa source discovery disabled; set EXA_API_KEY to enable it")
	}
	if openRouterExtractor.Enabled() {
		log.Printf("OpenRouter structured extraction enabled with model %q", cfg.OpenRouterModel)
	} else {
		log.Printf("OpenRouter structured extraction disabled; set OPENROUTER_API_KEY and OPENROUTER_MODEL to enable it")
	}
	if googlePlacesClient.Enabled() {
		log.Printf("Google Places identity resolution enabled; only stable Place IDs are persisted")
	} else {
		log.Printf("Google Places disabled; set GOOGLE_PLACES_API_KEY to enable official live review references")
	}
	if err := server.Run(mux); err != nil {
		log.Fatalf("run ingestion worker: %v", err)
	}
}
