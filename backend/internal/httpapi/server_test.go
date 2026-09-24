package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/fauzanebd/wheretowfc/backend/internal/googleplaces"
	"github.com/fauzanebd/wheretowfc/backend/internal/ingestion"
)

type testQueue struct{}

func (testQueue) Enqueue(context.Context, string, string) error { return nil }

type testGooglePlaces struct{ place googleplaces.Place }

func (testGooglePlaces) Enabled() bool { return true }
func (provider testGooglePlaces) GetPlace(context.Context, string) (googleplaces.Place, error) {
	return provider.place, nil
}
func (testGooglePlaces) PhotoMedia(context.Context, string, int) (string, error) {
	return "", googleplaces.ErrPlaceNotFound
}

func TestGetGooglePlaceReturnsUncachedLiveDetails(t *testing.T) {
	store := ingestion.NewMemoryStore()
	run := ingestion.Run{ID: "ing_test", GooglePlaceID: "place-123", Issues: []string{}, Warnings: []string{}, Fields: []ingestion.EvidenceField{}}
	if err := store.Save(t.Context(), run); err != nil {
		t.Fatal(err)
	}
	server := &Server{
		ingestion: ingestion.NewService(store, testQueue{}),
		googlePlaces: testGooglePlaces{place: googleplaces.Place{
			ID: "place-123", DisplayName: googleplaces.LocalizedText{Text: "Kopi Test"}, Reviews: []googleplaces.Review{},
		}},
	}
	request := httptest.NewRequest(http.MethodGet, "/v1/admin/ingestion-runs/ing_test/google-place", nil)
	request.SetPathValue("id", "ing_test")
	response := httptest.NewRecorder()
	server.getGooglePlace(response, request)

	if response.Code != http.StatusOK || response.Header().Get("Cache-Control") != "no-store, max-age=0" {
		t.Fatalf("unexpected response: status=%d cache=%q body=%s", response.Code, response.Header().Get("Cache-Control"), response.Body.String())
	}
	var place googleplaces.Place
	if err := json.Unmarshal(response.Body.Bytes(), &place); err != nil {
		t.Fatal(err)
	}
	if place.ID != "place-123" || place.DisplayName.Text != "Kopi Test" {
		t.Fatalf("unexpected place: %#v", place)
	}
}

// The admin's duplicate dialog is driven by this response: the status it branches on and the
// record it opens. Both are part of the contract, so both are asserted here.
func TestCreateIngestionRunRefusesADuplicateWithConflict(t *testing.T) {
	store := ingestion.NewMemoryStore()
	server := &Server{ingestion: ingestion.NewService(store, testQueue{})}

	first := postIngestionRun(t, server, `{"url":"https://example.com/cafe"}`)
	if first.Code != http.StatusAccepted {
		t.Fatalf("first submission should be accepted: status=%d body=%s", first.Code, first.Body.String())
	}
	var created ingestion.Run
	if err := json.Unmarshal(first.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}

	second := postIngestionRun(t, server, `{"url":"https://example.com/cafe"}`)
	if second.Code != http.StatusConflict {
		t.Fatalf("a duplicate should be refused with 409: status=%d body=%s", second.Code, second.Body.String())
	}
	var refusal struct {
		Error    string        `json:"error"`
		Existing ingestion.Run `json:"existing"`
	}
	if err := json.Unmarshal(second.Body.Bytes(), &refusal); err != nil {
		t.Fatal(err)
	}
	if refusal.Existing.ID != created.ID || refusal.Error == "" {
		t.Fatalf("the refusal must carry the existing record: %#v", refusal)
	}
	runs, err := store.List(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 1 {
		t.Fatalf("a refused duplicate must not be stored: %#v", runs)
	}

	forced := postIngestionRun(t, server, `{"url":"https://example.com/cafe","force":true}`)
	if forced.Code != http.StatusAccepted {
		t.Fatalf("force should still queue the crawl the admin asked for: status=%d body=%s", forced.Code, forced.Body.String())
	}
	if runs, err = store.List(t.Context()); err != nil || len(runs) != 2 {
		t.Fatalf("a forced duplicate should be stored: runs=%#v err=%v", runs, err)
	}
}

func postIngestionRun(t *testing.T, server *Server, body string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, "/v1/admin/ingestion-runs", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	server.createIngestionRun(response, request)
	return response
}
