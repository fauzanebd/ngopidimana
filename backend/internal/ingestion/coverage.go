package ingestion

import "strings"

func evidenceCoverage(fields []EvidenceField) float64 {
	present := map[string]bool{}
	for _, field := range fields {
		if field.Conflict || field.Excluded || strings.TrimSpace(field.Value) == "" {
			continue
		}
		present[field.Key] = true
	}
	groups := []struct {
		weight float64
		keys   []string
		cap    int
	}{
		{.10, []string{"Identity", "Google Maps"}, 2},
		{.15, []string{"Google rating", "Google review"}, 2},
		{.15, []string{"Address", "Area", "Latitude", "Longitude"}, 4},
		{.15, []string{"Opening hours", "Open 24 hours"}, 1},
		{.10, []string{"Minimum price", "Maximum price", "Typical spend minimum", "Typical spend maximum", "Menu item"}, 2},
		{.20, []string{"Musholla", "Toilet", "Wi-Fi", "Power outlets", "Car parking", "Motorcycle parking", "Accessibility", "Air conditioning", "Indoor seating", "Outdoor seating", "Smoking zone"}, 4},
		{.15, []string{"Quietness", "Crowd level", "Table size", "Seating comfort", "Lighting", "Laptop policy", "Long-stay friendliness", "Calls suitability", "Meetings suitability", "Wi-Fi quality", "Ambience", "Other note", "Manual caveat"}, 4},
	}
	coverage := 0.0
	for _, group := range groups {
		count := 0
		for _, key := range group.keys {
			if present[key] {
				count++
			}
		}
		coverage += group.weight * min(float64(count)/float64(group.cap), 1)
	}
	return float64(int(min(coverage, 1)*100+0.5)) / 100
}
