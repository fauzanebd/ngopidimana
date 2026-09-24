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
	"github.com/fauzanebd/wheretowfc/backend/internal/ingestion"
)

// Caching a resolved photo URL is what keeps Google calls proportional to *places
// viewed per window* rather than to visitors, searches, or cards rendered. The TTL
// is a deliberate trade: longer means fewer billed media calls, shorter means a URL
// Google invalidated is served for less time. A long TTL is only safe because the
// browser reports a failed image (see refreshPlacePhoto) and the stored reference
// can be re-resolved on demand.
const (
	photoCacheTTL        = 6 * time.Hour
	photoCacheMax        = 512
	photoDefaultWidthPx  = 800
	photoRefreshCooldown = 5 * time.Minute
	photoRefreshesPerDay = 200
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
	photo      placePhoto
	expiresAt  time.Time
	resolvedAt time.Time
}

// photoCache holds resolved URLs in process. It also tracks when each was resolved
// so a client-reported failure can be told apart from a client that simply repeats
// itself, and counts refreshes per day so the public refresh endpoint cannot be
// turned into a way to spend the Google budget.
type photoCache struct {
	mutex        sync.Mutex
	entries      map[string]photoCacheEntry
	dayStart     time.Time
	refreshedDay int
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
	cache.entries[placeID] = photoCacheEntry{photo: photo, expiresAt: now.Add(photoCacheTTL), resolvedAt: now}
}

// beginRefresh decides whether a reported failure may cost a Google call. A place
// that was resolved moments ago is already fresh — the report is repeated or the
// client is looping — and a daily budget bounds an endpoint anyone can call.
func (cache *photoCache) beginRefresh(placeID string, now time.Time) bool {
	cache.mutex.Lock()
	defer cache.mutex.Unlock()
	if entry, found := cache.entries[placeID]; found && now.Sub(entry.resolvedAt) < photoRefreshCooldown {
		return false
	}
	if now.Sub(cache.dayStart) > 24*time.Hour {
		cache.dayStart, cache.refreshedDay = now, 0
	}
	if cache.refreshedDay >= photoRefreshesPerDay {
		return false
	}
	cache.refreshedDay++
	delete(cache.entries, placeID)
	return true
}

// placePhotoHandler answers with a short-lived photo URL and the attribution that
// must accompany it. The browser loads the bytes straight from Google, so no image
// data is ever stored or re-hosted here, and the API key never leaves the server.
func (s *Server) placePhotoHandler(writer http.ResponseWriter, request *http.Request) {
	writer.Header().Set("Cache-Control", "no-store, max-age=0")
	placeID, ok := photoPlaceID(request)
	if !ok {
		writeError(writer, http.StatusNotFound, "no photo for this place")
		return
	}
	// An admin-attached photo needs no API key and no lookup, so it is answered
	// before anything Google-related is considered.
	if photo, found := s.overridePhoto(request.Context(), placeID); found {
		writeJSON(writer, http.StatusOK, photo)
		return
	}
	if !s.googlePlacesConfigured() {
		writeError(writer, http.StatusServiceUnavailable, "Google Places is not configured")
		return
	}
	now := time.Now()
	if photo, found := s.photos.get(placeID, now); found {
		writeJSON(writer, http.StatusOK, photo)
		return
	}
	photo, err := s.resolvePlacePhoto(request, placeID)
	if err != nil {
		s.writePhotoError(writer, placeID, err)
		return
	}
	s.photos.set(placeID, photo, now)
	writeJSON(writer, http.StatusOK, photo)
}

// refreshPlacePhoto is the other half of a long cache: the card reports an image
// it could not load, and this re-resolves that place's URL. It is public because
// visitors are the ones who see the failure.
func (s *Server) refreshPlacePhoto(writer http.ResponseWriter, request *http.Request) {
	writer.Header().Set("Cache-Control", "no-store, max-age=0")
	placeID, ok := photoPlaceID(request)
	if !ok {
		writeError(writer, http.StatusNotFound, "no photo for this place")
		return
	}
	if photo, found := s.overridePhoto(request.Context(), placeID); found {
		writeJSON(writer, http.StatusOK, photo)
		return
	}
	if !s.googlePlacesConfigured() {
		writeError(writer, http.StatusServiceUnavailable, "Google Places is not configured")
		return
	}
	now := time.Now()
	if !s.photos.beginRefresh(placeID, now) {
		writeError(writer, http.StatusTooManyRequests, "too many refreshes")
		return
	}
	photo, err := s.resolvePlacePhoto(request, placeID)
	if err != nil {
		s.writePhotoError(writer, placeID, err)
		return
	}
	s.photos.set(placeID, photo, now)
	writeJSON(writer, http.StatusOK, photo)
}

