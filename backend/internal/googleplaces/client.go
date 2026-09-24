package googleplaces

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode"
)

const maxResponseBytes = 1 << 20

var ErrNotConfigured = errors.New("Google Places API is not configured")
var ErrPlaceNotFound = errors.New("Google Places could not resolve this venue")

// APIError is a non-2xx answer from Google, kept structured so callers can tell a
// rejected input (400: "the provided Place ID is not valid") from an outage or a
// quota refusal (5xx, 429) instead of treating both as "no such place".
type APIError struct {
	StatusCode int
	Status     string
	Message    string
}

func (apiError *APIError) Error() string {
	return fmt.Sprintf("HTTP %d: %s", apiError.StatusCode, apiError.Message)
}

type Client struct {
	apiKey       string
	baseURL      string
	languageCode string
	regionCode   string
	defaultArea  string
	httpClient   *http.Client
}

type LocalizedText struct {
	Text         string `json:"text"`
	LanguageCode string `json:"languageCode,omitempty"`
}

type AuthorAttribution struct {
	DisplayName string `json:"displayName"`
	URI         string `json:"uri,omitempty"`
	PhotoURI    string `json:"photoUri,omitempty"`
}

type Review struct {
	Name                           string            `json:"name"`
	RelativePublishTimeDescription string            `json:"relativePublishTimeDescription,omitempty"`
	Text                           LocalizedText     `json:"text"`
	OriginalText                   LocalizedText     `json:"originalText"`
	Rating                         float64           `json:"rating"`
	AuthorAttribution              AuthorAttribution `json:"authorAttribution"`
	PublishTime                    string            `json:"publishTime,omitempty"`
	FlagContentURI                 string            `json:"flagContentUri,omitempty"`
	GoogleMapsURI                  string            `json:"googleMapsUri"`
}

type Attribution struct {
	Provider    string `json:"provider"`
	ProviderURI string `json:"providerUri,omitempty"`
}

type Location struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
}

type OpeningHours struct {
	WeekdayDescriptions []string `json:"weekdayDescriptions,omitempty"`
	OpenNow             bool     `json:"openNow,omitempty"`
}

type Money struct {
	CurrencyCode string `json:"currencyCode"`
	Units        string `json:"units"`
	Nanos        int    `json:"nanos,omitempty"`
}

type PriceRange struct {
	StartPrice Money `json:"startPrice"`
	EndPrice   Money `json:"endPrice"`
}

type AddressComponent struct {
	LongText  string   `json:"longText"`
	ShortText string   `json:"shortText,omitempty"`
	Types     []string `json:"types"`
}

// PlacePhoto is one entry of a place's `photos` field. `Name` is a resource name
// such as `places/ChIJ…/photos/AeJ…`; it is short-lived and must not be stored as
// if it were permanent (Google's Places policies forbid caching place content).
type PlacePhoto struct {
	Name               string              `json:"name"`
	WidthPx            int                 `json:"widthPx,omitempty"`
	HeightPx           int                 `json:"heightPx,omitempty"`
	AuthorAttributions []AuthorAttribution `json:"authorAttributions,omitempty"`
}

// PhotoRef is a photo reference together with the attribution that must be shown
// wherever the photo is displayed. It is a reference, not content: it resolves to
// a short-lived URL on demand and may have expired by the time it is used.
type PhotoRef struct {
	Name            string
	AttributionName string
	AttributionURI  string
}

// PhotoRefs lists usable photo references in Google's order, each with the
// attribution that must be displayed beside it. Photos with no reference are skipped.
func (place Place) PhotoRefs() []PhotoRef {
	references := make([]PhotoRef, 0, len(place.Photos))
	for _, photo := range place.Photos {
		if reference, ok := photo.Ref(); ok {
			references = append(references, reference)
		}
	}
	return references
}

// Ref turns one photo into the reference the catalogue stores and serves.
func (photo PlacePhoto) Ref() (PhotoRef, bool) {
	name := strings.TrimSpace(photo.Name)
	if name == "" {
		return PhotoRef{}, false
	}
	reference := PhotoRef{Name: name}
	for _, author := range photo.AuthorAttributions {
		if displayName := strings.TrimSpace(author.DisplayName); displayName != "" {
			reference.AttributionName = displayName
			reference.AttributionURI = strings.TrimSpace(author.URI)
			break
		}
	}
	return reference, true
}

// PrimaryPhoto is the one ingestion captures: the first usable reference.
func (place Place) PrimaryPhoto() *PhotoRef {
	if references := place.PhotoRefs(); len(references) > 0 {
		return &references[0]
	}
	return nil
}

