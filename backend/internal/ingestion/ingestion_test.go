package ingestion

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/fauzanebd/wheretowfc/backend/internal/googleplaces"
	"github.com/hibiken/asynq"
)

type fakeQueue struct{ runID, sourceURL string }

func (queue *fakeQueue) Enqueue(_ context.Context, runID, sourceURL string) error {
	queue.runID, queue.sourceURL = runID, sourceURL
	return nil
}

type fakePublisher struct {
	run Run
	err error
}

func (publisher *fakePublisher) Publish(_ context.Context, run Run) error {
	publisher.run = run
	return publisher.err
}

func (publisher *fakePublisher) Remove(_ context.Context, run Run) error {
	publisher.run = run
	return publisher.err
}

type fakeExtractor struct{ result ExtractedRecord }

func (extractor fakeExtractor) Extract(context.Context, string) (ExtractedRecord, error) {
	return extractor.result, nil
}

type fakeDiscoverer struct {
	result []DiscoveredSource
	err    error
}

type fakeEvidenceExtractor struct {
	input  ExtractionInput
	result ModelExtraction
}

func (extractor *fakeEvidenceExtractor) ExtractEvidence(_ context.Context, input ExtractionInput) (ModelExtraction, error) {
	extractor.input = input
	return extractor.result, nil
}

type fakePlaceResolver struct {
	placeID string
	err     error
	place   googleplaces.Place
}

func (resolver fakePlaceResolver) ResolvePlaceID(context.Context, string, string) (string, error) {
	return resolver.placeID, resolver.err
}

func (resolver fakePlaceResolver) GetPlace(context.Context, string) (googleplaces.Place, error) {
	return resolver.place, resolver.err
}

func (discoverer fakeDiscoverer) Discover(context.Context, DiscoverySeed) ([]DiscoveredSource, error) {
	return discoverer.result, discoverer.err
}

func TestCreateQueuesDurableRun(t *testing.T) {
	store, queue := NewMemoryStore(), &fakeQueue{}
	service := NewService(store, queue)
	run, err := service.Create(t.Context(), "https://example.com/cafe")
	if err != nil {
		t.Fatal(err)
	}
	if run.State != "enriching" || run.Stage != "queued" || queue.runID != run.ID || queue.sourceURL != run.URL {
		t.Fatalf("run not queued correctly: run=%#v queue=%#v", run, queue)
	}
	if stored, err := store.Get(t.Context(), run.ID); err != nil || stored.ID != run.ID {
		t.Fatalf("run was not saved: stored=%#v err=%v", stored, err)
	}
}

