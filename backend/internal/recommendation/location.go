package recommendation

import "strings"

const nearbyRadiusKM = 5.0

type locationScope struct {
	Label      string
	Kind       string
	Aliases    []string
	MatchTerms []string
	Center     Point
}

// The catalogue stores a reviewed area plus a full address rather than a
// separate administrative hierarchy. These scopes make common Jakarta names
// deterministic and let a municipality query include its kecamatan/kelurahan.
var locationScopes = []locationScope{
	{Label: "Blok M", Kind: "area", Aliases: []string{"blok m", "melawai"}, MatchTerms: []string{"blok m", "melawai"}, Center: Point{-6.2447, 106.8001}},
	{Label: "Kemang", Kind: "area", Aliases: []string{"kemang"}, MatchTerms: []string{"kemang", "bangka"}, Center: Point{-6.2607, 106.8133}},
	{Label: "Cipete", Kind: "area", Aliases: []string{"cipete"}, MatchTerms: []string{"cipete"}, Center: Point{-6.2778, 106.8007}},
	{Label: "Senopati", Kind: "area", Aliases: []string{"senopati", "scbd"}, MatchTerms: []string{"senopati", "scbd", "sudirman central business district"}, Center: Point{-6.2297, 106.8075}},
	{Label: "Tebet", Kind: "area", Aliases: []string{"tebet"}, MatchTerms: []string{"tebet", "menteng dalam", "manggarai selatan", "bukit duri", "kebon baru"}, Center: Point{-6.2359, 106.8523}},
	{Label: "Cilandak", Kind: "district", Aliases: []string{"cilandak", "lebak bulus", "pondok labu"}, MatchTerms: []string{"cilandak", "lebak bulus", "pondok labu", "gandaria selatan", "cipete selatan"}, Center: Point{-6.2898, 106.7992}},
	{Label: "Rawamangun", Kind: "area", Aliases: []string{"rawamangun"}, MatchTerms: []string{"rawamangun"}, Center: Point{-6.1974, 106.8914}},
	{Label: "Duren Sawit", Kind: "district", Aliases: []string{"duren sawit"}, MatchTerms: []string{"duren sawit", "pondok bambu", "klender", "malaka"}, Center: Point{-6.2345, 106.9248}},
	{
		Label: "Jakarta Selatan", Kind: "city", Aliases: []string{"jakarta selatan", "jaksel", "south jakarta"},
		MatchTerms: []string{"jakarta selatan", "kebayoran baru", "kebayoran lama", "pesanggrahan", "cilandak", "pasar minggu", "jagakarsa", "mampang prapatan", "mampang", "pancoran", "tebet", "setiabudi", "blok m", "melawai", "kemang", "bangka", "cipete", "senopati", "scbd", "lebak bulus", "pondok labu", "fatmawati", "gandaria", "radio dalam", "panglima polim", "wijaya", "kuningan", "pejaten", "ampera", "ragunan"},
		Center:     Point{-6.2615, 106.8106},
	},
	{
		Label: "Jakarta Timur", Kind: "city", Aliases: []string{"jakarta timur", "jaktim", "east jakarta"},
		MatchTerms: []string{"jakarta timur", "cakung", "cipayung", "ciracas", "duren sawit", "jatinegara", "kramat jati", "makasar", "matraman", "pasar rebo", "pulo gadung", "rawamangun", "klender", "pondok bambu"},
		Center:     Point{-6.2250, 106.9004},
	},
	{
		Label: "Jakarta Pusat", Kind: "city", Aliases: []string{"jakarta pusat", "jakpus", "central jakarta"},
		MatchTerms: []string{"jakarta pusat", "cempaka putih", "gambir", "johar baru", "kemayoran", "menteng", "sawah besar", "senen", "tanah abang"},
		Center:     Point{-6.1805, 106.8284},
	},
	{
		Label: "Jakarta Barat", Kind: "city", Aliases: []string{"jakarta barat", "jakbar", "west jakarta"},
		MatchTerms: []string{"jakarta barat", "cengkareng", "grogol petamburan", "kalideres", "kebon jeruk", "kembangan", "palmerah", "taman sari", "tambora", "puri indah"},
		Center:     Point{-6.1683, 106.7588},
	},
	{
		Label: "Jakarta Utara", Kind: "city", Aliases: []string{"jakarta utara", "jakut", "north jakarta"},
		MatchTerms: []string{"jakarta utara", "cilincing", "kelapa gading", "koja", "pademangan", "penjaringan", "tanjung priok", "pluit"},
		Center:     Point{-6.1214, 106.7741},
	},
}

var nearbyAliases = []string{"near me", "nearby", "around me", "sekitar saya", "dekat saya", "di sekitar sini", "dekat sini"}

func enforceLocationIntent(query string, profile Interpretation) Interpretation {
	if scope, ok := detectNamedLocation(query); ok {
		profile.Location = scope.Label
		profile.LocationMode = "named"
	} else if RequiresUserLocation(query) {
		profile.Location = "Near you"
		profile.LocationMode = "nearby"
	} else {
		profile.LocationMode = "default"
		if profile.Location == "" || profile.Location == "Jakarta Selatan" {
			profile.Location = "Jakarta"
		}
	}
	profile.Summary = interpretationSummary(profile)
	return profile
}

func applyDefaultUserLocation(profile Interpretation, userLocation *Point) Interpretation {
	if profile.LocationMode == "default" && userLocation != nil {
		profile.Location = "Near you"
		profile.LocationMode = "nearby"
		profile.Summary = interpretationSummary(profile)
	}
	return profile
}

func RequiresUserLocation(query string) bool {
	return containsLocationPhrase(normalizeLocationText(query), nearbyAliases)
}

func ValidPoint(point Point) bool {
	return point.Lat >= -90 && point.Lat <= 90 && point.Lng >= -180 && point.Lng <= 180 && !(point.Lat == 0 && point.Lng == 0)
}

func detectNamedLocation(query string) (locationScope, bool) {
	normalized := normalizeLocationText(query)
	for _, scope := range locationScopes {
		if containsLocationPhrase(normalized, scope.Aliases) {
			return scope, true
		}
	}
	return locationScope{}, false
}

func scopeForLabel(label string) (locationScope, bool) {
	for _, scope := range locationScopes {
		if strings.EqualFold(scope.Label, label) {
			return scope, true
		}
	}
	return locationScope{}, false
}

func cafeMatchesScope(cafe Cafe, scope locationScope) bool {
	haystack := normalizeLocationText(cafe.Area + " " + cafe.Address)
	return containsLocationPhrase(haystack, scope.MatchTerms)
}

func normalizeLocationText(value string) string {
	return strings.Join(strings.Fields(strings.Map(func(character rune) rune {
		if character >= 'A' && character <= 'Z' {
			return character + ('a' - 'A')
		}
		if (character >= 'a' && character <= 'z') || (character >= '0' && character <= '9') {
			return character
		}
		return ' '
	}, value)), " ")
}

func containsLocationPhrase(value string, phrases []string) bool {
	padded := " " + value + " "
	for _, phrase := range phrases {
		if strings.Contains(padded, " "+normalizeLocationText(phrase)+" ") {
			return true
		}
	}
	return false
}