// writePhotoError separates "this place has no photo" from "we could not get one",
// because only the second is worth an operator's attention.
func (s *Server) writePhotoError(writer http.ResponseWriter, placeID string, err error) {
	var apiError *googleplaces.APIError
	switch {
	case errors.Is(err, errNoPlacePhoto):
		writeError(writer, http.StatusNotFound, "no photo for this place")
	case errors.As(err, &apiError) && apiError.StatusCode == http.StatusBadRequest:
		writeError(writer, http.StatusNotFound, "no photo for this place")
	default:
		log.Printf("place photo for %s failed: %v", placeID, err)
		writeError(writer, http.StatusBadGateway, "could not load a photo for this place")
	}
}

func photoPlaceID(request *http.Request) (string, bool) {
	placeID := strings.TrimSpace(request.PathValue("googlePlaceID"))
	if placeID == "" || strings.ContainsAny(placeID, "/?#") {
		return "", false
	}
	return placeID, true
}

func (s *Server) googlePlacesConfigured() bool {
	return s.googlePlaces != nil && s.googlePlaces.Enabled()
}

func (s *Server) overridePhoto(ctx context.Context, placeID string) (placePhoto, bool) {
	if s.photoStore == nil {
		return placePhoto{}, false
	}
	override, found, err := s.photoStore.PhotoOverride(ctx, placeID)
	if err != nil {
		log.Printf("read photo override for %s: %v", placeID, err)
		return placePhoto{}, false
	}
	if !found || strings.TrimSpace(override.URL) == "" {
		return placePhoto{}, false
	}
	return placePhoto{URL: override.URL, Attribution: photoAttribution{Name: override.Attribution}}, true
}

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

// errNoPlacePhoto marks "this place simply has no photo", which the card renders
// as its generated art rather than as an error.
var errNoPlacePhoto = errors.New("this place has no photo")

// PlacePhotoStore reads and writes the photo metadata for a place: the reference
// captured at ingestion, and any admin-attached cover photo.
type PlacePhotoStore interface {
	PhotoRef(ctx context.Context, googlePlaceID string) (googleplaces.PhotoRef, bool, error)
	SavePhotoRef(ctx context.Context, googlePlaceID string, reference googleplaces.PhotoRef) error
	PhotoOverride(ctx context.Context, googlePlaceID string) (ingestion.PhotoOverride, bool, error)
}

// setPhotoOverride attaches an image the project has rights to, which the public
// API then serves instead of Google's photo.
func (s *Server) setPhotoOverride(writer http.ResponseWriter, request *http.Request) {
	var input ingestion.PhotoOverrideInput
	if err := decodeJSON(request, &input); err != nil {
		writeError(writer, http.StatusBadRequest, err.Error())
		return
	}
	s.writePhotoOverride(writer, request, input)
}

// clearPhotoOverride records an explicit "no override", which publishing applies
// (as opposed to deleting the record, which would leave the place as it was).
func (s *Server) clearPhotoOverride(writer http.ResponseWriter, request *http.Request) {
	s.writePhotoOverride(writer, request, ingestion.PhotoOverrideInput{})
}

func (s *Server) writePhotoOverride(writer http.ResponseWriter, request *http.Request, input ingestion.PhotoOverrideInput) {
	if s.ingestion == nil {
		writeError(writer, http.StatusServiceUnavailable, "the ingestion service is unavailable")
		return
	}
	run, err := s.ingestion.SetPhotoOverride(request.Context(), request.PathValue("id"), input)
	if err != nil {
		switch {
		case errors.Is(err, ingestion.ErrNotFound):
			writeError(writer, http.StatusNotFound, err.Error())
		case errors.Is(err, ingestion.ErrInvalidPhotoOverride), strings.Contains(err.Error(), "cannot be changed"):
			writeError(writer, http.StatusBadRequest, err.Error())
		default:
			writeError(writer, http.StatusServiceUnavailable, "could not save the cover photo")
		}
		return
	}
	writeJSON(writer, http.StatusOK, run)
}
