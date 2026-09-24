package httpapi

import (
	"context"
	"errors"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/fauzanebd/wheretowfc/backend/internal/googleplaces"
)

// photoCacheTTL bounds how long a resolved photo URL is reused. Google's photo
// URLs are short-lived and the Places policies forbid storing place content, so
// this is a courtesy cache that avoids a second API round trip for a card the
// visitor just scrolled past — not a content store.
const (
	photoCacheTTL       = 10 * time.Minute
	photoCacheMax       = 512
	photoDefaultWidthPx = 800
)

type photoAttribution struct {
	Name string `json:"name"`
	URI  string `json:"uri"`
}

type placePhoto struct {
	URL         string           `json:"photo_url"`
	Attribution photoAttribution `json:"attribution"`
}

type photoCacheEntry struct {
	photo     placePhoto
	expiresAt time.Time
}

type photoCache struct {
	mutex   sync.Mutex
	entries map[string]photoCacheEntry
}

func (cache *photoCache) get(placeID string, now time.Time) (placePhoto, bool) {
	cache.mutex.Lock()
	defer cache.mutex.Unlock()
	entry, found := cache.entries[placeID]
	if !found || !entry.expiresAt.After(now) {
		return placePhoto{}, false
	}
	return entry.photo, true
}

func (cache *photoCache) set(placeID string, photo placePhoto, now time.Time) {
	cache.mutex.Lock()
	defer cache.mutex.Unlock()
	if cache.entries == nil {
		cache.entries = map[string]photoCacheEntry{}
	}
	for key, entry := range cache.entries {
		if !entry.expiresAt.After(now) {
			delete(cache.entries, key)
		}
	}
	if len(cache.entries) >= photoCacheMax {
		return
	}
	cache.entries[placeID] = photoCacheEntry{photo: photo, expiresAt: now.Add(photoCacheTTL)}
}

// placePhotoHandler answers with a short-lived photo URL and the attribution that
// must accompany it. The browser loads the bytes straight from Google, so no
// image data is ever stored or re-hosted here, and the API key never leaves the
// server.
func (s *Server) placePhotoHandler(writer http.ResponseWriter, request *http.Request) {
	writer.Header().Set("Cache-Control", "no-store, max-age=0")
	if s.googlePlaces == nil || !s.googlePlaces.Enabled() {
		writeError(writer, http.StatusServiceUnavailable, "Google Places is not configured")
		return
	}
	placeID := strings.TrimSpace(request.PathValue("googlePlaceID"))
	if placeID == "" || strings.ContainsAny(placeID, "/?#") {
		writeError(writer, http.StatusNotFound, "no photo for this place")
		return
	}
	now := time.Now()
	if photo, found := s.photos.get(placeID, now); found {
		writeJSON(writer, http.StatusOK, photo)
		return
	}
	photo, err := s.resolvePlacePhoto(request, placeID)
	if err != nil {
		// A place without photos — or an id Google itself rejects — is an ordinary
		// answer: the card falls back to its generated art either way.
		var apiError *googleplaces.APIError
		switch {
		case errors.Is(err, errNoPlacePhoto):
			writeError(writer, http.StatusNotFound, "no photo for this place")
		case errors.As(err, &apiError) && apiError.StatusCode == http.StatusBadRequest:
			writeError(writer, http.StatusNotFound, "no photo for this place")
		default:
			// Everything else (quota, 5xx, transport) is a real fault, and the
			// operator needs Google's own words in the log.
			log.Printf("place photo for %s failed: %v", placeID, err)
			writeError(writer, http.StatusBadGateway, "could not load a photo for this place")
		}
		return
	}
	s.photos.set(placeID, photo, now)
	writeJSON(writer, http.StatusOK, photo)
}

// errNoPlacePhoto marks "this place simply has no photo", which the card renders
// as its generated art rather than as an error.
var errNoPlacePhoto = errors.New("this place has no photo")

// resolvePlacePhoto prefers the reference captured at ingestion, which costs one
// media call, and only pays for a Place Details lookup when Google rejects a name
// it has since expired. That lookup is the expensive half ($20/1000 against $7/1000
// for the photo itself), so the stored reference is worth keeping.
func (s *Server) resolvePlacePhoto(request *http.Request, placeID string) (placePhoto, error) {
	ctx := request.Context()
	if stored, found, err := s.storedPhotoRef(ctx, placeID); err == nil && found {
		if photoURL, mediaErr := s.googlePlaces.PhotoMedia(ctx, stored.Name, photoDefaultWidthPx); mediaErr == nil {
			return placePhoto{URL: photoURL, Attribution: photoAttribution{Name: stored.AttributionName, URI: stored.AttributionURI}}, nil
		}
	}
	place, err := s.googlePlaces.GetPlace(ctx, placeID)
	if err != nil {
		if errors.Is(err, googleplaces.ErrPlaceNotFound) {
			return placePhoto{}, errNoPlacePhoto
		}
		return placePhoto{}, err
	}
	// A name that was just issued should resolve, so a failure here is a fault
	// rather than the expiry the stored-reference path above expects. Try each
	// photo before giving up, and report the fault if none of them works.
	var lastErr error
	for _, reference := range place.PhotoRefs() {
		photoURL, mediaErr := s.googlePlaces.PhotoMedia(ctx, reference.Name, photoDefaultWidthPx)
		if mediaErr != nil {
			lastErr = mediaErr
			continue
		}
		s.rememberPhotoRef(ctx, placeID, reference)
		return placePhoto{URL: photoURL, Attribution: photoAttribution{Name: reference.AttributionName, URI: reference.AttributionURI}}, nil
	}
	if lastErr != nil {
		return placePhoto{}, lastErr
	}
	return placePhoto{}, errNoPlacePhoto
}

func (s *Server) storedPhotoRef(ctx context.Context, placeID string) (googleplaces.PhotoRef, bool, error) {
	if s.photoStore == nil {
		return googleplaces.PhotoRef{}, false, nil
	}
	return s.photoStore.PhotoRef(ctx, placeID)
}

// rememberPhotoRef failing is not worth failing the request over: the visitor still
// gets their photo, the next one just pays for a lookup again.
func (s *Server) rememberPhotoRef(ctx context.Context, placeID string, reference googleplaces.PhotoRef) {
	if s.photoStore == nil {
		return
	}
	if err := s.photoStore.SavePhotoRef(ctx, placeID, reference); err != nil {
		log.Printf("store photo reference for %s: %v", placeID, err)
	}
}
