package httpapi

import (
	"errors"
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
		// A place without photos is an ordinary answer, not a failure: the card
		// falls back to its generated art either way.
		if err == errNoPlacePhoto {
			writeError(writer, http.StatusNotFound, "no photo for this place")
			return
		}
		writeError(writer, http.StatusBadGateway, "could not load a photo for this place")
		return
	}
	s.photos.set(placeID, photo, now)
	writeJSON(writer, http.StatusOK, photo)
}

// errNoPlacePhoto marks "this place simply has no photo", which the card renders
// as its generated art rather than as an error.
var errNoPlacePhoto = errors.New("this place has no photo")

func (s *Server) resolvePlacePhoto(request *http.Request, placeID string) (placePhoto, error) {
	place, err := s.googlePlaces.GetPlace(request.Context(), placeID)
	if err != nil {
		if errors.Is(err, googleplaces.ErrPlaceNotFound) {
			return placePhoto{}, errNoPlacePhoto
		}
		return placePhoto{}, err
	}
	for _, candidate := range place.Photos {
		photoURL, err := s.googlePlaces.PhotoMedia(request.Context(), candidate.Name, photoDefaultWidthPx)
		if err != nil {
			continue
		}
		attribution := photoAttribution{}
		for _, author := range candidate.AuthorAttributions {
			if name := strings.TrimSpace(author.DisplayName); name != "" {
				attribution = photoAttribution{Name: name, URI: strings.TrimSpace(author.URI)}
				break
			}
		}
		return placePhoto{URL: photoURL, Attribution: attribution}, nil
	}
	return placePhoto{}, errNoPlacePhoto
}
