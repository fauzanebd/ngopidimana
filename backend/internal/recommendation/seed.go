package recommendation

import "fmt"

func SeedCafes() []Cafe {
	type spec struct {
		name, area, address, close, description, accent string
		lat, lng                                        float64
		min, max, fresh                                 int
		scores                                          map[string]float64
		facts                                           []string
		tags                                            []string
	}
	specs := []spec{
		{"Teras Sela", "Cipete", "Jl. Damai Raya, Cipete Utara", "22:00", "A leafy, low-volume room made for unhurried laptop sessions.", "sage", -6.274, 106.806, 28000, 52000, 18, map[string]float64{"wfc": .94, "quiet": .92, "minimalist": .82, "outdoor": .74}, []string{"musholla", "wifi", "outlets", "matcha", "parking"}, []string{"weekday calm", "big tables", "natural light"}},
		{"Ruang Hening", "Blok M", "Jl. Melawai VI, Kebayoran Baru", "23:00", "Focused desks, warm timber and a tucked-away prayer room.", "sky", -6.244, 106.799, 32000, 58000, 12, map[string]float64{"wfc": .97, "quiet": .95, "minimalist": .9}, []string{"musholla", "wifi", "outlets", "matcha", "late"}, []string{"focus seats", "quiet zone", "fast Wi-Fi"}},
		{"Atelier Karsa", "Senopati", "Jl. Suryo, Senopati", "22:00", "A composed studio-like café with careful coffee and calm corners.", "lavender", -6.236, 106.814, 42000, 72000, 25, map[string]float64{"wfc": .82, "quiet": .78, "minimalist": .97}, []string{"wifi", "outlets", "matcha", "parking"}, []string{"design-led", "specialty coffee", "soft lighting"}},
		{"Pagi di Taman", "Kemang", "Jl. Kemang Timur, Bangka", "21:00", "Garden tables, early brunch and an easy pace before lunchtime.", "meadow", -6.267, 106.819, 30000, 62000, 34, map[string]float64{"wfc": .74, "quiet": .7, "outdoor": .97, "breakfast": .9}, []string{"musholla", "wifi", "matcha", "parking", "breakfast", "pet_friendly"}, []string{"garden", "brunch", "pet friendly"}},
		{"Nalar Coffee Room", "Fatmawati", "Jl. RS Fatmawati Raya", "24:00", "A dependable late-night work room with plentiful power and seating.", "indigo", -6.292, 106.795, 25000, 49000, 42, map[string]float64{"wfc": .96, "quiet": .72, "minimalist": .7, "late": 1, "open_24h": 1}, []string{"musholla", "wifi", "outlets", "late", "open_24h", "parking"}, []string{"open 24h", "many outlets", "value"}},
		{"Kebun Sore", "Cilandak", "Jl. Lebak Bulus I, Cilandak", "22:00", "Breezy outdoor nooks balanced by a cool indoor work area.", "peach", -6.304, 106.793, 28000, 56000, 9, map[string]float64{"wfc": .81, "quiet": .8, "outdoor": .91}, []string{"musholla", "wifi", "outlets", "matcha", "parking", "pet_friendly"}, []string{"indoor + garden", "fresh evidence", "spacious"}},
		{"Titik Teduh", "Panglima Polim", "Jl. Panglima Polim V", "22:30", "A compact neighborhood café with acoustic treatment and kind service.", "butter", -6.250, 106.795, 26000, 48000, 71, map[string]float64{"wfc": .89, "quiet": .91, "minimalist": .77}, []string{"musholla", "wifi", "outlets", "matcha"}, []string{"acoustic room", "solo seats", "neighborhood"}},
		{"Arunika House", "Wijaya", "Jl. Wijaya II, Kebayoran Baru", "23:00", "A sun-washed house café for meetings that can turn into deep work.", "rose", -6.241, 106.806, 38000, 68000, 55, map[string]float64{"wfc": .84, "quiet": .75, "minimalist": .85}, []string{"musholla", "wifi", "outlets", "matcha", "late", "parking"}, []string{"meeting rooms", "sunny", "all-day menu"}},
		{"Satu Meja", "Tebet", "Jl. Tebet Barat Dalam", "22:00", "Communal tables, reliable internet and a practical neighborhood menu.", "cobalt", -6.238, 106.851, 22000, 45000, 31, map[string]float64{"wfc": .92, "quiet": .7, "minimalist": .64}, []string{"musholla", "wifi", "outlets", "matcha"}, []string{"communal tables", "affordable", "reliable"}},
		{"Studio Pendar", "Gandaria", "Jl. Gandaria Tengah III", "23:00", "Minimal lines and a soft-sound interior with seats for focused work.", "lilac", -6.256, 106.786, 34000, 61000, 46, map[string]float64{"wfc": .9, "quiet": .86, "minimalist": .96}, []string{"musholla", "wifi", "outlets", "matcha", "late"}, []string{"minimal", "soft sound", "focus friendly"}},
		{"Lereng Kota", "Kuningan", "Jl. Karet Pedurenan", "21:30", "A calm city-view room that works best in weekday mornings.", "sky", -6.219, 106.829, 36000, 65000, 78, map[string]float64{"wfc": .83, "quiet": .79, "minimalist": .8}, []string{"musholla", "wifi", "outlets", "parking"}, []string{"city view", "weekday calm", "meeting friendly"}},
		{"Rona Selatan", "Pejaten", "Jl. Pejaten Barat Raya", "22:00", "Warm color, cushioned seats and enough room for a long afternoon.", "coral", -6.280, 106.829, 24000, 47000, 63, map[string]float64{"wfc": .86, "quiet": .74, "minimalist": .65}, []string{"musholla", "wifi", "outlets", "matcha", "parking"}, []string{"comfortable seats", "long stay", "budget friendly"}},
		{"Kopi Di Antara", "Setiabudi", "Jl. Taman Setiabudi II", "23:30", "Hidden behind a courtyard, with quiet booths and a concise menu.", "sage", -6.210, 106.829, 30000, 54000, 22, map[string]float64{"wfc": .91, "quiet": .9, "minimalist": .79}, []string{"musholla", "wifi", "outlets", "late"}, []string{"quiet booths", "courtyard", "late close"}},
		{"Bawah Pohon", "Ampera", "Jl. Ampera Raya", "21:00", "A relaxed open-air café shaded by trees, ideal for casual work.", "meadow", -6.289, 106.817, 26000, 50000, 15, map[string]float64{"wfc": .76, "quiet": .73, "outdoor": .98}, []string{"musholla", "wifi", "matcha", "parking", "pet_friendly"}, []string{"shaded garden", "pet friendly", "casual work"}},
		{"Sore Studio", "Radio Dalam", "Jl. Radio Dalam Raya", "23:00", "A crisp, compact room with strong Wi-Fi and rotating matcha drinks.", "butter", -6.261, 106.789, 29000, 53000, 28, map[string]float64{"wfc": .9, "quiet": .82, "minimalist": .93}, []string{"wifi", "outlets", "matcha", "late"}, []string{"seasonal matcha", "compact", "fast Wi-Fi"}},
		{"Beranda 72", "Pasar Minggu", "Jl. Ragunan Raya", "22:00", "A generous terrace café with affordable food and easy parking.", "peach", -6.285, 106.839, 20000, 44000, 39, map[string]float64{"wfc": .78, "quiet": .69, "outdoor": .89}, []string{"musholla", "wifi", "outlets", "parking", "breakfast"}, []string{"value menu", "terrace", "easy parking"}},
		{"Jeda Pagi", "Kebayoran Lama", "Jl. Ciputat Raya", "20:30", "Bright mornings, simple breakfast and a quiet back room.", "rose", -6.267, 106.779, 23000, 46000, 49, map[string]float64{"wfc": .85, "quiet": .84, "minimalist": .72, "breakfast": .94}, []string{"musholla", "wifi", "outlets", "matcha", "breakfast"}, []string{"breakfast", "quiet back room", "morning light"}},
		{"Lantai Dua", "Mampang", "Jl. Bangka Raya", "24:00", "A two-floor late-night café: lively below, focused and quiet upstairs.", "indigo", -6.253, 106.820, 25000, 50000, 67, map[string]float64{"wfc": .93, "quiet": .81, "minimalist": .67, "late": 1, "open_24h": 1}, []string{"musholla", "wifi", "outlets", "matcha", "late", "open_24h", "parking"}, []string{"open 24h", "quiet upstairs", "many seats"}},
	}
	out := make([]Cafe, 0, len(specs))
	for index, item := range specs {
		facts, evidence := map[string]bool{}, map[string]float64{}
		for _, fact := range item.facts {
			facts[fact] = true
		}
		for key := range item.scores {
			evidence[key] = .86
		}
		for key := range facts {
			evidence[key] = .94
		}
		out = append(out, Cafe{
			ID: fmt.Sprintf("cafe_%02d", index+1), Name: item.name, Area: item.area, Address: item.address,
			Lat: item.lat, Lng: item.lng, PriceMin: item.min, PriceMax: item.max, OpenUntil: item.close,
			Scores: item.scores, Evidence: evidence, Facts: facts, Tags: item.tags, FreshnessDays: item.fresh,
			Description: item.description, Accent: item.accent,
			EvidenceSignals: len(facts) + len(item.scores),
		})
	}
	return out
}
