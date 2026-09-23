package recommendation

import (
	"context"
	"encoding/json"
	"testing"
)

func TestMixedLanguageInterpretation(t *testing.T) {
	profile := enforceLocationIntent("Cari cafe buat WFC di Jaksel. Harus ada musholla, quiet and minimalist, around 50k. Bonus kalau ada matcha.", InterpretDeterministically("Cari cafe buat WFC di Jaksel. Harus ada musholla, quiet and minimalist, around 50k. Bonus kalau ada matcha."))
	if profile.Location != "Jakarta Selatan" || profile.Budget != 50000 {
		t.Fatalf("unexpected entities: %#v", profile)
	}
	if len(profile.HardConstraints) != 1 || profile.HardConstraints[0].Key != "musholla" {
		t.Fatalf("hard constraints = %#v", profile.HardConstraints)
	}
}

func TestNamedAreaIsAHardGeographicConstraint(t *testing.T) {
	profile := enforceLocationIntent("cafe untuk ngopi di cipete", InterpretDeterministically("cafe untuk ngopi di cipete"))
	results := RankCafes([]Cafe{
		{ID: "cipete", Name: "Cipete Cafe", Area: "Cipete Selatan", Address: "Jl. Cipete Raya, Jakarta Selatan", Lat: -6.277, Lng: 106.801, Scores: map[string]float64{}, Evidence: map[string]float64{}, Facts: map[string]bool{}},
		{ID: "tebet", Name: "Tebet Cafe", Area: "Tebet", Address: "Jl. Tebet Raya, Jakarta Selatan", Lat: -6.238, Lng: 106.851, Scores: map[string]float64{}, Evidence: map[string]float64{}, Facts: map[string]bool{}},
	}, profile, nil)
	if len(results) != 1 || results[0].PlaceID != "cipete" || profile.LocationMode != "named" {
		t.Fatalf("Cipete query leaked results from another area: profile=%#v results=%#v", profile, results)
	}
}

func TestJakselIncludesItsDistrictsAndExcludesJaktim(t *testing.T) {
	profile := enforceLocationIntent("cafe di jaksel", InterpretDeterministically("cafe di jaksel"))
	results := RankCafes([]Cafe{
		{ID: "cipete", Name: "Cipete Cafe", Area: "Cipete", Address: "Jakarta Selatan", Lat: -6.277, Lng: 106.801, Scores: map[string]float64{}, Evidence: map[string]float64{}, Facts: map[string]bool{}},
		{ID: "tebet", Name: "Tebet Cafe", Area: "Tebet", Address: "Jakarta Selatan", Lat: -6.238, Lng: 106.851, Scores: map[string]float64{}, Evidence: map[string]float64{}, Facts: map[string]bool{}},
		{ID: "rawamangun", Name: "Rawamangun Cafe", Area: "Rawamangun", Address: "Jakarta Timur", Lat: -6.197, Lng: 106.891, Scores: map[string]float64{}, Evidence: map[string]float64{}, Facts: map[string]bool{}},
		{ID: "duren-sawit", Name: "Duren Sawit Cafe", Area: "Duren Sawit", Address: "Jakarta Timur", Lat: -6.234, Lng: 106.925, Scores: map[string]float64{}, Evidence: map[string]float64{}, Facts: map[string]bool{}},
	}, profile, nil)
	if len(results) != 2 {
		t.Fatalf("Jaksel should include Cipete/Tebet only: %#v", results)
	}
	for _, result := range results {
		if result.PlaceID == "rawamangun" || result.PlaceID == "duren-sawit" {
			t.Fatalf("Jaktim result leaked into Jaksel search: %#v", result)
		}
	}
}

func TestNearMeUsesUserPositionAndFiveKilometreRadius(t *testing.T) {
	profile := enforceLocationIntent("cafe near me", InterpretDeterministically("cafe near me"))
	origin := &Point{Lat: -6.2778, Lng: 106.8007}
	results := RankCafes([]Cafe{
		{ID: "near", Name: "Near Cafe", Area: "Cipete", Lat: -6.278, Lng: 106.801, Scores: map[string]float64{}, Evidence: map[string]float64{}, Facts: map[string]bool{}},
		{ID: "far", Name: "Far Cafe", Area: "Rawamangun", Lat: -6.197, Lng: 106.891, Scores: map[string]float64{}, Evidence: map[string]float64{}, Facts: map[string]bool{}},
	}, profile, origin)
	if !RequiresUserLocation("cafe sekitar saya") || len(results) != 1 || results[0].PlaceID != "near" {
		t.Fatalf("near-me location filtering failed: profile=%#v results=%#v", profile, results)
	}
}

