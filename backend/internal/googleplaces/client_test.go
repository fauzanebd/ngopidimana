package googleplaces

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestResolvePlaceIDSelectsExactVenueAndStoresNoContent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/v1/places:searchText" || request.Method != http.MethodPost {
			t.Fatalf("unexpected request: %s %s", request.Method, request.URL.Path)
		}
		if request.Header.Get("X-Goog-Api-Key") != "test-key" {
			t.Fatal("missing API key header")
		}
		if request.Header.Get("X-Goog-FieldMask") != "places.id,places.displayName" {
			t.Fatalf("unexpected field mask: %q", request.Header.Get("X-Goog-FieldMask"))
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"places":[{"id":"wrong","displayName":{"text":"TAAM Rawamangun"}},{"id":"right","displayName":{"text":"TAAM House - Cipete"}}]}`))
	}))
	defer server.Close()

	client := NewClient("test-key", server.URL, "id", "ID", "Jakarta", time.Second)
	placeID, err := client.ResolvePlaceID(context.Background(), "TAAM House - Cipete", "Jakarta Selatan")
	if err != nil {
		t.Fatal(err)
	}
	if placeID != "right" {
		t.Fatalf("resolved wrong place: %q", placeID)
	}
}

func TestGetPlaceRequestsOnlyDisplayFields(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/v1/places/place-123" || request.URL.Query().Get("languageCode") != "id" {
			t.Fatalf("unexpected details URL: %s", request.URL.String())
		}
		mask := request.Header.Get("X-Goog-FieldMask")
		if strings.Contains(mask, "reviewSummary") || !strings.Contains(mask, "reviews") {
			t.Fatalf("unexpected details field mask: %q", mask)
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"id":"place-123","displayName":{"text":"Kopi Test"},"googleMapsUri":"https://maps.google.com/test","rating":4.7,"userRatingCount":42,"reviews":[{"text":{"text":"Enak"},"rating":5,"authorAttribution":{"displayName":"A","uri":"https://maps.google.com/a","photoUri":"https://example.com/a.jpg"},"googleMapsUri":"https://maps.google.com/review"}]}`))
	}))
	defer server.Close()

	client := NewClient("test-key", server.URL, "id", "ID", "Jakarta", time.Second)
	place, err := client.GetPlace(context.Background(), "place-123")
	if err != nil {
		t.Fatal(err)
	}
	if place.DisplayName.Text != "Kopi Test" || len(place.Reviews) != 1 || place.Reviews[0].Text.Text != "Enak" {
		t.Fatalf("unexpected place response: %#v", place)
	}
}

func TestDisabledClientRejectsRequests(t *testing.T) {
	client := NewClient("", "", "", "", "", 0)
	if client.Enabled() {
		t.Fatal("client without key should be disabled")
	}
	if _, err := client.GetPlace(context.Background(), "place"); err != ErrNotConfigured {
		t.Fatalf("expected not configured, got %v", err)
	}
}
