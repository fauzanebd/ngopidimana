package ingestion

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode"
)

const maxExaResponseBytes = 1 << 20

const (
	discoveryGoogleMapsReview = "google_maps_review"
	discoveryTikTokReview     = "tiktok_review"
	discoveryWebReview        = "web_review"
)

type discoveryLane struct {
	Kind           string
	Query          string
	HighlightQuery string
	IncludeDomains []string
	ExcludeDomains []string
	Limit          int
}

type exaSearchItem struct {
	Title         string   `json:"title"`
	URL           string   `json:"url"`
	PublishedDate string   `json:"publishedDate"`
	Highlights    []string `json:"highlights"`
}

type Discoverer interface {
	Discover(context.Context, DiscoverySeed) ([]DiscoveredSource, error)
}

type ExaDiscoverer struct {
	apiKey     string
	baseURL    string
	maxResults int
	client     *http.Client
}

func NewExaDiscoverer(apiKey, baseURL string, maxResults int) *ExaDiscoverer {
	if maxResults < 1 {
		maxResults = 5
	}
	return &ExaDiscoverer{
		apiKey:     strings.TrimSpace(apiKey),
		baseURL:    strings.TrimRight(baseURL, "/"),
		maxResults: maxResults,
		client:     &http.Client{Timeout: 15 * time.Second},
	}
}

func (discoverer *ExaDiscoverer) Enabled() bool { return discoverer.apiKey != "" }

func (discoverer *ExaDiscoverer) Discover(ctx context.Context, seed DiscoverySeed) ([]DiscoveredSource, error) {
	if !discoverer.Enabled() {
		return nil, nil
	}
	lanes := discoveryLanes(seed, discoverer.maxResults)
	sources := make([]DiscoveredSource, 0, discoverer.maxResults)
	seen := map[string]struct{}{}
	failures := make([]error, 0, len(lanes))
	for _, lane := range lanes {
		if len(sources) >= discoverer.maxResults {
			break
		}
		limit := lane.Limit
		if limit == 0 || limit > discoverer.maxResults-len(sources) {
			limit = discoverer.maxResults - len(sources)
		}
		items, err := discoverer.search(ctx, lane, limit)
		if err != nil {
			failures = append(failures, fmt.Errorf("%s: %w", lane.Kind, err))
			continue
		}
		for _, item := range items {
			source, ok := discoveredSourceFromItem(item, lane.Kind, seed)
			if !ok {
				continue
			}
			if _, exists := seen[source.URL]; exists {
				continue
			}
			seen[source.URL] = struct{}{}
			sources = append(sources, source)
			if len(sources) >= discoverer.maxResults || countSourcesOfKind(sources, lane.Kind) >= limit {
				break
			}
		}
	}
	if len(sources) == 0 && len(failures) > 0 {
		return nil, fmt.Errorf("Exa review discovery failed: %w", errors.Join(failures...))
	}
	return sources, nil
}

