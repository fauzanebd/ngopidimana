package ingestion

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
)

type EvidenceExtractor interface {
	ExtractEvidence(context.Context, ExtractionInput) (ModelExtraction, error)
}

type ExtractionInput struct {
	RunID     string
	Name      string
	Area      string
	SourceURL string
	Documents []SourceDocument
}

type ModelExtraction struct {
	Claims    []ModelClaim    `json:"claims"`
	MenuItems []ModelMenuItem `json:"menu_items"`
	Summary   string          `json:"summary"`
	Model     string          `json:"-"`
}

type ModelClaim struct {
	Field      string  `json:"field"`
	Value      string  `json:"value"`
	SourceID   string  `json:"source_id"`
	Excerpt    string  `json:"excerpt"`
	Confidence float64 `json:"confidence"`
}

type ModelMenuItem struct {
	SourceLabel    string   `json:"source_label"`
	NormalizedName string   `json:"normalized_name"`
	Category       string   `json:"category"`
	Attributes     []string `json:"attributes"`
	Price          *int     `json:"price"`
	Currency       string   `json:"currency"`
	SourceID       string   `json:"source_id"`
	Excerpt        string   `json:"excerpt"`
	Confidence     float64  `json:"confidence"`
}

var extractionFieldLabels = map[string]string{
	"identity.name":                 "Identity",
	"identity.alias":                "Identity alias",
	"location.address":              "Address",
	"location.area":                 "Area",
	"location.latitude":             "Latitude",
	"location.longitude":            "Longitude",
	"hours.schedule":                "Opening hours",
	"hours.open_24h":                "Open 24 hours",
	"price.minimum":                 "Minimum price",
	"price.maximum":                 "Maximum price",
	"price.typical_minimum":         "Typical spend minimum",
	"price.typical_maximum":         "Typical spend maximum",
	"facilities.musholla":           "Musholla",
	"facilities.toilet":             "Toilet",
	"facilities.wifi":               "Wi-Fi",
	"facilities.outlets":            "Power outlets",
	"facilities.parking_car":        "Car parking",
	"facilities.parking_motorcycle": "Motorcycle parking",
	"facilities.accessible":         "Accessibility",
	"facilities.ac":                 "Air conditioning",
	"facilities.indoor":             "Indoor seating",
	"facilities.outdoor":            "Outdoor seating",
	"facilities.smoking_zone":       "Smoking zone",
	"work.quiet":                    "Quietness",
	"work.crowd":                    "Crowd level",
	"work.table_size":               "Table size",
	"work.seating_comfort":          "Seating comfort",
	"work.lighting":                 "Lighting",
	"work.laptop_policy":            "Laptop policy",
	"work.long_stay":                "Long-stay friendliness",
	"work.calls":                    "Calls suitability",
	"work.wifi_quality":             "Wi-Fi quality",
	"work.meetings":                 "Meetings suitability",
	"ambience.styles":               "Ambience",
	"policies.reservation":          "Reservation policy",
	"policies.time_limit":           "Time limit",
	"policies.minimum_order":        "Minimum order",
	"policies.pets":                 "Pet policy",
	"food.halal":                    "Halal evidence",
	"external.google_maps":          "Google Maps",
	"external.instagram":            "Instagram",
	"external.tiktok":               "TikTok",
	"external.website":              "Website",
}

var booleanExtractionFields = map[string]bool{
	"hours.open_24h": true, "facilities.musholla": true, "facilities.toilet": true,
	"facilities.wifi": true, "facilities.outlets": true, "facilities.parking_car": true,
	"facilities.parking_motorcycle": true, "facilities.accessible": true, "facilities.ac": true,
	"facilities.indoor": true, "facilities.outdoor": true, "facilities.smoking_zone": true,
	"policies.reservation": true, "policies.pets": true, "food.halal": true,
}

