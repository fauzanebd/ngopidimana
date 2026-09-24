package recommendation

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync/atomic"
	"time"
)

type Catalogue interface {
	ListPublished(context.Context) ([]Cafe, error)
}

type staticCatalogue struct{ cafes []Cafe }

func (catalogue staticCatalogue) ListPublished(context.Context) ([]Cafe, error) {
	return catalogue.cafes, nil
}

type Service struct {
	interpreter Interpreter
	catalogue   Catalogue
	size        atomic.Int64
}

func NewService(interpreter Interpreter, catalogues ...Catalogue) *Service {
	catalogue := Catalogue(staticCatalogue{cafes: SeedCafes()})
	if len(catalogues) > 0 && catalogues[0] != nil {
		catalogue = catalogues[0]
	}
	service := &Service{catalogue: catalogue, interpreter: interpreter}
	if initial, err := catalogue.ListPublished(context.Background()); err == nil {
		service.size.Store(int64(len(initial)))
	}
	return service
}

func (s *Service) CatalogueSize() int { return int(s.size.Load()) }

func (s *Service) Recommend(ctx context.Context, request Request) Response {
	started := time.Now()
	locale := NormalizeLocale(request.Locale)
	profile, err := s.interpreter.Interpret(ctx, request.Query)
	if err != nil {
		profile = InterpretDeterministically(request.Query)
		profile.FallbackReason = catalogFor(locale).fallbackReason
	}
	profile = enforceDeterministicConstraints(request.Query, profile)
	profile = enforceLocationIntent(request.Query, profile)
	profile = applyDefaultUserLocation(profile, request.UserLocation)
	profile = normalizeInterpretationCollections(profile)
	profile = localizeInterpretation(profile, locale)
	cafes, catalogueErr := s.catalogue.ListPublished(ctx)
	if catalogueErr != nil {
		cafes = []Cafe{}
	}
	s.size.Store(int64(len(cafes)))
	results := RankCafes(cafes, profile, request.UserLocation, locale)
	if len(results) > request.Limit {
		results = results[:request.Limit]
	}
	return Response{
		Results: results, Interpretation: profile, RequestID: fmt.Sprintf("rec_%d", time.Now().UnixNano()),
		ElapsedMS: time.Since(started).Milliseconds(), CatalogueSize: len(cafes),
	}
}

func normalizeInterpretationCollections(profile Interpretation) Interpretation {
	if profile.HardConstraints == nil {
		profile.HardConstraints = []Requirement{}
	}
	if profile.SoftPreferences == nil {
		profile.SoftPreferences = []Requirement{}
	}
	return profile
}

