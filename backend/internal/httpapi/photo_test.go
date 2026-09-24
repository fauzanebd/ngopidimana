package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/fauzanebd/wheretowfc/backend/internal/googleplaces"
)

// photoGooglePlaces is a Google Places stand-in that counts calls, so the tests
// can tell a cached answer from a fresh round trip.
type photoGooglePlaces struct {
	place       googleplaces.Place
	mediaURL    string
	mediaErrors map[string]error
	placeCalls  int
	mediaCalls  int
}

func (fake *photoGooglePlaces) Enabled() bool { return true }

func (fake *photoGooglePlaces) GetPlace(context.Context, string) (googleplaces.Place, error) {
	fake.placeCalls++
	if fake.place.ID == "" {
		return googleplaces.Place{}, googleplaces.ErrPlaceNotFound
	}
	return fake.place, nil
}

func (fake *photoGooglePlaces) PhotoMedia(_ context.Context, name string, _ int) (string, error) {
	fake.mediaCalls++
	if err, found := fake.mediaErrors[name]; found {
		return "", err
	}
	return fake.mediaURL, nil
}

func photoServer(fake *photoGooglePlaces) *Server { return &Server{googlePlaces: fake} }

func getPhoto(t *testing.T, server *Server, path string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodGet, path, nil)
	request.SetPathValue("googlePlaceID", placeIDFromPath(path))
	response := httptest.NewRecorder()
	server.placePhotoHandler(response, request)
	return response
}

// placeIDFromPath pulls the id out of `/v1/places/{id}/photo`.
func placeIDFromPath(path string) string {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) < 2 {
		return ""
	}
	return parts[len(parts)-2]
}

func TestPlacePhotoReturnsURLAndRequiredAttribution(t *testing.T) {
	fake := &photoGooglePlaces{
		place: googleplaces.Place{ID: "ChIJtest", Photos: []googleplaces.PlacePhoto{{
			Name: "places/ChIJtest/photos/AeJ", AuthorAttributions: []googleplaces.AuthorAttribution{{DisplayName: "Jane Doe", URI: "https://maps.google.com/contrib/1"}},
		}}},
		mediaURL: "https://lh3.googleusercontent.com/abc",
	}
	response := getPhoto(t, photoServer(fake), "/v1/places/ChIJtest/photo")
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", response.Code, response.Body.String())
	}
	var photo placePhoto
	if err := json.Unmarshal(response.Body.Bytes(), &photo); err != nil {
		t.Fatal(err)
	}
	if photo.URL != "https://lh3.googleusercontent.com/abc" {
		t.Fatalf("photo url = %q", photo.URL)
	}
	if photo.Attribution.Name != "Jane Doe" || photo.Attribution.URI != "https://maps.google.com/contrib/1" {
		t.Fatalf("attribution = %#v, want the photo author", photo.Attribution)
	}
	if fake.placeCalls != 1 || fake.mediaCalls != 1 {
		t.Fatalf("calls = place:%d media:%d, want 1/1", fake.placeCalls, fake.mediaCalls)
	}
}

func TestPlacePhotoIsServedFromCacheOnRepeatViews(t *testing.T) {
	fake := &photoGooglePlaces{
		place:    googleplaces.Place{ID: "ChIJtest", Photos: []googleplaces.PlacePhoto{{Name: "places/ChIJtest/photos/AeJ"}}},
		mediaURL: "https://lh3.googleusercontent.com/abc",
	}
	server := photoServer(fake)
	for range 3 {
		if response := getPhoto(t, server, "/v1/places/ChIJtest/photo"); response.Code != http.StatusOK {
			t.Fatalf("status = %d", response.Code)
		}
	}
	if fake.placeCalls != 1 || fake.mediaCalls != 1 {
		t.Fatalf("calls = place:%d media:%d, want a single round trip for three views", fake.placeCalls, fake.mediaCalls)
	}
}

func TestPlacePhotoFallsBackToTheNextPhoto(t *testing.T) {
	fake := &photoGooglePlaces{
		place: googleplaces.Place{ID: "ChIJtest", Photos: []googleplaces.PlacePhoto{
			{Name: "places/ChIJtest/photos/broken"},
			{Name: "places/ChIJtest/photos/working"},
		}},
		mediaURL:    "https://lh3.googleusercontent.com/working",
		mediaErrors: map[string]error{"places/ChIJtest/photos/broken": errors.New("expired photo name")},
	}
	response := getPhoto(t, photoServer(fake), "/v1/places/ChIJtest/photo")
	if response.Code != http.StatusOK || !json.Valid(response.Body.Bytes()) {
		t.Fatalf("status = %d body = %s", response.Code, response.Body.String())
	}
	var photo placePhoto
	if err := json.Unmarshal(response.Body.Bytes(), &photo); err != nil {
		t.Fatal(err)
	}
	if photo.URL != "https://lh3.googleusercontent.com/working" || fake.mediaCalls != 2 {
		t.Fatalf("photo = %#v after %d media calls", photo, fake.mediaCalls)
	}
}

func TestPlacePhotoWithoutPhotosIsNotFoundNotAnError(t *testing.T) {
	empty := &photoGooglePlaces{place: googleplaces.Place{ID: "ChIJtest"}}
	response := getPhoto(t, photoServer(empty), "/v1/places/ChIJtest/photo")
	if response.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 for a place with no photos", response.Code)
	}
	unknown := &photoGooglePlaces{}
	if response := getPhoto(t, photoServer(unknown), "/v1/places/ChIJmissing/photo"); response.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 for an unknown place", response.Code)
	}
}

func TestPlacePhotoWithoutGooglePlacesConfigured(t *testing.T) {
	response := getPhoto(t, &Server{}, "/v1/places/ChIJtest/photo")
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", response.Code)
	}
}

func TestPlacePhotoRejectsMalformedPlaceIDs(t *testing.T) {
	fake := &photoGooglePlaces{place: googleplaces.Place{ID: "ChIJtest"}, mediaURL: "https://lh3.googleusercontent.com/abc"}
	server := photoServer(fake)
	for _, id := range []string{"", "../../etc/passwd", "ChIJtest/extra"} {
		request := httptest.NewRequest(http.MethodGet, "/v1/places/photo", nil)
		request.SetPathValue("googlePlaceID", id)
		response := httptest.NewRecorder()
		server.placePhotoHandler(response, request)
		if response.Code != http.StatusNotFound {
			t.Fatalf("id %q = %d, want 404", id, response.Code)
		}
	}
	if fake.placeCalls != 0 {
		t.Fatalf("malformed ids reached Google %d times", fake.placeCalls)
	}
}