var conflictSensitiveFields = map[string]bool{
	"Identity": true, "Address": true, "Area": true, "Latitude": true, "Longitude": true,
	"Opening hours": true, "Open 24 hours": true, "Minimum price": true, "Maximum price": true,
	"Musholla": true, "Toilet": true, "Wi-Fi": true, "Power outlets": true,
	"Car parking": true, "Motorcycle parking": true, "Accessibility": true,
	"Air conditioning": true, "Indoor seating": true, "Outdoor seating": true,
	"Smoking zone": true, "Pet policy": true, "Halal evidence": true,
}

var priceNumberPattern = regexp.MustCompile(`(?i)(?:rp\s*)?([0-9][0-9.,]*)(?:\s*(k|rb|ribu))?`)
var addressNumberPattern = regexp.MustCompile(`\bno\s*([0-9]+[a-z]?)\b`)
var postalCodePattern = regexp.MustCompile(`\b[0-9]{5}\b`)

func mergeModelExtraction(record ExtractedRecord, extraction ModelExtraction, capturedAt time.Time) (ExtractedRecord, int) {
	documents := make(map[string]SourceDocument, len(record.Documents))
	for _, document := range record.Documents {
		documents[document.ID] = document
	}
	accepted := 0
	for _, claim := range extraction.Claims {
		field, ok := modelClaimToEvidence(claim, documents, extraction.Model, capturedAt)
		if !ok || evidenceExists(record.Fields, field) {
			continue
		}
		record.Fields = append(record.Fields, field)
		accepted++
	}
	for _, item := range extraction.MenuItems {
		field, ok := modelMenuItemToEvidence(item, documents, extraction.Model, capturedAt)
		if !ok || evidenceExists(record.Fields, field) {
			continue
		}
		record.Fields = append(record.Fields, field)
		accepted++
	}
	record = detectEvidenceConflicts(record)
	record = chooseRecordIdentity(record)
	return record, accepted
}

func modelClaimToEvidence(claim ModelClaim, documents map[string]SourceDocument, model string, capturedAt time.Time) (EvidenceField, bool) {
	label, supported := extractionFieldLabels[claim.Field]
	document, sourceExists := documents[claim.SourceID]
	if !supported || !sourceExists || !excerptSupported(document.Content, claim.Excerpt) {
		return EvidenceField{}, false
	}
	if (claim.Field == "identity.name" || claim.Field == "identity.alias") && strings.Contains(document.Kind, "review") {
		return EvidenceField{}, false
	}
	if !claimSemanticallySupported(claim.Field, claim.Excerpt) {
		return EvidenceField{}, false
	}
	value, ok := normalizeClaimValue(claim.Field, claim.Value)
	if !ok {
		return EvidenceField{}, false
	}
	// Reservation/listing sites often expose a broad per-person spend band. That is
	// useful evidence, but it is not the same fact as the cheapest/most expensive
	// menu item and therefore must not manufacture a price conflict.
	if (claim.Field == "price.minimum" || claim.Field == "price.maximum") && genericVenuePriceBand(claim.Excerpt) {
		if claim.Field == "price.minimum" {
			label = "Typical spend minimum"
		} else {
			label = "Typical spend maximum"
		}
	}
	if strings.HasPrefix(claim.Field, "external.") && value != document.URL && !strings.Contains(document.Content, value) {
		return EvidenceField{}, false
	}
	return EvidenceField{
		Key: label, Value: value, Source: sourceName(document), SourceID: document.ID,
		SourceURL: document.URL, Excerpt: truncateText(strings.Join(strings.Fields(claim.Excerpt), " "), 500),
		CapturedAt: firstNonEmpty(document.CapturedAt, capturedAt.Format(time.RFC3339)), PublishedAt: document.PublishedAt,
		Method: "llm_structured", Extractor: "openrouter:" + model, Confidence: normalizeModelConfidence(claim.Confidence, document.Kind),
	}, true
}