func RankCafes(cafes []Cafe, profile Interpretation, userLocation *Point, locale Locale) []Recommendation {
	c := catalogFor(locale)
	location := Point{Lat: -6.2088, Lng: 106.8456}
	if userLocation != nil {
		location = *userLocation
	}
	scope, hasScope := scopeForLabel(profile.Location)
	if profile.LocationMode == "named" && hasScope && userLocation == nil {
		location = scope.Center
	}
	requirements := append(append([]Requirement{}, profile.HardConstraints...), profile.SoftPreferences...)
	results := make([]Recommendation, 0, len(cafes))
	for _, cafe := range cafes {
		distance := haversine(location.Lat, location.Lng, cafe.Lat, cafe.Lng)
		if profile.LocationMode == "named" && hasScope && !cafeMatchesScope(cafe, scope) {
			continue
		}
		if profile.LocationMode == "nearby" && (userLocation == nil || distance > nearbyRadiusKM) {
			continue
		}
		eligible := true
		for _, req := range profile.HardConstraints {
			if !cafe.Facts[req.Key] {
				eligible = false
				break
			}
		}
		if !eligible {
			continue
		}

		weighted, weights, evidenceCoverage, facilityMatches := 0.0, 0.0, 0.0, 0.0
		matched := make([]string, 0, 5)
		for _, req := range requirements {
			score := cafe.Scores[req.Key]
			if cafe.Facts[req.Key] {
				score = math.Max(score, 1)
			}
			confidence := cafe.Evidence[req.Key]
			if confidence == 0 {
				confidence = .68
			}
			weighted += req.Weight * score * confidence
			weights += req.Weight
			evidenceCoverage += req.Weight * confidence
			if score >= .66 && len(matched) < 5 {
				matched = append(matched, req.Label)
			}
			if cafe.Facts[req.Key] {
				facilityMatches++
			}
		}
		fuzzyFit, coverage := .62, .7
		if weights > 0 {
			fuzzyFit, coverage = weighted/weights, evidenceCoverage/weights
		}
		facilityFit := clamp(.55+facilityMatches/math.Max(1, float64(len(requirements)))*.45, 0, 1)
		distanceFit := math.Exp(-distance / 9)
		budgetFit := .86
		if profile.Budget > 0 {
			if cafe.PriceMin <= profile.Budget {
				budgetFit = 1 - clamp(float64(cafe.PriceMax-profile.Budget)/float64(max(15000, profile.Budget)), 0, .35)
			} else {
				budgetFit = math.Exp(-float64(cafe.PriceMin-profile.Budget) / 30000)
			}
		}
		freshnessFit := clamp(1-float64(cafe.FreshnessDays)/240, .45, 1)
		raw := .55*fuzzyFit + .15*facilityFit + .15*distanceFit + .10*budgetFit + .05*freshnessFit
		confidence := clamp(coverage*profile.Confidence, .48, .97)
		final := clamp(raw*(.82+.18*confidence), 0, .99)

		caveats := append([]string{}, cafe.Caveats...)
		if cafe.FreshnessDays > 60 {
			caveats = append(caveats, fmt.Sprintf(c.staleCaveat, cafe.FreshnessDays))
		}
		if profile.Budget > 0 && cafe.PriceMax > profile.Budget {
			caveats = append(caveats, c.budgetCaveat)
		}
		if len(matched) == 0 {
			matched = append(matched, c.matchedFallback)
		}
		links := []ReviewLink{}
		if cafe.ReviewURL != "" {
			links = append(links, ReviewLink{Platform: "Review", URL: cafe.ReviewURL})
		}
		mapsURL := cafe.MapsURL
		if mapsURL == "" {
			mapsURL = "https://www.google.com/maps/search/?api=1&query=" + strings.ReplaceAll(cafe.Name+" "+cafe.Area+" Jakarta", " ", "+")
		}
		results = append(results, Recommendation{
			PlaceID: cafe.ID, GooglePlaceID: cafe.GooglePlaceID, Name: cafe.Name, Area: cafe.Area, Address: cafe.Address, MatchScore: round(final),
			ResultConfidence: round(confidence), MatchedOn: matched, Caveats: caveats,
			MapsURL: mapsURL, ReviewLinks: links,
			PriceMin: cafe.PriceMin, PriceMax: cafe.PriceMax, DistanceKM: math.Round(distance*10) / 10,
			OpenUntil: cafe.OpenUntil, Open24Hours: cafe.Facts["open_24h"], FreshnessDays: cafe.FreshnessDays,
			Description: cafe.Description, Accent: cafe.Accent, EvidenceSummary: fmt.Sprintf(c.evidenceSummary, cafe.EvidenceSignals, cafe.FreshnessDays),
			ScoreComponents: map[string]float64{"preference_fit": round(fuzzyFit), "facilities": round(facilityFit), "distance": round(distanceFit), "budget": round(budgetFit), "freshness": round(freshnessFit)},
		})
	}
	sort.SliceStable(results, func(i, j int) bool { return results[i].MatchScore > results[j].MatchScore })
	return results
}

func haversine(lat1, lng1, lat2, lng2 float64) float64 {
	const radius = 6371.0
	toRad := math.Pi / 180
	dLat, dLng := (lat2-lat1)*toRad, (lng2-lng1)*toRad
	a := math.Sin(dLat/2)*math.Sin(dLat/2) + math.Cos(lat1*toRad)*math.Cos(lat2*toRad)*math.Sin(dLng/2)*math.Sin(dLng/2)
	return radius * 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
}

func NormalizeQuery(query string) string { return strings.TrimSpace(query) }
