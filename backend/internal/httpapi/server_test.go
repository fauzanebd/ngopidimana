package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