func (discoverer *ExaDiscoverer) search(ctx context.Context, lane discoveryLane, limit int) ([]exaSearchItem, error) {
	payload := struct {
		Query          string   `json:"query"`
		Type           string   `json:"type"`
		NumResults     int      `json:"numResults"`
		IncludeDomains []string `json:"includeDomains,omitempty"`
		ExcludeDomains []string `json:"excludeDomains,omitempty"`
		Contents       struct {
			Highlights struct {
				Query string `json:"query"`
			} `json:"highlights"`
		} `json:"contents"`
	}{
		Query: lane.Query, Type: "auto", NumResults: max(limit*2, limit),
		IncludeDomains: lane.IncludeDomains, ExcludeDomains: lane.ExcludeDomains,
	}
	payload.Contents.Highlights.Query = lane.HighlightQuery
	encoded, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("encode request: %w", err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, discoverer.baseURL+"/search", bytes.NewReader(encoded))
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("x-api-key", discoverer.apiKey)
	response, err := discoverer.client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("search: %w", err)
	}
	defer response.Body.Close()
	limited := io.LimitReader(response.Body, maxExaResponseBytes+1)
	body, err := io.ReadAll(limited)
	if err != nil {
		return nil, fmt.Errorf("read Exa response: %w", err)
	}
	if len(body) > maxExaResponseBytes {
		return nil, errors.New("Exa response exceeds the 1 MiB limit")
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("Exa returned HTTP %d: %s", response.StatusCode, truncateText(string(body), 240))
	}
	var result struct {
		Results []exaSearchItem `json:"results"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("decode Exa response: %w", err)
	}
	return result.Results, nil
}

func discoveryLanes(seed DiscoverySeed, maxResults int) []discoveryLane {
	name := safeQueryTerm(seed.Name, 160)
	area := safeQueryTerm(seed.Area, 120)
	location := ""
	if area != "" {
		location = " in " + area
	}
	googleLimit, tiktokLimit := 1, 0
	if maxResults >= 2 {
		googleLimit = maxResults / 2
		tiktokLimit = maxResults - googleLimit - 1
		if tiktokLimit < 1 {
			tiktokLimit = 1
		}
	}
	lanes := []discoveryLane{{
		Kind: discoveryGoogleMapsReview, Limit: googleLimit, IncludeDomains: []string{"google.com"},
		Query:          fmt.Sprintf("Google Maps customer reviews for the cafe named %q%s", name, location),
		HighlightQuery: "First-person customer review statements about Wi-Fi, power outlets, noise, laptop friendliness, seating, opening hours, prices, food, drinks, and ambience. Preserve the review wording.",
	}}
	if tiktokLimit > 0 {
		lanes = append(lanes, discoveryLane{
			Kind: discoveryTikTokReview, Limit: tiktokLimit, IncludeDomains: []string{"tiktok.com"},
			Query:          fmt.Sprintf("TikTok cafe review video for %q%s showing the venue, menu, food, drinks, ambience, and suitability for working", name, location),
			HighlightQuery: "Creator caption or transcript statements describing the cafe, menu, prices, food, drinks, interior, ambience, crowds, Wi-Fi, outlets, or working experience.",
		})
	}
	if maxResults-len(lanes) >= 0 {
		lanes = append(lanes, discoveryLane{
			Kind: discoveryWebReview, ExcludeDomains: []string{"google.com", "tiktok.com"},
			Query:          fmt.Sprintf("Recent first-person customer reviews and current menu information for the cafe named %q%s", name, location),
			HighlightQuery: "First-person visit evidence about the cafe's Wi-Fi, outlets, noise, work suitability, menu, prices, food, drinks, seating, and ambience.",
		})
	}
	return lanes
}

func discoveredSourceFromItem(item exaSearchItem, kind string, seed DiscoverySeed) (DiscoveredSource, bool) {
	cleanURL, ok := publicWebURL(item.URL)
	if !ok || !sourceAllowedForLane(cleanURL, kind) {
		return DiscoveredSource{}, false
	}
	cleanURL.Fragment = ""
	excerpt := truncateText(strings.Join(strings.Fields(strings.Join(item.Highlights, " ")), " "), 800)
	title := truncateText(strings.Join(strings.Fields(item.Title), " "), 180)
	if excerpt == "" || isDiscoveryBoilerplate(kind, excerpt) || !sourceMentionsVenue(seed.Name, title+" "+excerpt) {
		return DiscoveredSource{}, false
	}
	return DiscoveredSource{Title: title, URL: cleanURL.String(), Excerpt: excerpt, PublishedAt: item.PublishedDate, Kind: kind}, true
}

func sourceAllowedForLane(sourceURL *url.URL, kind string) bool {
	host := strings.ToLower(sourceURL.Hostname())
	switch kind {
	case discoveryGoogleMapsReview:
		return isGoogleMapsURL(sourceURL.String()) || host == "maps.app.goo.gl" || (host == "goo.gl" && strings.HasPrefix(sourceURL.Path, "/maps"))
	case discoveryTikTokReview:
		return (host == "tiktok.com" || strings.HasSuffix(host, ".tiktok.com")) && (strings.Contains(sourceURL.Path, "/video/") || strings.Contains(sourceURL.Path, "/photo/"))
	default:
		return true
	}
}

func sourceMentionsVenue(name, content string) bool {
	brand, branch := venueIdentityParts(name)
	content = normalizeSearchText(content)
	if brand == "" || content == "" {
		return false
	}
	if !containsVenueTerms(content, brand) {
		return false
	}
	return branch == "" || containsVenueTerms(content, branch)
}

func containsVenueTerms(content, identity string) bool {
	if strings.Contains(content, identity) {
		return true
	}
	required := make([]string, 0, len(strings.Fields(identity)))
	for _, token := range strings.Fields(identity) {
		if len([]rune(token)) >= 2 {
			required = append(required, token)
		}
	}
	if len(required) == 0 {
		return false
	}
	paddedContent := " " + content + " "
	for _, token := range required {
		if !strings.Contains(paddedContent, " "+token+" ") {
			return false
		}
	}
	return true
}

func venueIdentityParts(value string) (brand, branch string) {
	value = strings.Join(strings.Fields(value), " ")
	for _, separator := range []string{" - ", " – ", " — ", " | ", " · "} {
		if before, after, found := strings.Cut(value, separator); found && strings.TrimSpace(before) != "" {
			return normalizeSearchText(before), normalizeSearchText(after)
		}
	}
	return normalizeSearchText(value), ""
}

func isDiscoveryBoilerplate(kind, excerpt string) bool {
	if kind != discoveryGoogleMapsReview {
		return false
	}
	normalized := normalizeSearchText(excerpt)
	return strings.Contains(normalized, "when you have eliminated the javascript") ||
		strings.Contains(normalized, "enable javascript to see google maps") ||
		strings.Contains(normalized, "this page can t load google maps correctly")
}

func normalizeSearchText(value string) string {
	return strings.Join(strings.Fields(strings.Map(func(character rune) rune {
		if unicode.IsLetter(character) || unicode.IsDigit(character) {
			return unicode.ToLower(character)
		}
		return ' '
	}, value)), " ")
}

func countSourcesOfKind(sources []DiscoveredSource, kind string) int {
	count := 0
	for _, source := range sources {
		if source.Kind == kind {
			count++
		}
	}
	return count
}

func mergeDiscoveredSources(record ExtractedRecord, sources []DiscoveredSource, capturedAt time.Time) ExtractedRecord {
	for index, source := range sources {
		parsed, ok := publicWebURL(source.URL)
		if !ok {
			continue
		}
		title := source.Title
		if title == "" {
			title = parsed.Hostname()
		}
		key, sourceLabel, confidence := discoveredEvidencePresentation(source.Kind, parsed.Hostname())
		for _, field := range record.Fields {
			if sameHostname(field.SourceURL, source.URL) {
				confidence = min(confidence+.05, .78)
				break
			}
		}
		record.Fields = append(record.Fields, EvidenceField{
			Key:         key,
			Value:       title,
			Source:      sourceLabel,
			SourceID:    fmt.Sprintf("exa_%d", index+1),
			SourceURL:   source.URL,
			Excerpt:     source.Excerpt,
			CapturedAt:  capturedAt.Format(time.RFC3339),
			PublishedAt: source.PublishedAt,
			Method:      "search",
			Extractor:   "exa",
			Confidence:  confidence,
		})
	}
	return record
}

func discoveredSourceDocuments(sources []DiscoveredSource, capturedAt time.Time) []SourceDocument {
	documents := make([]SourceDocument, 0, len(sources))
	for index, source := range sources {
		if _, ok := publicWebURL(source.URL); !ok || strings.TrimSpace(source.Excerpt) == "" {
			continue
		}
		documents = append(documents, SourceDocument{
			ID:          fmt.Sprintf("exa_%d", index+1),
			URL:         source.URL,
			Kind:        "exa_" + firstNonEmpty(source.Kind, discoveryWebReview),
			Title:       source.Title,
			Content:     source.Excerpt,
			CapturedAt:  capturedAt.Format(time.RFC3339),
			PublishedAt: source.PublishedAt,
		})
	}
	return documents
}

func discoveredEvidencePresentation(kind, host string) (key, source string, confidence float64) {
	switch kind {
	case discoveryGoogleMapsReview:
		return "Google Maps review context", "Exa · Google Maps", .78
	case discoveryTikTokReview:
		return "TikTok review context", "Exa · TikTok", .7
	default:
		return "Web review context", "Exa · " + host, .65
	}
}

func publicWebURL(raw string) (*url.URL, bool) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Host == "" || parsed.User != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return nil, false
	}
	return parsed, true
}

func sameHostname(left, right string) bool {
	leftURL, leftOK := publicWebURL(left)
	rightURL, rightOK := publicWebURL(right)
	return leftOK && rightOK && strings.EqualFold(leftURL.Hostname(), rightURL.Hostname())
}

func safeQueryTerm(value string, limit int) string {
	value = strings.Map(func(character rune) rune {
		if unicode.IsControl(character) {
			return ' '
		}
		return character
	}, value)
	value = strings.ReplaceAll(value, `"`, "")
	return truncateText(strings.Join(strings.Fields(value), " "), limit)
}

func truncateText(value string, limit int) string {
	runes := []rune(strings.TrimSpace(value))
	if len(runes) <= limit {
		return string(runes)
	}
	return string(runes[:limit]) + "…"
}
