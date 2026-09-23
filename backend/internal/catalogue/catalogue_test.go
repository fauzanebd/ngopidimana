package catalogue

import (
	"database/sql"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/fauzanebd/wheretowfc/backend/internal/ingestion"
	_ "github.com/jackc/pgx/v5/stdlib"
)

func TestPublicationDataRequiresSearchableLocation(t *testing.T) {
	_, err := publicationData(ingestion.Run{Name: "Kopi Test", Area: "Cipete", GooglePlaceID: "place-test", Fields: []ingestion.EvidenceField{{Key: "Identity", Value: "Kopi Test"}}})
	if err == nil || !strings.Contains(err.Error(), "address, coordinates") {
		t.Fatalf("expected missing-location validation, got %v", err)
	}
}

func TestStorePublishesAndLoadsReviewedCafe(t *testing.T) {
	databaseURL := os.Getenv("CATALOGUE_INTEGRATION_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set CATALOGUE_INTEGRATION_DATABASE_URL to run Postgres integration test")
	}
	database, err := sql.Open("pgx", databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	identity := fmt.Sprintf("integration-%d", time.Now().UnixNano())
	t.Cleanup(func() { _, _ = database.Exec(`DELETE FROM places WHERE google_place_id = $1`, identity) })
	run := ingestion.Run{ID: identity, Name: "Integration Café", Area: "Cipete", GooglePlaceID: identity, UpdatedAt: time.Now().UTC(), Fields: []ingestion.EvidenceField{
		{Key: "Address", Value: "Jl. Integration 1", Source: "Google Maps", SourceURL: "https://maps.google.com/integration", Confidence: .97},
		{Key: "Latitude", Value: "-6.277797", Source: "Google Maps", SourceURL: "https://maps.google.com/integration", Confidence: .99},
		{Key: "Longitude", Value: "106.800699", Source: "Google Maps", SourceURL: "https://maps.google.com/integration", Confidence: .99},
		{Key: "Power outlets", Value: "Yes", Source: "Google Maps review", SourceURL: "https://maps.google.com/review/integration", Confidence: .92},
		{ID: "manual-integration", Key: "Other note", Value: "Test-only caveat", Source: "Manually added", Method: "manual", Confidence: .9},
		{Key: "Google Maps", Value: "https://maps.google.com/integration", Source: "Google Maps", SourceURL: "https://maps.google.com/integration", Confidence: 1},
	}}
	store := NewStore(database)
	if err := store.Publish(t.Context(), run); err != nil {
		t.Fatal(err)
	}
	cafes, err := store.ListPublished(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, cafe := range cafes {
		if cafe.Name == run.Name {
			found = cafe.Facts["outlets"] && cafe.MapsURL != "" && len(cafe.Tags) == 1
		}
	}
	if !found {
		t.Fatalf("published café was not searchable: %#v", cafes)
	}
}

func TestPublicationDataMapsReviewedEvidence(t *testing.T) {
	now := time.Now().UTC()
	run := ingestion.Run{Name: "TAAM House - Cipete", Area: "Cipete Selatan", GooglePlaceID: "place-123", UpdatedAt: now, Fields: []ingestion.EvidenceField{
		{Key: "Address", Value: "Jl. Cipete Raya", Confidence: .97},
		{Key: "Latitude", Value: "-6.277797", Confidence: .99},
		{Key: "Longitude", Value: "106.800699", Confidence: .99},
		{Key: "Power outlets", Value: "Yes", Confidence: .92},
		{Key: "Wi-Fi quality", Value: "Reported difficult to connect", Confidence: .9},
		{Key: "Other note", Value: "Small tables downstairs", Method: "manual", Confidence: .9},
		{Key: "Google Maps", Value: "https://maps.google.com/place/123", Confidence: 1},
	}}
	data, err := publicationData(run)
	if err != nil {
		t.Fatal(err)
	}
	if data.area != "Cipete Selatan" || data.lng != 106.800699 || data.links["google_maps"] == "" {
		t.Fatalf("location or link missing: %#v", data)
	}
	if data.scores["wfc"].value >= .8 {
		t.Fatalf("bad Wi-Fi should reduce WFC score: %#v", data.scores)
	}
}
