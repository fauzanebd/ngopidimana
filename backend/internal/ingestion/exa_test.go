package ingestion

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestExaDiscovererSearchesAndSanitizesResults(t *testing.T) {
	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		requestCount++
		if request.Method != http.MethodPost || request.URL.Path != "/search" {
			t.Fatalf("unexpected request: %s %s", request.Method, request.URL.Path)
		}
		if request.Header.Get("x-api-key") != "exa-test" {
			t.Fatal("missing Exa API key")
		}
		var body struct {
			Query          string   `json:"query"`
			NumResults     int      `json:"numResults"`
			IncludeDomains []string `json:"includeDomains"`
			ExcludeDomains []string `json:"excludeDomains"`
			Contents       struct {
				Highlights struct {
					Query string `json:"query"`
				} `json:"highlights"`
			} `json:"contents"`
		}
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body.NumResults < 1 || body.Contents.Highlights.Query == "" || !strings.Contains(body.Query, "Kopi Test") {
			t.Fatalf("unexpected Exa request: %#v", body)
		}
		response.Header().Set("Content-Type", "application/json")
		switch {
		case len(body.IncludeDomains) == 1 && body.IncludeDomains[0] == "google.com":
			_, _ = response.Write([]byte(`{"results":[{"title":"Kopi Test - Google Maps","url":"https://www.google.com/maps/place/Kopi+Test/#reviews","publishedDate":"2026-09-20T00:00:00.000Z","highlights":["Quiet upstairs and fast Wi-Fi."]},{"title":"Google Maps help","url":"https://support.google.com/maps","highlights":["Manage reviews."]}]}`))
		case len(body.IncludeDomains) == 1 && body.IncludeDomains[0] == "tiktok.com":
			_, _ = response.Write([]byte(`{"results":[{"title":"Kopi Test cafe review","url":"https://www.tiktok.com/@reviewer/video/123","publishedDate":"2026-09-21T00:00:00.000Z","highlights":["Plenty of tables and drinks from Rp30k."]},{"title":"Unrelated cafe","url":"https://www.tiktok.com/@reviewer/video/999","highlights":["Another place."]}]}`))
		default:
			_, _ = response.Write([]byte(`{"results":[{"title":"Kopi Test official menu","url":"https://kopi.example/menu#drinks","publishedDate":"2026-09-22T00:00:00.000Z","highlights":["Open daily.","Fast Wi-Fi."]},{"title":"Unsafe","url":"javascript:alert(1)","highlights":["ignore"]}]}`))
		}
	}))
	defer server.Close()

	discoverer := NewExaDiscoverer("exa-test", server.URL, 5)
	sources, err := discoverer.Discover(t.Context(), DiscoverySeed{Name: "Kopi Test", URL: "https://kopi.example"})
	if err != nil {
		t.Fatal(err)
	}
	if requestCount != 3 || len(sources) != 3 {
		t.Fatalf("expected three prioritized discovery lanes, requests=%d sources=%#v", requestCount, sources)
	}
	if sources[0].Kind != discoveryGoogleMapsReview || sources[1].Kind != discoveryTikTokReview || sources[2].Kind != discoveryWebReview {
		t.Fatalf("unexpected source priority: %#v", sources)
	}
	if sources[2].URL != "https://kopi.example/menu" || sources[2].Excerpt != "Open daily. Fast Wi-Fi." {
		t.Fatalf("unexpected sources: %#v", sources)
	}
}

func TestExaDiscovererIsNoopWithoutKey(t *testing.T) {
	discoverer := NewExaDiscoverer("", "https://api.exa.ai", 5)
	sources, err := discoverer.Discover(t.Context(), DiscoverySeed{Name: "Kopi Test"})
	if err != nil || len(sources) != 0 || discoverer.Enabled() {
		t.Fatalf("disabled Exa discoverer should be a no-op: sources=%#v err=%v", sources, err)
	}
}

func TestVenueMatchingRejectsAnotherBusinessInTheSameNeighborhood(t *testing.T) {
	content := "SOGOGI SHABU & GRILL CIPETE - Jl. Cipete Raya reviews and opening hours"
	if sourceMentionsVenue("TAAM House - Cipete", content) {
		t.Fatal("a shared neighborhood must not count as a venue identity match")
	}
	if !sourceMentionsVenue("TAAM House - Cipete", "TAAM House Cipete has quiet upstairs seating") {
		t.Fatal("the actual venue brand should match")
	}
}

func TestVenueMatchingRejectsAnotherBranchOfTheSameBrand(t *testing.T) {
	if sourceMentionsVenue("TAAM House - Cipete", "TAAM House Rawamangun has quiet upstairs seating") {
		t.Fatal("another branch of the same brand must not count as a venue match")
	}
	if !sourceMentionsVenue("TAAM House - Cipete", "Review of TAAM House branch in Cipete") {
		t.Fatal("brand and requested branch should match even with different punctuation")
	}
}

func TestVenueMatchingPreservesBusinessWordsInBrand(t *testing.T) {
	if sourceMentionsVenue("Agreya Coffee Menteng", "Agreya Menteng has comfortable seats") {
		t.Fatal("dropping Coffee from the venue identity could match a different business")
	}
	if !sourceMentionsVenue("Agreya Coffee Menteng", "Agreya Coffee in Menteng has comfortable seats") {
		t.Fatal("the complete normalized venue identity should match")
	}
}

func TestGoogleMapsShellIsNotAcceptedAsReviewEvidence(t *testing.T) {
	item := exaSearchItem{
		Title: "Google Maps", URL: "https://www.google.com/maps/place/TAAM+House+-+Cipete/",
		Highlights: []string{"Google Maps. When you have eliminated the JavaScript, whatever remains must be an empty page."},
	}
	if source, ok := discoveredSourceFromItem(item, discoveryGoogleMapsReview, DiscoverySeed{Name: "TAAM House - Cipete"}); ok {
		t.Fatalf("generic Google Maps shell was accepted: %#v", source)
	}
}
