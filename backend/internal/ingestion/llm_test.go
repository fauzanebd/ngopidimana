package ingestion

import (
	"strings"
	"testing"
	"time"
)

func TestMergeModelExtractionValidatesCitationsNormalizesAndDetectsConflicts(t *testing.T) {
	record := ExtractedRecord{
		Name: "Kopi Test", Confidence: .8, Issues: []string{},
		Fields: []EvidenceField{{Key: "Opening hours", Value: "08:00-23:00", SourceURL: "https://kopi.example", Confidence: .9}},
		Documents: []SourceDocument{
			{ID: "source_1", URL: "https://kopi.example", Kind: "submitted_html", Content: "Wi-Fi tersedia. Drinks start from Rp 50k."},
			{ID: "exa_1", URL: "https://listing.example/kopi", Kind: "exa_highlight", Content: "Open every day from 08:00 until 22:00. Matcha Latte Rp45k."},
		},
	}
	price := 45000
	extraction := ModelExtraction{Model: "test/model", Claims: []ModelClaim{
		{Field: "facilities.wifi", Value: "yes", SourceID: "source_1", Excerpt: "Wi-Fi tersedia", Confidence: .96},
		{Field: "price.minimum", Value: "Rp 50k", SourceID: "source_1", Excerpt: "Drinks start from Rp 50k", Confidence: .9},
		{Field: "hours.schedule", Value: "08:00-22:00", SourceID: "exa_1", Excerpt: "Open every day from 08:00 until 22:00", Confidence: .9},
		{Field: "facilities.musholla", Value: "yes", SourceID: "source_1", Excerpt: "musholla available", Confidence: .9},
	}, MenuItems: []ModelMenuItem{{
		SourceLabel: "Matcha Latte", NormalizedName: "matcha latte", Category: "tea", Attributes: []string{"matcha"},
		Price: &price, Currency: "IDR", SourceID: "exa_1", Excerpt: "Matcha Latte Rp45k", Confidence: .9,
	}}}

	merged, accepted := mergeModelExtraction(record, extraction, time.Date(2026, 9, 23, 0, 0, 0, 0, time.UTC))
	if accepted != 4 {
		t.Fatalf("expected four locally supported items, got %d: %#v", accepted, merged.Fields)
	}
	joined := ""
	for _, field := range merged.Fields {
		joined += field.Key + ":" + field.Value + "\n"
		if field.Key == "Wi-Fi" && (field.Value != "Yes" || field.Confidence != .9) {
			t.Fatalf("Wi-Fi evidence was not normalized and capped: %#v", field)
		}
	}
	if !strings.Contains(joined, "Minimum price:Rp50000") || !strings.Contains(joined, "Menu item:matcha latte") || strings.Contains(joined, "Musholla") {
		t.Fatalf("unexpected normalized evidence:\n%s", joined)
	}
	if !containsString(merged.Issues, "Conflicting evidence for Opening hours") {
		t.Fatalf("opening-hours conflict was not detected: %#v", merged.Issues)
	}
}

func TestBuildExtractionPromptBoundsDocuments(t *testing.T) {
	documents := make([]SourceDocument, 0, 10)
	for index := 0; index < 10; index++ {
		documents = append(documents, SourceDocument{ID: "source", URL: "https://example.com", Content: strings.Repeat("x", 2000)})
	}
	prompt, err := buildExtractionPrompt(ExtractionInput{SourceURL: "https://example.com", Documents: documents}, 4000)
	if err != nil {
		t.Fatal(err)
	}
	if len([]rune(prompt)) > 5000 || strings.Count(prompt, `"content"`) > 2 {
		t.Fatalf("prompt input was not bounded: length=%d", len([]rune(prompt)))
	}
}

func TestCrowdClaimRequiresActualCrowdLanguage(t *testing.T) {
	documents := map[string]SourceDocument{"review": {ID: "review", Kind: "google_places_review", URL: "https://example.com/review", Content: "makanan ringan dan makanan beratnya banyak pilihan"}}
	claim := ModelClaim{Field: "work.crowd", Value: "many choices", SourceID: "review", Excerpt: "makanan ringan dan makanan beratnya banyak pilihan", Confidence: .9}
	if _, ok := modelClaimToEvidence(claim, documents, "test", time.Now()); ok {
		t.Fatal("menu variety must not be accepted as crowd-level evidence")
	}
	documents["review"] = SourceDocument{ID: "review", Kind: "google_places_review", URL: "https://example.com/review", Content: "ramai setelah jam makan siang"}
	claim.Value, claim.Excerpt = "crowded after lunch", "ramai setelah jam makan siang"
	if _, ok := modelClaimToEvidence(claim, documents, "test", time.Now()); !ok {
		t.Fatal("explicit crowd evidence should be accepted")
	}
}

