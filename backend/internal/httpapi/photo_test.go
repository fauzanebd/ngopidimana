package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/fauzanebd/wheretowfc/backend/internal/googleplaces"
	"github.com/fauzanebd/wheretowfc/backend/internal/ingestion"
)

// photoGooglePlaces is a Google Places stand-in that counts calls, so the tests
// can tell a cached answer from a fresh round trip.
type photoGooglePlaces struct {
	place       googleplaces.Place
	mediaURL    string
	mediaErrors map[string]error
	placeError  error
	placeCalls  int
	mediaCalls  int
}

func (fake *photoGooglePlaces) Enabled() bool { return true }

func (fake *photoGooglePlaces) GetPlace(context.Context, string) (googleplaces.Place, error) {
	fake.placeCalls++
	if fake.placeError != nil {
		return googleplaces.Place{}, fake.placeError
	}
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

func TestPlacePhotoRejectedIDIsNotFoundNotAGatewayError(t *testing.T) {
	// Google answers 400 INVALID_ARGUMENT for an id it will not accept, which is
	// the same user-facing answer as a place that simply has no photos.
	fake := &photoGooglePlaces{
		place:       googleplaces.Place{ID: "ChIJbad"},
		mediaErrors: map[string]error{},
	}
	fake.placeError = &googleplaces.APIError{StatusCode: http.StatusBadRequest, Status: "INVALID_ARGUMENT", Message: "The provided Place ID is not valid."}
	if response := getPhoto(t, photoServer(fake), "/v1/places/ChIJbad/photo"); response.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 for an id Google rejects", response.Code)
	}

	// An outage is not a missing photo, and must not be silently swallowed.
	fake.placeError = &googleplaces.APIError{StatusCode: http.StatusTooManyRequests, Status: "RESOURCE_EXHAUSTED", Message: "Quota exceeded"}
	if response := getPhoto(t, photoServer(fake), "/v1/places/ChIJbad/photo"); response.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502 when Google refuses us", response.Code)
	}
}

// fakePhotoStore stands in for the catalogue's stored references.
type fakePhotoStore struct {
	stored    map[string]googleplaces.PhotoRef
	overrides map[string]ingestion.PhotoOverride
	saves     int
	lastSave  googleplaces.PhotoRef
}

func (store *fakePhotoStore) PhotoOverride(_ context.Context, googlePlaceID string) (ingestion.PhotoOverride, bool, error) {
	override, found := store.overrides[googlePlaceID]
	return override, found, nil
}

func (store *fakePhotoStore) PhotoRef(_ context.Context, googlePlaceID string) (googleplaces.PhotoRef, bool, error) {
	reference, found := store.stored[googlePlaceID]
	return reference, found, nil
}

func (store *fakePhotoStore) SavePhotoRef(_ context.Context, googlePlaceID string, reference googleplaces.PhotoRef) error {
	if store.stored == nil {
		store.stored = map[string]googleplaces.PhotoRef{}
	}
	store.stored[googlePlaceID] = reference
	store.saves++
	store.lastSave = reference
	return nil
}

func TestPlacePhotoUsesTheStoredReferenceWithoutALookup(t *testing.T) {
	// This is the whole point of storing the reference: rendering costs one media
	// call ($7/1000) and no Place Details lookup ($20/1000).
	fake := &photoGooglePlaces{mediaURL: "https://lh3.googleusercontent.com/abc"}
	store := &fakePhotoStore{stored: map[string]googleplaces.PhotoRef{
		"ChIJtest": {Name: "places/ChIJtest/photos/AeJ", AttributionName: "Tika Anggi", AttributionURI: "https://maps.google.com/contrib/1"},
	}}
	server := photoServer(fake)
	server.photoStore = store

	response := getPhoto(t, server, "/v1/places/ChIJtest/photo")
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", response.Code, response.Body.String())
	}
	var photo placePhoto
	if err := json.Unmarshal(response.Body.Bytes(), &photo); err != nil {
		t.Fatal(err)
	}
	if photo.URL != "https://lh3.googleusercontent.com/abc" || photo.Attribution.Name != "Tika Anggi" {
		t.Fatalf("photo = %#v", photo)
	}
	if fake.placeCalls != 0 {
		t.Fatalf("stored reference still cost %d Place Details lookups, want 0", fake.placeCalls)
	}
	if fake.mediaCalls != 1 {
		t.Fatalf("media calls = %d, want 1", fake.mediaCalls)
	}
}

