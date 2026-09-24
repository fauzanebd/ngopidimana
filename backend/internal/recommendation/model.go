package recommendation

type Point struct {
	Lat float64 `json:"lat"`
	Lng float64 `json:"lng"`
}

type Request struct {
	Query        string `json:"query"`
	UserLocation *Point `json:"user_location,omitempty"`
	Limit        int    `json:"limit"`
	Locale       string `json:"locale,omitempty"`
}

type Requirement struct {
	Key        string  `json:"key"`
	Label      string  `json:"label"`
	Kind       string  `json:"kind"`
	Weight     float64 `json:"weight"`
	Confidence float64 `json:"confidence"`
}

type Interpretation struct {
	HardConstraints []Requirement `json:"hard_constraints"`
	SoftPreferences []Requirement `json:"soft_preferences"`
	Location        string        `json:"location"`
	LocationMode    string        `json:"location_mode"`
	Budget          int           `json:"budget,omitempty"`
	Summary         string        `json:"summary"`
	Confidence      float64       `json:"confidence"`
	Provider        string        `json:"provider"`
	Model           string        `json:"model,omitempty"`
	LatencyMS       int64         `json:"latency_ms"`
	FallbackReason  string        `json:"fallback_reason,omitempty"`
}

type Cafe struct {
	ID              string
	GooglePlaceID   string
	Name            string
	Area            string
	Address         string
	Lat             float64
	Lng             float64
	PriceMin        int
	PriceMax        int
	OpenUntil       string
	Scores          map[string]float64
	Evidence        map[string]float64
	Facts           map[string]bool
	Tags            []string
	ReviewURL       string
	MapsURL         string
	Caveats         []string
	FreshnessDays   int
	Description     string
	Accent          string
	EvidenceSignals int
}

type ReviewLink struct {
	Platform string `json:"platform"`
	URL      string `json:"url"`
}

type Recommendation struct {
	PlaceID          string             `json:"place_id"`
	GooglePlaceID    string             `json:"google_place_id,omitempty"`
	Name             string             `json:"name"`
	Area             string             `json:"area"`
	Address          string             `json:"address"`
	MatchScore       float64            `json:"match_score"`
	ResultConfidence float64            `json:"result_confidence"`
	MatchedOn        []string           `json:"matched_on"`
	Caveats          []string           `json:"caveats"`
	MapsURL          string             `json:"maps_url"`
	ReviewLinks      []ReviewLink       `json:"review_links"`
	PriceMin         int                `json:"price_min"`
	PriceMax         int                `json:"price_max"`
	DistanceKM       float64            `json:"distance_km"`
	OpenUntil        string             `json:"open_until"`
	Open24Hours      bool               `json:"open_24_hours"`
	FreshnessDays    int                `json:"freshness_days"`
	Description      string             `json:"description"`
	Accent           string             `json:"accent"`
	EvidenceSummary  string             `json:"evidence_summary"`
	ScoreComponents  map[string]float64 `json:"score_components"`
}

type Response struct {
	Results        []Recommendation `json:"results"`
	Interpretation Interpretation   `json:"interpretation"`
	RequestID      string           `json:"request_id"`
	ElapsedMS      int64            `json:"elapsed_ms"`
	CatalogueSize  int              `json:"catalogue_size"`
}
