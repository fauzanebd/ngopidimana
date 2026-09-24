package googleplaces

import (
	"context"
	"errors"
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

func TestPhotoMediaResolvesShortLivedURLWithoutFetchingBytes(t *testing.T) {
	var seenPath, seenQuery string
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		seenPath, seenQuery = request.URL.Path, request.URL.RawQuery
		if request.Header.Get("X-Goog-Api-Key") != "test-key" {
			t.Fatal("missing API key header")
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"name":"places/place-123/photos/AeJ","photoUri":"https://lh3.googleusercontent.com/abc"}`))
	}))
	defer server.Close()

	client := NewClient("test-key", server.URL, "id", "ID", "Jakarta", time.Second)
	photoURL, err := client.PhotoMedia(context.Background(), "places/place-123/photos/AeJ", 800)
	if err != nil {
		t.Fatal(err)
	}
	if photoURL != "https://lh3.googleusercontent.com/abc" {
		t.Fatalf("photo url = %q", photoURL)
	}
	if seenPath != "/v1/places/place-123/photos/AeJ/media" {
		t.Fatalf("request path = %q", seenPath)
	}
	// skipHttpRedirect keeps the response JSON so the bytes are never pulled
	// through this service, and the width keeps the media small.
	if !strings.Contains(seenQuery, "skipHttpRedirect=true") || !strings.Contains(seenQuery, "maxWidthPx=800") {
		t.Fatalf("request query = %q", seenQuery)
	}
}

func TestPhotoMediaRejectsNamesThatAreNotPhotoResources(t *testing.T) {
	client := NewClient("test-key", "https://places.googleapis.com", "id", "ID", "Jakarta", time.Second)
	for _, name := range []string{
		"", "places/place-123", "places//photos/AeJ", "places/place-123/photos/",
		"places/place-123/photos/AeJ/extra", "../../etc/passwd", "places/place-123/photos/AeJ?x=1",
	} {
		if _, err := client.PhotoMedia(context.Background(), name, 800); err == nil {
			t.Fatalf("PhotoMedia accepted %q", name)
		}
	}
}

func TestNotFoundIsReportedAsAMissingPlace(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusNotFound)
		_, _ = writer.Write([]byte(`{"error":{"message":"Place not found","status":"NOT_FOUND"}}`))
	}))
	defer server.Close()

	client := NewClient("test-key", server.URL, "id", "ID", "Jakarta", time.Second)
	if _, err := client.GetPlace(context.Background(), "ChIJmissing"); !errors.Is(err, ErrPlaceNotFound) {
		t.Fatalf("GetPlace on a missing id = %v, want ErrPlaceNotFound", err)
	}
}

func TestPrimaryPhotoSkipsUnusablePhotosAndKeepsAttribution(t *testing.T) {
	place := Place{Photos: []PlacePhoto{
		{Name: "   "},
		{Name: "places/p/photos/first", AuthorAttributions: []AuthorAttribution{{DisplayName: "  "}, {DisplayName: "Tika Anggi", URI: "https://maps.google.com/contrib/1"}}},
		{Name: "places/p/photos/second"},
	}}
	reference := place.PrimaryPhoto()
	if reference == nil || reference.Name != "places/p/photos/first" {
		t.Fatalf("primary photo = %#v, want the first photo with a reference", reference)
	}
	if reference.AttributionName != "Tika Anggi" || reference.AttributionURI != "https://maps.google.com/contrib/1" {
		t.Fatalf("attribution = %#v, want the first author with a name", reference)
	}
	if references := place.PhotoRefs(); len(references) != 2 {
		t.Fatalf("usable references = %d, want 2 (the nameless one skipped)", len(references))
	}
	if empty := (Place{}).PrimaryPhoto(); empty != nil {
		t.Fatalf("a place with no photos returned %#v", empty)
	}
}