func TestPlacePhotoRefreshesAnExpiredStoredReference(t *testing.T) {
	fake := &photoGooglePlaces{
		place: googleplaces.Place{ID: "ChIJtest", Photos: []googleplaces.PlacePhoto{{
			Name: "places/ChIJtest/photos/fresh", AuthorAttributions: []googleplaces.AuthorAttribution{{DisplayName: "New Author", URI: "https://maps.google.com/contrib/2"}},
		}}},
		mediaURL:    "https://lh3.googleusercontent.com/fresh",
		mediaErrors: map[string]error{"places/ChIJtest/photos/stale": errors.New("expired photo name")},
	}
	store := &fakePhotoStore{stored: map[string]googleplaces.PhotoRef{
		"ChIJtest": {Name: "places/ChIJtest/photos/stale", AttributionName: "Old Author"},
	}}
	server := photoServer(fake)
	server.photoStore = store

	response := getPhoto(t, server, "/v1/places/ChIJtest/photo")
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", response.Code, response.Body.String())
	}
	var photo placePhoto
	if err := json.Unmarshal(response.Body.Bytes(), &photo); err != nil {
		t.Fatal(err)
	}
	if photo.URL != "https://lh3.googleusercontent.com/fresh" || photo.Attribution.Name != "New Author" {
		t.Fatalf("photo = %#v", photo)
	}
	if fake.placeCalls != 1 || fake.mediaCalls != 2 {
		t.Fatalf("calls = place:%d media:%d, want 1 lookup then 2 media attempts", fake.placeCalls, fake.mediaCalls)
	}
	// The refreshed name is kept, so the next render is cheap again.
	if store.saves != 1 || store.lastSave.Name != "places/ChIJtest/photos/fresh" || store.lastSave.AttributionName != "New Author" {
		t.Fatalf("stored = %#v after %d saves", store.lastSave, store.saves)
	}
}

func TestOwnedPhotoOverridesGoogleAndCostsNothing(t *testing.T) {
	fake := &photoGooglePlaces{place: googleplaces.Place{ID: "ChIJtest"}, mediaURL: "https://lh3.googleusercontent.com/google"}
	store := &fakePhotoStore{overrides: map[string]ingestion.PhotoOverride{
		"ChIJtest": {URL: "https://cdn.pengenkekopi.shop/tis-my-cafe.jpg", Attribution: "Venue photo"},
	}}
	server := photoServer(fake)
	server.photoStore = store

	response := getPhoto(t, server, "/v1/places/ChIJtest/photo")
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", response.Code, response.Body.String())
	}
	var photo placePhoto
	if err := json.Unmarshal(response.Body.Bytes(), &photo); err != nil {
		t.Fatal(err)
	}
	if photo.URL != "https://cdn.pengenkekopi.shop/tis-my-cafe.jpg" || photo.Attribution.Name != "Venue photo" {
		t.Fatalf("photo = %#v, want the owned image", photo)
	}
	if fake.placeCalls != 0 || fake.mediaCalls != 0 {
		t.Fatalf("an owned photo cost Google calls: place=%d media=%d", fake.placeCalls, fake.mediaCalls)
	}
}

func TestOwnedPhotoWorksWithoutGoogleConfigured(t *testing.T) {
	// An owned photo needs no API key, which is exactly what you want when the key
	// is absent (local dev) or the quota is exhausted.
	store := &fakePhotoStore{overrides: map[string]ingestion.PhotoOverride{
		"ChIJtest": {URL: "https://cdn.pengenkekopi.shop/x.jpg"},
	}}
	server := &Server{}
	server.photoStore = store
	if response := getPhoto(t, server, "/v1/places/ChIJtest/photo"); response.Code != http.StatusOK {
		t.Fatalf("status = %d, want the owned photo even with no Google client", response.Code)
	}
}

func postRefresh(t *testing.T, server *Server, placeID string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, "/v1/places/"+placeID+"/photo/refresh", nil)
	request.SetPathValue("googlePlaceID", placeID)
	response := httptest.NewRecorder()
	server.refreshPlacePhoto(response, request)
	return response
}

func TestRefreshReResolvesOnceThenBacksOff(t *testing.T) {
	fake := &photoGooglePlaces{
		place:    googleplaces.Place{ID: "ChIJtest", Photos: []googleplaces.PlacePhoto{{Name: "places/ChIJtest/photos/a"}}},
		mediaURL: "https://lh3.googleusercontent.com/fresh",
	}
	server := photoServer(fake)

	if response := postRefresh(t, server, "ChIJtest"); response.Code != http.StatusOK {
		t.Fatalf("first refresh = %d: %s", response.Code, response.Body.String())
	}
	// A client stuck in a retry loop must not be able to spend the budget: the URL
	// was just resolved, so a second report is refused.
	if response := postRefresh(t, server, "ChIJtest"); response.Code != http.StatusTooManyRequests {
		t.Fatalf("second refresh = %d, want 429", response.Code)
	}
	if fake.mediaCalls != 1 {
		t.Fatalf("media calls = %d, want exactly one", fake.mediaCalls)
	}
}

func TestRefreshDailyBudgetBoundsTheEndpoint(t *testing.T) {
	fake := &photoGooglePlaces{
		place:    googleplaces.Place{ID: "ChIJtest", Photos: []googleplaces.PlacePhoto{{Name: "places/ChIJtest/photos/a"}}},
		mediaURL: "https://lh3.googleusercontent.com/fresh",
	}
	server := photoServer(fake)
	now := time.Now()
	// Spend the day's budget on distinct places, then confirm the next is refused.
	for index := range photoRefreshesPerDay {
		server.photos.beginRefresh(fmt.Sprintf("ChIJ%d", index), now)
	}
	if server.photos.beginRefresh("ChIJnew", now) {
		t.Fatal("the daily refresh budget did not bound the endpoint")
	}
}
