package ingestion

import (
	"strings"
	"testing"

	"github.com/fauzanebd/wheretowfc/backend/internal/googleplaces"
)

func TestGooglePlaceAreaPrefersNamedBranch(t *testing.T) {
	components := []googleplaces.AddressComponent{{LongText: "Kecamatan Cilandak", Types: []string{"administrative_area_level_3"}}}
	if area := googlePlaceArea("TAAM House - Cipete", components); area != "Cipete" {
		t.Fatalf("expected branch area, got %q", area)
	}
	if area := googlePlaceArea("Kopi Domu Berkisah", components); area != "Cilandak" {
		t.Fatalf("expected cleaned address area, got %q", area)
	}
}

func TestDeterministicReviewSignalsKeepUsefulCaveats(t *testing.T) {
	document := SourceDocument{ID: "review", URL: "https://maps.google.com/review", Kind: "google_places_review", Content: "Tempatnya luas dan banyak tempat duduk. Suasana nyaman dan tenang. Colokan banyak, cuma WiFinya agak susah Connect. Parkir motor banyak, kalau mobil hanya cukup untuk 4-5 mobil."}
	record := mergeDeterministicReviewSignals(ExtractedRecord{}, document, "2026-09-23T00:00:00Z")
	joined := ""
	for _, field := range record.Fields {
		joined += field.Key + ":" + field.Value + "\n"
		if field.Excerpt != "" && !strings.Contains(field.Excerpt, "Colokan banyak") {
			// Every deterministic excerpt is allowed to use the complete bounded review;
			// it must not truncate away the supporting middle/end of this short review.
			if len([]rune(field.Excerpt)) < len([]rune(document.Content)) {
				t.Fatalf("supporting review was unexpectedly truncated: %q", field.Excerpt)
			}
		}
	}
	for _, expected := range []string{"Power outlets:Yes", "Wi-Fi quality:Reported difficult to connect", "Quietness:Reported quiet", "Seating comfort:Reported spacious with many seats", "Parking notes:Car parking reported limited"} {
		if !strings.Contains(joined, expected) {
			t.Fatalf("missing %q from signals:\n%s", expected, joined)
		}
	}
}