func modelMenuItemToEvidence(item ModelMenuItem, documents map[string]SourceDocument, model string, capturedAt time.Time) (EvidenceField, bool) {
	document, sourceExists := documents[item.SourceID]
	if !sourceExists || !excerptSupported(document.Content, item.Excerpt) {
		return EvidenceField{}, false
	}
	sourceLabel := strings.Join(strings.Fields(item.SourceLabel), " ")
	normalizedName := strings.Join(strings.Fields(item.NormalizedName), " ")
	label := firstNonEmpty(normalizedName, sourceLabel)
	if label == "" {
		return EvidenceField{}, false
	}
	parts := []string{truncateText(label, 140)}
	if sourceLabel != "" && !strings.EqualFold(sourceLabel, label) && !looksLikeSourceDescriptor(sourceLabel) {
		parts = append(parts, "source label: "+truncateText(sourceLabel, 140))
	}
	if category := strings.TrimSpace(item.Category); category != "" {
		parts = append(parts, "category: "+category)
	}
	if item.Price != nil && *item.Price >= 0 {
		currency := strings.ToUpper(strings.TrimSpace(item.Currency))
		if currency == "" || currency == "IDR" || currency == "RP" {
			parts = append(parts, fmt.Sprintf("Rp%d", *item.Price))
		} else {
			parts = append(parts, fmt.Sprintf("%s %d", currency, *item.Price))
		}
	}
	if len(item.Attributes) > 0 {
		attributes := make([]string, 0, len(item.Attributes))
		for _, attribute := range item.Attributes {
			if cleaned := strings.Join(strings.Fields(attribute), " "); cleaned != "" {
				if uncertaintyAttribute(cleaned) && !containsAny(normalizeSearchText(item.Excerpt), "unsure", "uncertain", "not sure", "apa ya", "kurang tahu", "tidak tahu", "nggak tahu") {
					continue
				}
				attributes = append(attributes, cleaned)
			}
		}
		if len(attributes) > 0 {
			sort.Strings(attributes)
			parts = append(parts, strings.Join(attributes, ", "))
		}
	}
	return EvidenceField{
		Key: "Menu item", Value: strings.Join(parts, " · "), Source: sourceName(document), SourceID: document.ID,
		SourceURL: document.URL, Excerpt: truncateText(strings.Join(strings.Fields(item.Excerpt), " "), 500),
		CapturedAt: firstNonEmpty(document.CapturedAt, capturedAt.Format(time.RFC3339)), PublishedAt: document.PublishedAt,
		Method: "llm_structured", Extractor: "openrouter:" + model, Confidence: normalizeModelConfidence(item.Confidence, document.Kind),
	}, true
}

func claimSemanticallySupported(field, excerpt string) bool {
	normalized := normalizeSearchText(excerpt)
	switch field {
	case "work.crowd":
		return containsAny(normalized, "crowd", "crowded", "ramai", "sepi", "packed", "busy", "penuh", "pengunjung", "antre", "antri")
	default:
		return true
	}
}

func uncertaintyAttribute(value string) bool {
	normalized := normalizeSearchText(value)
	return containsAny(normalized, "unsure", "uncertain", "not sure", "exact name")
}

func looksLikeSourceDescriptor(value string) bool {
	normalized := strings.ToLower(strings.Join(strings.Fields(value), " "))
	return strings.HasPrefix(normalized, "google maps review") || strings.HasPrefix(normalized, "review by ") || strings.HasPrefix(normalized, "exa ")
}

func normalizeClaimValue(field, raw string) (string, bool) {
	value := strings.Join(strings.Fields(raw), " ")
	if value == "" {
		return "", false
	}
	if booleanExtractionFields[field] {
		switch strings.ToLower(strings.Trim(value, ".")) {
		case "yes", "true", "available", "ada", "tersedia", "allowed":
			return "Yes", true
		case "no", "false", "not available", "tidak ada", "tidak tersedia", "not allowed", "dilarang":
			return "No", true
		default:
			return "", false
		}
	}
	if strings.HasPrefix(field, "price.") {
		price, ok := normalizePrice(value)
		if !ok {
			return "", false
		}
		return fmt.Sprintf("Rp%d", price), true
	}
	if field == "location.latitude" || field == "location.longitude" {
		number, err := strconv.ParseFloat(strings.ReplaceAll(value, ",", "."), 64)
		if err != nil || (field == "location.latitude" && (number < -90 || number > 90)) || (field == "location.longitude" && (number < -180 || number > 180)) {
			return "", false
		}
		return strconv.FormatFloat(number, 'f', 6, 64), true
	}
	if strings.HasPrefix(field, "external.") {
		parsed, ok := publicWebURL(value)
		if !ok {
			return "", false
		}
		return parsed.String(), true
	}
	return truncateText(value, 500), true
}