func TestMenuUncertaintyCannotLeakFromAnotherItem(t *testing.T) {
	documents := map[string]SourceDocument{"review": {ID: "review", Kind: "google_places_review", URL: "https://example.com/review", Content: "tapi adikku pesan smorse brownie dan dia suka banget!"}}
	field, ok := modelMenuItemToEvidence(ModelMenuItem{
		SourceLabel: "Smorse brownie", NormalizedName: "Smorse brownie", Category: "dessert",
		Attributes: []string{"reviewer was unsure of exact name"}, SourceID: "review", Excerpt: "tapi adikku pesan smorse brownie dan dia suka banget!", Confidence: .8,
	}, documents, "test", time.Now())
	if !ok || strings.Contains(strings.ToLower(field.Value), "unsure") {
		t.Fatalf("unsupported uncertainty leaked into menu item: %#v", field)
	}
}

func TestAreaAdministrativePrefixDoesNotCreateConflict(t *testing.T) {
	record := detectEvidenceConflicts(ExtractedRecord{Fields: []EvidenceField{
		{Key: "Area", Value: "Kecamatan Tebet", SourceURL: "https://maps.google.com/place"},
		{Key: "Area", Value: "Tebet", SourceURL: "https://maps.google.com/review"},
	}})
	if len(record.Issues) != 0 || record.Fields[0].Conflict || record.Fields[1].Conflict {
		t.Fatalf("equivalent area names were treated as conflicting: %#v", record)
	}
}

func TestEquivalentAddressFormattingDoesNotCreateConflict(t *testing.T) {
	record := detectEvidenceConflicts(ExtractedRecord{Fields: []EvidenceField{
		{Key: "Address", Value: "Jl. Tebet Timur Dalam II No.42, RT.1/RW.4, Tebet Tim., Kec. Tebet, Kota Jakarta Selatan, Daerah Khusus Ibukota Jakarta 12820", SourceURL: "https://maps.google.com/place"},
		{Key: "Address", Value: "Jalan Tebet Timur Dalam II No.42 Tebet Jakarta 12820", SourceURL: "https://listing.example/place"},
	}})
	if len(record.Issues) != 0 || record.Fields[0].Conflict || record.Fields[1].Conflict {
		t.Fatalf("equivalent address formats were treated as conflicting: %#v", record)
	}
}

func TestCurrentGoogleOperationalFactBeatsLowerConfidenceListing(t *testing.T) {
	record := detectEvidenceConflicts(ExtractedRecord{Fields: []EvidenceField{
		{Key: "Opening hours", Value: "Senin–Minggu: 09.00–22.00", SourceURL: "https://maps.google.com/place", Method: "api", Extractor: "google_places", Confidence: .95},
		{Key: "Opening hours", Value: "Monday–Sunday: 10:00–20:00", SourceURL: "https://listing.example/place", Method: "llm_structured", Extractor: "openrouter:test", Confidence: .72},
	}})
	if len(record.Issues) != 0 || record.Fields[0].Conflict || record.Fields[1].Conflict {
		t.Fatalf("lower-confidence listing should remain secondary, not block publication: %#v", record)
	}
}

func TestStrongContradictionStillCreatesConflict(t *testing.T) {
	record := detectEvidenceConflicts(ExtractedRecord{Fields: []EvidenceField{
		{Key: "Opening hours", Value: "09:00–22:00", SourceURL: "https://maps.google.com/place", Method: "api", Extractor: "google_places", Confidence: .95},
		{Key: "Opening hours", Value: "10:00–20:00", SourceURL: "https://official.example", Method: "deterministic", Extractor: "json_ld", Confidence: .9},
	}})
	if !containsString(record.Issues, "Conflicting evidence for Opening hours") || !record.Fields[0].Conflict || !record.Fields[1].Conflict {
		t.Fatalf("similarly strong sources must still require review: %#v", record)
	}
}

func TestGenericPerPersonBandIsNotMenuPrice(t *testing.T) {
	documents := map[string]SourceDocument{"listing": {
		ID: "listing", Kind: "exa_web_review", URL: "https://listing.example/place",
		Content: "Price Range $$ (Rp 100.000 - 250.000 per person)",
	}}
	field, ok := modelClaimToEvidence(ModelClaim{
		Field: "price.minimum", Value: "Rp100000", SourceID: "listing",
		Excerpt: "Price Range $$ (Rp 100.000 - 250.000 per person)", Confidence: .9,
	}, documents, "test", time.Now())
	if !ok || field.Key != "Typical spend minimum" || field.Value != "Rp100000" {
		t.Fatalf("generic spend band was classified as a menu-price extreme: %#v ok=%v", field, ok)
	}
}