type Place struct {
	ID                  string             `json:"id"`
	DisplayName         LocalizedText      `json:"displayName"`
	FormattedAddress    string             `json:"formattedAddress,omitempty"`
	AddressComponents   []AddressComponent `json:"addressComponents,omitempty"`
	GoogleMapsURI       string             `json:"googleMapsUri"`
	WebsiteURI          string             `json:"websiteUri,omitempty"`
	Location            Location           `json:"location"`
	RegularOpeningHours OpeningHours       `json:"regularOpeningHours"`
	PriceRange          PriceRange         `json:"priceRange"`
	Rating              float64            `json:"rating,omitempty"`
	UserRatingCount     int                `json:"userRatingCount,omitempty"`
	Reviews             []Review           `json:"reviews"`
	Photos              []PlacePhoto       `json:"photos,omitempty"`
	Attributions        []Attribution      `json:"attributions,omitempty"`
}

func NewClient(apiKey, baseURL, languageCode, regionCode, defaultArea string, timeout time.Duration) *Client {
	if timeout <= 0 {
		timeout = 12 * time.Second
	}
	return &Client{
		apiKey:       strings.TrimSpace(apiKey),
		baseURL:      strings.TrimRight(firstNonEmpty(strings.TrimSpace(baseURL), "https://places.googleapis.com"), "/"),
		languageCode: firstNonEmpty(strings.TrimSpace(languageCode), "id"),
		regionCode:   firstNonEmpty(strings.TrimSpace(regionCode), "ID"),
		defaultArea:  firstNonEmpty(strings.TrimSpace(defaultArea), "Jakarta, Indonesia"),
		httpClient:   &http.Client{Timeout: timeout},
	}
}

func (client *Client) Enabled() bool { return client != nil && client.apiKey != "" }

// ResolvePlaceID performs a bounded Text Search and returns only the stable ID.
// The caller may persist the ID; the remaining Google response is intentionally discarded.
func (client *Client) ResolvePlaceID(ctx context.Context, name, area string) (string, error) {
	if !client.Enabled() {
		return "", ErrNotConfigured
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return "", ErrPlaceNotFound
	}
	area = firstNonEmpty(strings.TrimSpace(area), client.defaultArea)
	payload := struct {
		TextQuery    string `json:"textQuery"`
		LanguageCode string `json:"languageCode"`
		RegionCode   string `json:"regionCode"`
		PageSize     int    `json:"pageSize"`
	}{TextQuery: strings.TrimSpace(name + " " + area), LanguageCode: client.languageCode, RegionCode: client.regionCode, PageSize: 5}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("encode Google Places search: %w", err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, client.baseURL+"/v1/places:searchText", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("build Google Places search: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Goog-Api-Key", client.apiKey)
	request.Header.Set("X-Goog-FieldMask", "places.id,places.displayName")
	var response struct {
		Places []struct {
			ID               string        `json:"id"`
			DisplayName      LocalizedText `json:"displayName"`
			FormattedAddress string        `json:"formattedAddress"`
		} `json:"places"`
	}
	if err := client.doJSON(request, &response); err != nil {
		return "", fmt.Errorf("Google Places text search: %w", err)
	}
	for _, candidate := range response.Places {
		if candidate.ID != "" && venueNameMatches(name, candidate.DisplayName.Text) {
			return candidate.ID, nil
		}
	}
	return "", ErrPlaceNotFound
}

// GetPlace fetches the bounded Place Details fields used by ingestion and admin review.
func (client *Client) GetPlace(ctx context.Context, placeID string) (Place, error) {
	if !client.Enabled() {
		return Place{}, ErrNotConfigured
	}
	placeID = strings.TrimSpace(placeID)
	if placeID == "" || strings.Contains(placeID, "/") {
		return Place{}, ErrPlaceNotFound
	}
	query := url.Values{"languageCode": {client.languageCode}, "regionCode": {client.regionCode}}
	endpoint := client.baseURL + "/v1/places/" + url.PathEscape(placeID) + "?" + query.Encode()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return Place{}, fmt.Errorf("build Google Place Details request: %w", err)
	}
	request.Header.Set("X-Goog-Api-Key", client.apiKey)
	request.Header.Set("X-Goog-FieldMask", "id,displayName,formattedAddress,addressComponents,googleMapsUri,websiteUri,location,regularOpeningHours,priceRange,rating,userRatingCount,reviews,photos,attributions")
	var place Place
	if err := client.doJSON(request, &place); err != nil {
		return Place{}, fmt.Errorf("Google Place Details: %w", err)
	}
	if place.ID == "" {
		return Place{}, ErrPlaceNotFound
	}
	if place.Reviews == nil {
		place.Reviews = []Review{}
	}
	return place, nil
}