func TestUnspecifiedLocationDefaultsToUserPosition(t *testing.T) {
	origin := &Point{Lat: -6.2778, Lng: 106.8007}
	profile := enforceLocationIntent("quiet cafe for working", InterpretDeterministically("quiet cafe for working"))
	profile = applyDefaultUserLocation(profile, origin)
	results := RankCafes([]Cafe{
		{ID: "near", Name: "Near Cafe", Area: "Cipete", Lat: -6.278, Lng: 106.801, Scores: map[string]float64{"quiet": .9}, Evidence: map[string]float64{}, Facts: map[string]bool{}},
		{ID: "far", Name: "Far Cafe", Area: "Rawamangun", Lat: -6.197, Lng: 106.891, Scores: map[string]float64{"quiet": .9}, Evidence: map[string]float64{}, Facts: map[string]bool{}},
	}, profile, origin)
	if profile.LocationMode != "nearby" || profile.Location != "Near you" || len(results) != 1 || results[0].PlaceID != "near" {
		t.Fatalf("location-less query did not default to nearby results: profile=%#v results=%#v", profile, results)
	}

	named := enforceLocationIntent("quiet cafe di Tebet", InterpretDeterministically("quiet cafe di Tebet"))
	named = applyDefaultUserLocation(named, origin)
	if named.LocationMode != "named" || named.Location != "Tebet" {
		t.Fatalf("device location overrode an explicit named area: %#v", named)
	}
}

func TestHardFiltersAreNeverViolated(t *testing.T) {
	profile := InterpretDeterministically("WFC di Jaksel, wajib ada musholla dan harus ada matcha")
	results := RankCafes(SeedCafes(), profile, nil)
	for _, result := range results {
		var found Cafe
		for _, cafe := range SeedCafes() {
			if cafe.ID == result.PlaceID {
				found = cafe
				break
			}
		}
		if !found.Facts["musholla"] || !found.Facts["matcha"] {
			t.Fatalf("hard constraint violation: %s", result.Name)
		}
	}
}

func TestOpen24HoursIsDistinctFromOpenLate(t *testing.T) {
	profile := enforceDeterministicConstraints("harus 24 jam", Interpretation{
		Location: "Blok M", SoftPreferences: []Requirement{{Key: "late", Label: "Open late", Kind: "soft"}},
	})
	results := RankCafes(SeedCafes(), profile, nil)
	if len(results) != 2 || len(profile.HardConstraints) != 1 || profile.HardConstraints[0].Key != "open_24h" {
		t.Fatalf("unexpected 24-hour result: profile=%#v results=%#v", profile, results)
	}
	for _, result := range results {
		if !result.Open24Hours {
			t.Fatalf("non-24-hour result returned: %#v", result)
		}
	}
}

func TestRecommendationResponseSerializesEmptyConstraintsAsArrays(t *testing.T) {
	service := NewService(deterministicInterpreter{})
	response := service.Recommend(context.Background(), Request{Query: "Garden cafe in Kemang for brunch", Limit: 15})

	if response.Interpretation.HardConstraints == nil || response.Interpretation.SoftPreferences == nil {
		t.Fatalf("recommendation collections must be initialized: %#v", response.Interpretation)
	}

	encoded, err := json.Marshal(response)
	if err != nil {
		t.Fatal(err)
	}
	var payload struct {
		Interpretation struct {
			HardConstraints json.RawMessage `json:"hard_constraints"`
			SoftPreferences json.RawMessage `json:"soft_preferences"`
		} `json:"interpretation"`
	}
	if err := json.Unmarshal(encoded, &payload); err != nil {
		t.Fatal(err)
	}
	if string(payload.Interpretation.HardConstraints) != "[]" {
		t.Fatalf("hard_constraints must serialize as [], got %s", payload.Interpretation.HardConstraints)
	}
	if string(payload.Interpretation.SoftPreferences) == "null" {
		t.Fatalf("soft_preferences must serialize as an array, got null")
	}
}