func TestDeletePermanentlyRemovesRun(t *testing.T) {
	store := NewMemoryStore()
	service := NewService(store, &fakeQueue{})
	run, err := service.Create(t.Context(), "https://example.com/cafe")
	if err != nil {
		t.Fatal(err)
	}
	if err := service.Delete(t.Context(), run.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Get(t.Context(), run.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("deleted run should not be retrievable: %v", err)
	}
	runs, err := store.List(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 0 {
		t.Fatalf("deleted run remained in list: %#v", runs)
	}
	if err := store.Save(t.Context(), run); !errors.Is(err, ErrNotFound) {
		t.Fatalf("an active worker must not recreate a deleted run: %v", err)
	}
}

func TestDeleteMissingRunReturnsNotFound(t *testing.T) {
	service := NewService(NewMemoryStore(), &fakeQueue{})
	if err := service.Delete(t.Context(), "missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected not found, got %v", err)
	}
}

func TestManualEvidenceCanBeAddedRemovedAndRequiresRepublish(t *testing.T) {
	store := NewMemoryStore()
	run := Run{ID: "ing_manual", State: "published", Stage: "published", Fields: []EvidenceField{{Key: "Identity", Value: "Kopi Test", Confidence: .9}}}
	if err := store.Save(t.Context(), run); err != nil {
		t.Fatal(err)
	}
	service := NewService(store, &fakeQueue{})
	updated, err := service.AddManualEvidence(t.Context(), run.ID, ManualEvidenceInput{Key: "Seating comfort", Value: "Upstairs chairs have back support."})
	if err != nil {
		t.Fatal(err)
	}
	if updated.State != "needs_review" || len(updated.Fields) != 2 || updated.Fields[1].Method != "manual" || updated.Fields[1].Source != "Manually added" || updated.Fields[1].ID == "" {
		t.Fatalf("unexpected manual evidence: %#v", updated)
	}
	updated, err = service.RemoveManualEvidence(t.Context(), run.ID, updated.Fields[1].ID)
	if err != nil || len(updated.Fields) != 1 {
		t.Fatalf("manual evidence was not removed: run=%#v err=%v", updated, err)
	}
}

func TestPublishCallsCataloguePublisherBeforeChangingState(t *testing.T) {
	store := NewMemoryStore()
	run := Run{ID: "ing_publish", State: "needs_review", Issues: []string{}, Fields: []EvidenceField{}}
	if err := store.Save(t.Context(), run); err != nil {
		t.Fatal(err)
	}
	publisher := &fakePublisher{}
	service := NewService(store, &fakeQueue{}, publisher)
	updated, err := service.Update(t.Context(), run.ID, "publish")
	if err != nil {
		t.Fatal(err)
	}
	if publisher.run.ID != run.ID || updated.State != "published" {
		t.Fatalf("publish did not project catalogue before state transition: run=%#v publisher=%#v", updated, publisher.run)
	}
}

func TestEvidenceReviewCanChooseExcludeRestoreAndCorrect(t *testing.T) {
	store := NewMemoryStore()
	run := Run{ID: "ing_resolution", State: "needs_review", Issues: []string{"Conflicting evidence for Address"}, Fields: []EvidenceField{
		{Key: "Address", Value: "Jl. Wrong", Source: "Web", SourceURL: "https://one.example", Confidence: .7, Conflict: true},
		{Key: "Address", Value: "Jl. Correct", Source: "Google Maps", SourceURL: "https://two.example", Confidence: .97, Conflict: true},
	}}
	if err := store.Save(t.Context(), run); err != nil {
		t.Fatal(err)
	}
	service := NewService(store, &fakeQueue{})
	loaded, err := service.Get(t.Context(), run.ID)
	if err != nil || loaded.Fields[0].ID == "" || loaded.Fields[1].ID == "" {
		t.Fatalf("evidence IDs were not assigned: run=%#v err=%v", loaded, err)
	}
	resolved, err := service.ChooseEvidence(t.Context(), run.ID, loaded.Fields[1].ID)
	if err != nil || len(resolved.Issues) != 0 || !resolved.Fields[0].Excluded || resolved.Fields[1].Excluded {
		t.Fatalf("conflict was not resolved by choosing evidence: run=%#v err=%v", resolved, err)
	}
	restored, err := service.SetEvidenceExcluded(t.Context(), run.ID, resolved.Fields[0].ID, false)
	if err != nil || len(restored.Issues) != 1 {
		t.Fatalf("restoring conflicting evidence should restore the conflict: run=%#v err=%v", restored, err)
	}
	corrected, err := service.ReplaceEvidence(t.Context(), run.ID, restored.Fields[0].ID, "Jl. Correct")
	if err != nil || len(corrected.Issues) != 0 || !corrected.Fields[0].Excluded || corrected.Fields[len(corrected.Fields)-1].Method != "manual" {
		t.Fatalf("manual correction did not preserve and exclude the source: run=%#v err=%v", corrected, err)
	}
}

func TestNormalizeExistingRunMigratesGenericSpendBand(t *testing.T) {
	run := normalizeRunEvidence(Run{Fields: []EvidenceField{
		{Key: "Minimum price", Value: "Rp25000", SourceURL: "https://maps.google.com/place", Method: "api", Extractor: "google_places", Confidence: .85},
		{Key: "Minimum price", Value: "Rp100000", SourceURL: "https://listing.example/place", Excerpt: "$$ · Price Range 100000 - 250000 IDR per person", Confidence: .72},
		{Key: "Maximum price", Value: "Rp250000", SourceURL: "https://listing.example/place", Excerpt: "$$ · Price Range 100000 - 250000 IDR per person", Confidence: .72},
	}})
	if run.Fields[1].Key != "Typical spend minimum" || run.Fields[2].Key != "Typical spend maximum" || len(run.Issues) != 0 {
		t.Fatalf("legacy generic spend evidence was not migrated cleanly: %#v", run)
	}
}

func TestProcessorExtractsEvidenceAndMovesToReview(t *testing.T) {
	store := NewMemoryStore()
	run := Run{ID: "ing_test", URL: "https://example.com/cafe", State: "enriching", Issues: []string{}, Fields: []EvidenceField{}}
	if err := store.Save(t.Context(), run); err != nil {
		t.Fatal(err)
	}
	processor := NewProcessor(store, fakeExtractor{result: ExtractedRecord{
		Name: "Kopi Test", Confidence: .9,
		Fields: []EvidenceField{{Key: "Identity", Value: "Kopi Test", Source: "Page metadata", SourceURL: run.URL, Confidence: .9}},
	}}, fakePlaceResolver{placeID: "place-123", place: googleplaces.Place{GoogleMapsURI: "https://maps.google.com/place", Rating: 4.8, UserRatingCount: 20, Reviews: []googleplaces.Review{{Rating: 5, Text: googleplaces.LocalizedText{Text: "Wi-Fi cepat dan banyak colokan"}, AuthorAttribution: googleplaces.AuthorAttribution{DisplayName: "Reviewer"}, GoogleMapsURI: "https://maps.google.com/review/1"}}}}, fakeDiscoverer{result: []DiscoveredSource{{Title: "Kopi Test menu", URL: "https://menu.example/menu", Excerpt: "Coffee and Wi-Fi"}}}, nil)
	payload, _ := json.Marshal(taskPayload{RunID: run.ID, URL: run.URL})
	if err := processor.Handle(t.Context(), asynq.NewTask(TaskTypeEnrich, payload)); err != nil {
		t.Fatal(err)
	}
	updated, err := store.Get(t.Context(), run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.State != "needs_review" || updated.Progress != 100 || updated.Name != "Kopi Test" || len(updated.Fields) < 7 {
		t.Fatalf("unexpected processed run: %#v", updated)
	}
	foundExa := false
	for _, field := range updated.Fields {
		foundExa = foundExa || (field.SourceURL == "https://menu.example/menu" && field.Excerpt != "")
	}
	if !foundExa {
		t.Fatalf("Exa evidence missing: %#v", updated.Fields)
	}
	if updated.GooglePlaceID != "place-123" {
		t.Fatalf("stable Google Place ID was not stored: %#v", updated)
	}
}

func TestDiscoveryFailureDoesNotFailStaticIngestion(t *testing.T) {
	store := NewMemoryStore()
	run := Run{ID: "ing_warning", URL: "https://example.com", State: "enriching", Issues: []string{}, Warnings: []string{}, Fields: []EvidenceField{}}
	if err := store.Save(t.Context(), run); err != nil {
		t.Fatal(err)
	}
	processor := NewProcessor(store, fakeExtractor{result: ExtractedRecord{Name: "Kopi Test", Confidence: .8}}, nil, fakeDiscoverer{err: errors.New("temporary outage")}, nil)
	payload, _ := json.Marshal(taskPayload{RunID: run.ID, URL: run.URL})
	if err := processor.Handle(t.Context(), asynq.NewTask(TaskTypeEnrich, payload)); err != nil {
		t.Fatal(err)
	}
	updated, err := store.Get(t.Context(), run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.State != "needs_review" || len(updated.Issues) != 0 || len(updated.Warnings) != 1 {
		t.Fatalf("discovery failure should be non-blocking: %#v", updated)
	}
}

func TestGoogleReviewFeedsCitedStructuredExtraction(t *testing.T) {
	store := NewMemoryStore()
	run := Run{ID: "ing_google_review", URL: "https://maps.app.goo.gl/test", State: "enriching", Issues: []string{}, Warnings: []string{}, Fields: []EvidenceField{}}
	if err := store.Save(t.Context(), run); err != nil {
		t.Fatal(err)
	}
	reviewText := "Wi-Fi cepat dan tersedia banyak colokan dekat meja."
	places := fakePlaceResolver{placeID: "place-123", place: googleplaces.Place{
		ID: "place-123", GoogleMapsURI: "https://maps.google.com/place", Reviews: []googleplaces.Review{{
			Name: "places/place-123/reviews/review-1", Rating: 5, OriginalText: googleplaces.LocalizedText{Text: reviewText},
			AuthorAttribution: googleplaces.AuthorAttribution{DisplayName: "Reviewer", URI: "https://maps.google.com/user", PhotoURI: "https://example.com/avatar.jpg"},
			GoogleMapsURI:     "https://maps.google.com/review/1",
		}},
	}}
	model := &fakeEvidenceExtractor{result: ModelExtraction{Model: "test-model", Claims: []ModelClaim{{
		Field: "facilities.wifi", Value: "yes", SourceID: "google_review_1", Excerpt: "Wi-Fi cepat", Confidence: .95,
	}}, MenuItems: []ModelMenuItem{}}}
	processor := NewProcessor(store, fakeExtractor{result: ExtractedRecord{
		Name: "Kopi Test", Confidence: .8, Fields: []EvidenceField{{Key: "Identity", Value: "Kopi Test", Confidence: .8}},
		Documents: []SourceDocument{{ID: "source_1", URL: run.URL, Kind: "html", Content: "Kopi Test"}},
	}}, places, nil, model)
	payload, _ := json.Marshal(taskPayload{RunID: run.ID, URL: run.URL})
	if err := processor.Handle(t.Context(), asynq.NewTask(TaskTypeEnrich, payload)); err != nil {
		t.Fatal(err)
	}
	if len(model.input.Documents) != 2 || model.input.Documents[1].Kind != "google_places_review" || model.input.Documents[1].Content != reviewText {
		t.Fatalf("Google review did not reach structured extraction: %#v", model.input.Documents)
	}
	updated, err := store.Get(t.Context(), run.ID)
	if err != nil {
		t.Fatal(err)
	}
	var storedReview, derivedWiFi bool
	for _, field := range updated.Fields {
		storedReview = storedReview || (field.Key == "Google review" && field.AuthorName == "Reviewer" && field.SourceURL == "https://maps.google.com/review/1")
		derivedWiFi = derivedWiFi || (field.Key == "Wi-Fi" && field.Value == "Yes" && field.SourceID == "google_review_1")
	}
	if !storedReview || !derivedWiFi {
		t.Fatalf("review evidence or derived claim missing: %#v", updated.Fields)
	}
}

func TestExtractHTMLMetadataAndJSONLD(t *testing.T) {
	document := `<html><head><title>Kopi Test | Jakarta</title><meta name="description" content="Quiet work cafe"><script type="application/ld+json">{"@type":"CafeOrCoffeeShop","name":"Kopi Structured","address":{"streetAddress":"Jl. Test 1","addressLocality":"Jakarta"},"openingHours":["Mo-Su 00:00-23:59"]}</script></head></html>`
	record := extractHTML([]byte(document), "https://example.com")
	if record.Name != "Kopi Test" || record.Confidence != .9 || len(record.Fields) < 4 {
		t.Fatalf("unexpected extraction: %#v", record)
	}
	joined := ""
	for _, field := range record.Fields {
		joined += field.Key + ":" + field.Value + "\n"
	}
	if !strings.Contains(joined, "Address:Jl. Test 1, Jakarta") || !strings.Contains(joined, "Opening hours:Mo-Su 00:00-23:59") {
		t.Fatalf("structured evidence missing:\n%s", joined)
	}
}

func TestExtractGoogleMapsIdentityFromResolvedPlaceURL(t *testing.T) {
	document := `<html><head><title>Google Maps</title><meta name="description" content="Mencari bisnis lokal, melihat peta, dan melihat rute di Google Maps."></head><body>Search locations on Google Maps</body></html>`
	resolvedURL := "https://www.google.com/maps/place/TAAM+House+-+Cipete/@-6.2777973,106.7981242,17z/data=!3m1!4b1"
	record := extractHTML([]byte(document), resolvedURL)
	if record.Name != "TAAM House - Cipete" {
		t.Fatalf("expected place identity from resolved Maps URL, got %q", record.Name)
	}
	if len(record.Fields) != 2 || record.Fields[0].Key != "Identity" || record.Fields[0].Extractor != "google_maps_url" {
		t.Fatalf("unexpected Google Maps evidence: %#v", record.Fields)
	}
	if record.Fields[0].Value != "TAAM House - Cipete" || record.Fields[0].Source != "Google Maps URL" {
		t.Fatalf("generic Maps metadata was used as identity: %#v", record.Fields[0])
	}
	if !strings.Contains(record.Documents[0].Content, "Google Maps place from resolved URL: TAAM House - Cipete") {
		t.Fatalf("place identity missing from extraction document: %#v", record.Documents[0])
	}
}

func TestGoogleMapsPlaceNameDecodesEscapedPath(t *testing.T) {
	name := googleMapsPlaceName("https://www.google.co.id/maps/place/Kopi+Tuku+%E2%80%93+Cipete/data=!4m6")
	if name != "Kopi Tuku – Cipete" {
		t.Fatalf("unexpected decoded Maps place name: %q", name)
	}
}

func TestSourceURLRejectsCredentials(t *testing.T) {
	if _, err := parseSourceURL("https://user:pass@example.com/cafe"); err == nil {
		t.Fatal("expected credential-bearing URL to be rejected")
	}
}