// PhotoMedia resolves a photo resource name into a short-lived URL the browser can
// load directly. The image bytes never pass through this service, and the URL is
// deliberately returned rather than the bytes so nothing is stored or re-hosted.
func (client *Client) PhotoMedia(ctx context.Context, photoName string, maxWidthPx int) (string, error) {
	if !client.Enabled() {
		return "", ErrNotConfigured
	}
	if !validPhotoName(photoName) {
		return "", ErrPlaceNotFound
	}
	if maxWidthPx < 1 || maxWidthPx > 4800 {
		maxWidthPx = 800
	}
	query := url.Values{"maxWidthPx": {strconv.Itoa(maxWidthPx)}, "skipHttpRedirect": {"true"}}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, client.baseURL+"/v1/"+photoName+"/media?"+query.Encode(), nil)
	if err != nil {
		return "", fmt.Errorf("build Google photo request: %w", err)
	}
	request.Header.Set("X-Goog-Api-Key", client.apiKey)
	var media struct {
		Name     string `json:"name"`
		PhotoURI string `json:"photoUri"`
	}
	if err := client.doJSON(request, &media); err != nil {
		return "", fmt.Errorf("Google Place Photo: %w", err)
	}
	if strings.TrimSpace(media.PhotoURI) == "" {
		return "", ErrPlaceNotFound
	}
	return media.PhotoURI, nil
}

// validPhotoName accepts only the resource shape the Places API issues, so a value
// from an upstream response can be placed in a request path without escaping
// concerns.
func validPhotoName(name string) bool {
	parts := strings.Split(name, "/")
	if len(parts) != 4 || parts[0] != "places" || parts[2] != "photos" {
		return false
	}
	if parts[1] == "" || parts[3] == "" {
		return false
	}
	return !strings.ContainsAny(name, "?#%")
}

func (client *Client) doJSON(request *http.Request, target any) error {
	response, err := client.httpClient.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	limited := io.LimitReader(response.Body, maxResponseBytes+1)
	body, err := io.ReadAll(limited)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}
	if len(body) > maxResponseBytes {
		return errors.New("response exceeds the 1 MiB limit")
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		// A 404 is the ordinary "this place id does not exist" answer, and callers
		// distinguish it from a transport or quota failure.
		if response.StatusCode == http.StatusNotFound {
			return ErrPlaceNotFound
		}
		var apiError struct {
			Error struct {
				Message string `json:"message"`
				Status  string `json:"status"`
			} `json:"error"`
		}
		_ = json.Unmarshal(body, &apiError)
		message := strings.TrimSpace(apiError.Error.Message)
		if message == "" {
			message = http.StatusText(response.StatusCode)
		}
		return &APIError{StatusCode: response.StatusCode, Status: apiError.Error.Status, Message: truncate(message, 240)}
	}
	if err := json.Unmarshal(body, target); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	return nil
}

// venueNameMatches reports whether a Places candidate is the venue that was searched for.
//
// The comparison has to survive localization. Google returns displayName in the requested
// language, and Indonesian listings are often shortened to the brand alone: searching
// "Kipakane Indonesian Cuisine Menteng" with languageCode=id returns a candidate named
// "Kipakane". So a candidate is accepted when either
//
//   - every token of the expected name appears in it — the listing is at least as specific
//     as the name we extracted, or
//   - every token of the candidate appears in the expected name *and* both start with the
//     same token — the listing is the same venue under a shorter local name.
//
// Anything else is refused: binding the wrong place is worse than binding none, because the
// ID is what every later Google fact is attributed to.
func venueNameMatches(expected, actual string) bool {
	expected, actual = normalize(expected), normalize(actual)
	if expected == "" || actual == "" {
		return false
	}
	if expected == actual {
		return true
	}
	if containsAllTokens(actual, expected) {
		return true
	}
	return containsAllTokens(expected, actual) && leadingToken(actual) == leadingToken(expected)
}

// containsAllTokens reports whether every whole word of needle appears in haystack, ignoring
// single-character tokens that carry no identity.
func containsAllTokens(haystack, needle string) bool {
	padded := " " + haystack + " "
	for _, token := range strings.Fields(needle) {
		if len([]rune(token)) >= 2 && !strings.Contains(padded, " "+token+" ") {
			return false
		}
	}
	return true
}

func leadingToken(value string) string {
	tokens := strings.Fields(value)
	if len(tokens) == 0 {
		return ""
	}
	return tokens[0]
}

func normalize(value string) string {
	return strings.Join(strings.Fields(strings.Map(func(character rune) rune {
		if unicode.IsLetter(character) || unicode.IsDigit(character) {
			return unicode.ToLower(character)
		}
		return ' '
	}, value)), " ")
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func truncate(value string, maximum int) string {
	runes := []rune(value)
	if len(runes) <= maximum {
		return value
	}
	return string(runes[:maximum]) + "…"
}
