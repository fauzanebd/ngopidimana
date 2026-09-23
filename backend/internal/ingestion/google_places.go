package ingestion

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/fauzanebd/wheretowfc/backend/internal/googleplaces"
)

type placeDetailsProvider interface {
	GetPlace(context.Context, string) (googleplaces.Place, error)
}

func mergeGooglePlaceEvidence(record ExtractedRecord, place googleplaces.Place, capturedAt time.Time) ExtractedRecord {
	captured := capturedAt.Format(time.RFC3339)
	appendField := func(field EvidenceField) {
		if !evidenceExists(record.Fields, field) {
			record.Fields = append(record.Fields, field)
		}
	}
	if strings.TrimSpace(place.FormattedAddress) != "" {
		appendField(EvidenceField{Key: "Address", Value: place.FormattedAddress, Source: "Google Maps", SourceID: "google_place", SourceURL: place.GoogleMapsURI, CapturedAt: captured, Method: "api", Extractor: "google_places", Confidence: .97})
	}
	if area := googlePlaceArea(place.DisplayName.Text, place.AddressComponents); area != "" {
		appendField(EvidenceField{Key: "Area", Value: area, Source: "Google Maps", SourceID: "google_place", SourceURL: place.GoogleMapsURI, CapturedAt: captured, Method: "api", Extractor: "google_places", Confidence: .96})
		if strings.TrimSpace(record.Area) == "" {
			record.Area = area
		}
	}
	if place.Location.Latitude != 0 || place.Location.Longitude != 0 {
		appendField(EvidenceField{Key: "Latitude", Value: strconv.FormatFloat(place.Location.Latitude, 'f', 6, 64), Source: "Google Maps", SourceID: "google_place", SourceURL: place.GoogleMapsURI, CapturedAt: captured, Method: "api", Extractor: "google_places", Confidence: .99})
		appendField(EvidenceField{Key: "Longitude", Value: strconv.FormatFloat(place.Location.Longitude, 'f', 6, 64), Source: "Google Maps", SourceID: "google_place", SourceURL: place.GoogleMapsURI, CapturedAt: captured, Method: "api", Extractor: "google_places", Confidence: .99})
	}
	if len(place.RegularOpeningHours.WeekdayDescriptions) > 0 {
		hours := strings.Join(place.RegularOpeningHours.WeekdayDescriptions, "; ")
		appendField(EvidenceField{Key: "Opening hours", Value: hours, Source: "Google Maps", SourceID: "google_place", SourceURL: place.GoogleMapsURI, CapturedAt: captured, Method: "api", Extractor: "google_places", Confidence: .95})
		lowerHours := strings.ToLower(hours)
		if strings.Count(lowerHours, "24 jam") >= 7 || strings.Count(lowerHours, "open 24 hours") >= 7 {
			appendField(EvidenceField{Key: "Open 24 hours", Value: "Yes", Source: "Google Maps", SourceID: "google_place", SourceURL: place.GoogleMapsURI, CapturedAt: captured, Method: "deterministic", Extractor: "google_places_hours", Confidence: .97})
		}
	}
	if place.GoogleMapsURI != "" {
		appendField(EvidenceField{Key: "Google Maps", Value: place.GoogleMapsURI, Source: "Google Maps", SourceID: "google_place", SourceURL: place.GoogleMapsURI, CapturedAt: captured, Method: "api", Extractor: "google_places", Confidence: 1})
	}
	if place.WebsiteURI != "" {
		appendField(EvidenceField{Key: "Website", Value: place.WebsiteURI, Source: "Google Maps", SourceID: "google_place", SourceURL: place.GoogleMapsURI, CapturedAt: captured, Method: "api", Extractor: "google_places", Confidence: .9})
	}
	if amount, ok := googleMoneyAmount(place.PriceRange.StartPrice); ok {
		appendField(EvidenceField{Key: "Minimum price", Value: fmt.Sprintf("Rp%d", amount), Source: "Google Maps", SourceID: "google_place", SourceURL: place.GoogleMapsURI, CapturedAt: captured, Method: "api", Extractor: "google_places", Confidence: .85})
	}
	if amount, ok := googleMoneyAmount(place.PriceRange.EndPrice); ok {
		appendField(EvidenceField{Key: "Maximum price", Value: fmt.Sprintf("Rp%d", amount), Source: "Google Maps", SourceID: "google_place", SourceURL: place.GoogleMapsURI, CapturedAt: captured, Method: "api", Extractor: "google_places", Confidence: .85})
	}
	if place.Rating > 0 {
		value := fmt.Sprintf("%.1f", place.Rating)
		if place.UserRatingCount > 0 {
			value += fmt.Sprintf(" from %d ratings", place.UserRatingCount)
		}
		record.Fields = append(record.Fields, EvidenceField{
			Key: "Google rating", Value: value, Source: "Google Maps", SourceID: "google_place",
			SourceURL: place.GoogleMapsURI, CapturedAt: captured, Method: "api", Extractor: "google_places", Confidence: .95,
		})
	}
	for index, review := range place.Reviews {
		excerpt := strings.Join(strings.Fields(firstNonEmpty(review.OriginalText.Text, review.Text.Text)), " ")
		if excerpt == "" {
			continue
		}
		sourceID := fmt.Sprintf("google_review_%d", index+1)
		sourceURL := firstNonEmpty(review.GoogleMapsURI, place.GoogleMapsURI)
		publishedAt := review.PublishTime
		record.Fields = append(record.Fields, EvidenceField{
			Key: "Google review", Value: fmt.Sprintf("%.0f/5 · %s", review.Rating, firstNonEmpty(review.AuthorAttribution.DisplayName, "Google Maps user")),
			Source: "Google Maps", SourceID: sourceID, SourceURL: sourceURL, Excerpt: truncateText(excerpt, 800),
			CapturedAt: captured, PublishedAt: publishedAt, Method: "api", Extractor: "google_places", Confidence: .88,
			AuthorName: review.AuthorAttribution.DisplayName, AuthorURL: review.AuthorAttribution.URI, AuthorPhoto: review.AuthorAttribution.PhotoURI,
		})
		record.Documents = append(record.Documents, SourceDocument{
			ID: sourceID, URL: sourceURL, Kind: "google_places_review", Title: "Google Maps review by " + firstNonEmpty(review.AuthorAttribution.DisplayName, "a user"),
			Content: excerpt, CapturedAt: captured, PublishedAt: publishedAt,
		})
		record = mergeDeterministicReviewSignals(record, record.Documents[len(record.Documents)-1], captured)
	}
	return record
}