func normalizePrice(value string) (int, bool) {
	match := priceNumberPattern.FindStringSubmatch(value)
	if len(match) < 2 {
		return 0, false
	}
	digits := strings.ReplaceAll(strings.ReplaceAll(match[1], ".", ""), ",", "")
	amount, err := strconv.Atoi(digits)
	if err != nil || amount < 0 {
		return 0, false
	}
	if len(match) > 2 && match[2] != "" {
		amount *= 1000
	}
	return amount, true
}

func normalizeModelConfidence(value float64, sourceKind string) float64 {
	if value < 0 {
		value = 0
	}
	maximum := .9
	switch sourceKind {
	case "google_places_review":
		maximum = .88
	case "exa_tiktok_review":
		maximum = .7
	case "exa_web_review":
		maximum = .72
	case "exa_google_maps_review", "exa_highlight":
		maximum = .78
	}
	if value > maximum {
		value = maximum
	}
	return value
}

func excerptSupported(content, excerpt string) bool {
	normalizedExcerpt := normalizeEvidenceText(excerpt)
	return len([]rune(normalizedExcerpt)) >= 4 && strings.Contains(normalizeEvidenceText(content), normalizedExcerpt)
}

func normalizeEvidenceText(value string) string {
	value = strings.Map(func(character rune) rune {
		if unicode.IsSpace(character) {
			return ' '
		}
		return unicode.ToLower(character)
	}, value)
	return strings.Join(strings.Fields(value), " ")
}

func evidenceExists(fields []EvidenceField, candidate EvidenceField) bool {
	fingerprint := evidenceFingerprint(candidate)
	for _, field := range fields {
		if evidenceFingerprint(field) == fingerprint {
			return true
		}
	}
	return false
}

func evidenceFingerprint(field EvidenceField) string {
	return normalizeEvidenceText(field.Key) + "|" + normalizeEvidenceText(field.Value) + "|" + strings.ToLower(field.SourceURL)
}

func detectEvidenceConflicts(record ExtractedRecord) ExtractedRecord {
	type valuesBySource map[string]map[string]struct{}
	groups := map[string]valuesBySource{}
	for _, field := range record.Fields {
		if field.Excluded || !conflictSensitiveFields[field.Key] || field.SourceURL == "" {
			continue
		}
		value := canonicalConflictValue(field.Key, field.Value)
		if value == "" {
			continue
		}
		if groups[field.Key] == nil {
			groups[field.Key] = valuesBySource{}
		}
		if groups[field.Key][value] == nil {
			groups[field.Key][value] = map[string]struct{}{}
		}
		groups[field.Key][value][strings.ToLower(field.SourceURL)] = struct{}{}
	}
	conflicted := map[string]bool{}
	for key, values := range groups {
		sources := map[string]struct{}{}
		for _, valueSources := range values {
			for source := range valueSources {
				sources[source] = struct{}{}
			}
		}
		if len(values) > 1 && len(sources) > 1 && !hasAuthoritativeGoogleWinner(key, record.Fields) {
			conflicted[key] = true
		}
	}
	for index := range record.Fields {
		record.Fields[index].Conflict = !record.Fields[index].Excluded && conflicted[record.Fields[index].Key]
	}
	issues := record.Issues[:0]
	for _, issue := range record.Issues {
		if !strings.HasPrefix(issue, "Conflicting evidence for ") {
			issues = append(issues, issue)
		}
	}
	record.Issues = issues
	for key := range conflicted {
		issue := "Conflicting evidence for " + key
		if !containsString(record.Issues, issue) {
			record.Issues = append(record.Issues, issue)
		}
	}
	return record
}