func googleMoneyAmount(money googleplaces.Money) (int, bool) {
	if money.CurrencyCode != "IDR" || strings.TrimSpace(money.Units) == "" {
		return 0, false
	}
	amount, err := strconv.Atoi(money.Units)
	return amount, err == nil && amount >= 0
}

func googlePlaceArea(placeName string, components []googleplaces.AddressComponent) string {
	if branch := branchArea(placeName); branch != "" {
		return branch
	}
	preferredTypes := []string{"neighborhood", "sublocality_level_2", "sublocality_level_1", "administrative_area_level_3"}
	for _, preferred := range preferredTypes {
		for _, component := range components {
			for _, componentType := range component.Types {
				if componentType == preferred && strings.TrimSpace(component.LongText) != "" {
					return cleanAreaName(component.LongText)
				}
			}
		}
	}
	return ""
}

func branchArea(placeName string) string {
	normalized := strings.NewReplacer("–", "-", "—", "-").Replace(strings.TrimSpace(placeName))
	parts := strings.Split(normalized, "-")
	if len(parts) < 2 {
		return ""
	}
	candidate := strings.TrimSpace(parts[len(parts)-1])
	if candidate == "" || len([]rune(candidate)) > 40 || strings.ContainsAny(candidate, "/@") {
		return ""
	}
	return candidate
}

func cleanAreaName(value string) string {
	value = strings.TrimSpace(value)
	for _, prefix := range []string{"Kecamatan ", "Kec. ", "Kelurahan "} {
		if strings.HasPrefix(strings.ToLower(value), strings.ToLower(prefix)) {
			return strings.TrimSpace(value[len(prefix):])
		}
	}
	return value
}

func mergeDeterministicReviewSignals(record ExtractedRecord, document SourceDocument, captured string) ExtractedRecord {
	normalized := normalizeSearchText(document.Content)
	appendSignal := func(key, value, excerpt string, confidence float64) {
		field := EvidenceField{Key: key, Value: value, Source: "Google Maps review", SourceID: document.ID, SourceURL: document.URL, Excerpt: excerpt, CapturedAt: captured, PublishedAt: document.PublishedAt, Method: "deterministic", Extractor: "google_review_rules", Confidence: confidence}
		if !evidenceExists(record.Fields, field) {
			record.Fields = append(record.Fields, field)
		}
	}
	if containsAny(normalized, "colokan banyak", "banyak colokan", "colokan tersedia", "power outlet tersedia") {
		appendSignal("Power outlets", "Yes", matchingPhrase(document.Content, "colokan", "outlet"), .92)
	}
	if strings.Contains(normalized, "wifi") || strings.Contains(normalized, "wi fi") {
		switch {
		case containsAny(normalized, "susah connect", "sulit connect", "susah konek", "sulit konek", "wifi lambat", "wi fi lambat"):
			appendSignal("Wi-Fi quality", "Reported difficult to connect", matchingPhrase(document.Content, "WiFi", "Wi-Fi", "wifi"), .9)
		case containsAny(normalized, "wifi cepat", "wi fi cepat", "wifi stabil", "wi fi stabil"):
			appendSignal("Wi-Fi quality", "Reported fast and stable", matchingPhrase(document.Content, "WiFi", "Wi-Fi", "wifi"), .88)
		}
	}
	switch {
	case containsAny(normalized, "tidak tenang", "nggak tenang", "tidak kondusif", "berisik", "noisy"):
		appendSignal("Quietness", "Reported noisy", matchingPhrase(document.Content, "tenang", "kondusif", "berisik", "noisy"), .86)
	case containsAny(normalized, "suasana tenang", "nyaman dan tenang", "tempat tenang", "kondusif", "quiet"):
		appendSignal("Quietness", "Reported quiet", matchingPhrase(document.Content, "tenang", "kondusif", "quiet"), .88)
	}
	if containsAny(normalized, "luas dan banyak tempat duduk", "banyak tempat duduk", "plenty of seats") {
		appendSignal("Seating comfort", "Reported spacious with many seats", matchingPhrase(document.Content, "tempat duduk", "seats"), .86)
	}
	if (strings.Contains(normalized, "mobil") || strings.Contains(normalized, "car")) && containsAny(normalized, "4 5 mobil", "cukup untuk 4", "only enough for 4", "limited car parking") {
		appendSignal("Parking notes", "Car parking reported limited to around 4–5 vehicles", matchingPhrase(document.Content, "mobil", "car"), .86)
	}
	return record
}

func containsAny(content string, phrases ...string) bool {
	for _, phrase := range phrases {
		if strings.Contains(content, normalizeSearchText(phrase)) {
			return true
		}
	}
	return false
}

func matchingPhrase(content string, needles ...string) string {
	for _, needle := range needles {
		if strings.Contains(strings.ToLower(content), strings.ToLower(needle)) {
			return truncateText(strings.TrimSpace(content), 500)
		}
	}
	return truncateText(content, 180)
}