func canonicalConflictValue(key, value string) string {
	if key == "Area" {
		value = cleanAreaName(value)
	}
	if key == "Address" {
		if signature := canonicalAddressSignature(value); signature != "" {
			return signature
		}
	}
	return strings.Map(func(character rune) rune {
		if unicode.IsLetter(character) || unicode.IsDigit(character) {
			return unicode.ToLower(character)
		}
		return -1
	}, value)
}

// canonicalAddressSignature deliberately uses only the street, number and postal
// code. Administrative names are frequently abbreviated by one source and fully
// expanded by another even when both identify the exact same storefront.
func canonicalAddressSignature(value string) string {
	normalized := normalizeSearchText(value)
	normalized = strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(normalized, "jalan "), "jl "))
	marker := addressNumberPattern.FindStringSubmatchIndex(normalized)
	postal := postalCodePattern.FindString(normalized)
	if len(marker) < 4 || postal == "" {
		return ""
	}
	street := strings.Map(func(character rune) rune {
		if unicode.IsLetter(character) || unicode.IsDigit(character) {
			return character
		}
		return -1
	}, strings.TrimSpace(normalized[:marker[0]]))
	number := normalized[marker[2]:marker[3]]
	if street == "" || number == "" {
		return ""
	}
	return street + "|" + number + "|" + postal
}

// Current Google Places fields are our strongest source for operational facts.
// A lower-confidence indexed listing remains visible in the audit trail, but it
// should not block publication unless it is almost as strong or manually verified.
func hasAuthoritativeGoogleWinner(key string, fields []EvidenceField) bool {
	if key != "Opening hours" && key != "Minimum price" && key != "Maximum price" {
		return false
	}
	best := -1.0
	for _, field := range fields {
		if field.Excluded || field.Key != key {
			continue
		}
		if field.Method == "api" && field.Extractor == "google_places" && field.Confidence >= .8 && field.Confidence > best {
			best = field.Confidence
		}
	}
	if best < 0 {
		return false
	}
	for _, field := range fields {
		if field.Excluded || field.Key != key || (field.Method == "api" && field.Extractor == "google_places") {
			continue
		}
		if field.Method == "manual" || field.Confidence > best-.1 {
			return false
		}
	}
	return true
}

func genericVenuePriceBand(excerpt string) bool {
	normalized := strings.ToLower(strings.Join(strings.Fields(excerpt), " "))
	return strings.HasPrefix(normalized, "$") ||
		strings.Contains(normalized, "price range") ||
		strings.Contains(normalized, "rentang harga per orang") ||
		strings.Contains(normalized, "per person") ||
		strings.Contains(normalized, "per orang")
}

func chooseRecordIdentity(record ExtractedRecord) ExtractedRecord {
	bestNameConfidence, bestAreaConfidence := -1.0, -1.0
	for _, field := range record.Fields {
		if field.Conflict || field.Excluded {
			continue
		}
		switch field.Key {
		case "Identity":
			if field.Confidence > bestNameConfidence {
				record.Name, bestNameConfidence = field.Value, field.Confidence
			}
		case "Area":
			if field.Confidence > bestAreaConfidence {
				record.Area, bestAreaConfidence = field.Value, field.Confidence
			}
		}
	}
	if bestNameConfidence >= 0 {
		record.Issues = removeString(record.Issues, missingCafeNameIssue)
	}
	return record
}

func sourceName(document SourceDocument) string {
	switch document.Kind {
	case "google_places_review":
		return "Google Maps review"
	case "exa_google_maps_review":
		return "Google Maps review context"
	case "exa_tiktok_review":
		return "TikTok review context"
	case "exa_web_review", "exa_highlight":
		return "Web review context"
	}
	parsed, err := url.Parse(document.URL)
	if err == nil && parsed.Hostname() != "" {
		return "Extracted · " + parsed.Hostname()
	}
	return "Extracted source"
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func removeString(values []string, target string) []string {
	filtered := values[:0]
	for _, value := range values {
		if value != target {
			filtered = append(filtered, value)
		}
	}
	return filtered
}
