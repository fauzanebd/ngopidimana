package catalogue

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/fauzanebd/wheretowfc/backend/internal/googleplaces"
	"github.com/fauzanebd/wheretowfc/backend/internal/ingestion"
	"github.com/fauzanebd/wheretowfc/backend/internal/recommendation"
)

type Store struct{ db *sql.DB }

func NewStore(db *sql.DB) *Store { return &Store{db: db} }

func (store *Store) Remove(ctx context.Context, run ingestion.Run) error {
	if strings.TrimSpace(run.GooglePlaceID) == "" {
		return nil
	}
	_, err := store.db.ExecContext(ctx, `DELETE FROM places WHERE google_place_id = $1`, run.GooglePlaceID)
	return err
}

func (store *Store) Publish(ctx context.Context, run ingestion.Run) error {
	data, err := publicationData(run)
	if err != nil {
		return err
	}
	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var placeID string
	err = tx.QueryRowContext(ctx, `
		INSERT INTO places (name, slug, status, google_place_id, photo_name, photo_attribution_name, photo_attribution_uri, photo_refreshed_at,
			photo_override_url, photo_override_attribution, updated_at)
		VALUES ($1, $2, 'published', NULLIF($3, ''), $4, $5, $6, CASE WHEN $4 = '' THEN NULL ELSE now() END, $7, $8, now())
		ON CONFLICT (google_place_id) WHERE google_place_id IS NOT NULL
		DO UPDATE SET name = EXCLUDED.name, status = 'published', updated_at = now(),
			-- A republish that captured no photo must not discard a reference that
			-- still works; it is only replaced when the new crawl found one.
			photo_name = COALESCE(NULLIF(EXCLUDED.photo_name, ''), places.photo_name),
			photo_attribution_name = CASE WHEN EXCLUDED.photo_name = '' THEN places.photo_attribution_name ELSE EXCLUDED.photo_attribution_name END,
			photo_attribution_uri = CASE WHEN EXCLUDED.photo_name = '' THEN places.photo_attribution_uri ELSE EXCLUDED.photo_attribution_uri END,
			photo_refreshed_at = COALESCE(EXCLUDED.photo_refreshed_at, places.photo_refreshed_at),
			-- The admin's own cover photo only changes when a run actually carries an
			-- override, so re-ingesting a place cannot silently drop it.
			photo_override_url = CASE WHEN $9 THEN EXCLUDED.photo_override_url ELSE places.photo_override_url END,
			photo_override_attribution = CASE WHEN $9 THEN EXCLUDED.photo_override_attribution ELSE places.photo_override_attribution END
		RETURNING id::text`, data.name, publicationSlug(data.name, run.GooglePlaceID), run.GooglePlaceID,
		data.photo.Name, data.photo.AttributionName, data.photo.AttributionURI,
		data.photoOverride.URL, data.photoOverride.Attribution, run.PhotoOverride != nil).Scan(&placeID)
	if err != nil {
		return fmt.Errorf("upsert place: %w", err)
	}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO place_locations (place_id, address, area, geog)
		VALUES ($1::uuid, $2, $3, ST_SetSRID(ST_MakePoint($4, $5), 4326)::geography)
		ON CONFLICT (place_id) DO UPDATE SET address = EXCLUDED.address, area = EXCLUDED.area, geog = EXCLUDED.geog`,
		placeID, data.address, data.area, data.lng, data.lat)
	if err != nil {
		return fmt.Errorf("save location: %w", err)
	}
	for _, table := range []string{"place_facts", "place_scores", "place_sources", "external_links", "menu_items"} {
		if _, err := tx.ExecContext(ctx, "DELETE FROM "+table+" WHERE place_id = $1::uuid", placeID); err != nil {
			return fmt.Errorf("replace %s: %w", table, err)
		}
	}

	for index, field := range run.Fields {
		captured := parseTime(field.CapturedAt, run.UpdatedAt)
		sourceURL := strings.TrimSpace(field.SourceURL)
		if sourceURL == "" {
			sourceURL = fmt.Sprintf("manual://admin/%s/%d", run.ID, index)
		}
		payload, _ := json.Marshal(map[string]any{
			"value": field.Value, "method": field.Method, "extractor": field.Extractor,
			"author_name": field.AuthorName, "author_url": field.AuthorURL,
		})
		var sourceID string
		err := tx.QueryRowContext(ctx, `INSERT INTO place_sources (place_id, source_type, url, captured_at, payload_json)
			VALUES ($1::uuid, $2, $3, $4, $5::jsonb) RETURNING id::text`, placeID, sourceType(field), sourceURL, captured, payload).Scan(&sourceID)
		if err != nil {
			return fmt.Errorf("save evidence source: %w", err)
		}
		excerpt := strings.TrimSpace(field.Excerpt)
		if excerpt == "" {
			excerpt = field.Value
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO evidence_items (place_id, source_id, field_key, excerpt_or_region, confidence, conflict)
			VALUES ($1::uuid, $2::uuid, $3, $4, $5, $6)`, placeID, sourceID, field.Key, excerpt, clamp(field.Confidence), field.Conflict); err != nil {
			return fmt.Errorf("save evidence item: %w", err)
		}
	}

	for _, fact := range data.facts {
		if _, err := tx.ExecContext(ctx, `INSERT INTO place_facts
			(place_id, key, bool_value, numeric_value, text_value, confidence, observed_at, verification_method)
			VALUES ($1::uuid, $2, $3, $4, $5, $6, $7, $8)`, placeID, fact.key, fact.boolValue, fact.numericValue, fact.textValue, clamp(fact.confidence), fact.observedAt, fact.method); err != nil {
			return fmt.Errorf("save fact %s: %w", fact.key, err)
		}
	}
	for dimension, score := range data.scores {
		if _, err := tx.ExecContext(ctx, `INSERT INTO place_scores (place_id, dimension, score, confidence, model_version)
			VALUES ($1::uuid, $2, $3, $4, 'catalogue-rules-v1')`, placeID, dimension, clamp(score.value), clamp(score.confidence)); err != nil {
			return fmt.Errorf("save score %s: %w", dimension, err)
		}
	}
	for platform, link := range data.links {
		if _, err := tx.ExecContext(ctx, `INSERT INTO external_links (place_id, platform, url, verified_at)
			VALUES ($1::uuid, $2, $3, now()) ON CONFLICT DO NOTHING`, placeID, platform, link); err != nil {
			return fmt.Errorf("save external link: %w", err)
		}
	}
	for _, item := range data.menuItems {
		attributes, _ := json.Marshal(item.attributes)
		if _, err := tx.ExecContext(ctx, `INSERT INTO menu_items
			(place_id, source_label, normalized_name, category, attributes_json, price, currency)
			VALUES ($1::uuid, $2, $3, $4, $5::jsonb, $6, 'IDR')`, placeID, item.sourceLabel, item.name, item.category, attributes, item.price); err != nil {
			return fmt.Errorf("save menu item: %w", err)
		}
	}
	return tx.Commit()
}

type fact struct {
	key          string
	boolValue    *bool
	numericValue *float64
	textValue    *string
	confidence   float64
	observedAt   time.Time
	method       string
}

type score struct{ value, confidence float64 }
type menuItem struct {
	sourceLabel, name, category string
	attributes                  []string
	price                       *float64
}
type publication struct {
	name, area, address string
	lat, lng            float64
	photo               googleplaces.PhotoRef
	photoOverride       ingestion.PhotoOverride
	facts               []fact
	scores              map[string]score
	links               map[string]string
	menuItems           []menuItem
}

func publicationData(run ingestion.Run) (publication, error) {
	data := publication{name: strings.TrimSpace(run.Name), area: strings.TrimSpace(run.Area), scores: map[string]score{}, links: map[string]string{}}
	if run.Photo != nil {
		data.photo = *run.Photo
	}
	if run.PhotoOverride != nil {
		data.photoOverride = *run.PhotoOverride
	}
	best := map[string]ingestion.EvidenceField{}
	for _, field := range run.Fields {
		if field.Conflict || field.Excluded || strings.TrimSpace(field.Value) == "" {
			continue
		}
		if current, ok := best[field.Key]; !ok || field.Confidence > current.Confidence {
			best[field.Key] = field
		}
		if field.Key == "Menu item" {
			data.menuItems = append(data.menuItems, parseMenuItem(field.Value))
		}
	}
	if identity := best["Identity"].Value; data.name == "" || data.name == "New café candidate" {
		data.name = identity
	}
	if data.area == "" {
		data.area = best["Area"].Value
	}
	data.address = best["Address"].Value
	data.lat, _ = strconv.ParseFloat(best["Latitude"].Value, 64)
	data.lng, _ = strconv.ParseFloat(best["Longitude"].Value, 64)
	missing := []string{}
	if data.name == "" {
		missing = append(missing, "identity")
	}
	if strings.TrimSpace(run.GooglePlaceID) == "" {
		missing = append(missing, "stable Google Place ID")
	}
	if data.area == "" {
		missing = append(missing, "area")
	}
	if data.address == "" {
		missing = append(missing, "address")
	}
	if data.lat == 0 || data.lng == 0 {
		missing = append(missing, "coordinates")
	}
	if len(missing) > 0 {
		return publication{}, fmt.Errorf("required evidence missing: %s; re-run extraction before publishing", strings.Join(missing, ", "))
	}

	boolKeys := map[string]string{
		"Musholla": "musholla", "Toilet": "toilet", "Wi-Fi": "wifi", "Power outlets": "outlets",
		"Car parking": "parking", "Motorcycle parking": "motorcycle_parking", "Accessibility": "accessible",
		"Air conditioning": "ac", "Indoor seating": "indoor", "Outdoor seating": "outdoor",
		"Smoking zone": "smoking_zone", "Open 24 hours": "open_24h", "Pet policy": "pet_friendly",
	}
	textKeys := map[string]string{
		"Opening hours": "opening_hours", "Wi-Fi quality": "wifi_quality", "Quietness": "quietness",
		"Crowd level": "crowd_level", "Table size": "table_size", "Seating comfort": "seating_comfort",
		"Laptop policy": "laptop_policy", "Long-stay friendliness": "long_stay", "Ambience": "ambience",
		"Other note": "other_note", "Manual caveat": "caveat", "Parking notes": "parking_notes",
	}
	for key, field := range best {
		observed := parseTime(field.CapturedAt, run.UpdatedAt)
		if factKey, ok := boolKeys[key]; ok {
			value, parsed := parseBool(field.Value)
			if parsed {
				data.facts = append(data.facts, fact{key: factKey, boolValue: &value, confidence: field.Confidence, observedAt: observed, method: first(field.Method, "evidence")})
			}
		}
		if factKey, ok := textKeys[key]; ok {
			value := field.Value
			data.facts = append(data.facts, fact{key: factKey, textValue: &value, confidence: field.Confidence, observedAt: observed, method: first(field.Method, "evidence")})
		}
		if key == "Minimum price" || key == "Maximum price" || key == "Typical spend minimum" || key == "Typical spend maximum" {
			if value, ok := parsePrice(field.Value); ok {
				factKey := "price_min"
				if key == "Maximum price" {
					factKey = "price_max"
				} else if key == "Typical spend minimum" {
					factKey = "typical_spend_min"
				} else if key == "Typical spend maximum" {
					factKey = "typical_spend_max"
				}
				number := float64(value)
				data.facts = append(data.facts, fact{key: factKey, numericValue: &number, confidence: field.Confidence, observedAt: observed, method: first(field.Method, "evidence")})
			}
		}
		switch key {
		case "Google Maps":
			data.links["google_maps"] = field.Value
		case "Website":
			data.links["website"] = field.Value
		case "Instagram":
			data.links["instagram"] = field.Value
		case "TikTok":
			data.links["tiktok"] = field.Value
		}
		if strings.Contains(strings.ToLower(field.Source), "google maps review") && field.SourceURL != "" {
			data.links["google_review"] = field.SourceURL
		}
	}
	if _, ok := data.links["google_maps"]; !ok && strings.Contains(run.URL, "google") {
		data.links["google_maps"] = run.URL
	}
	data.scores = deriveScores(best, data.facts)
	return data, nil
}

func deriveScores(fields map[string]ingestion.EvidenceField, facts []fact) map[string]score {
	out := map[string]score{}
	bools := map[string]bool{}
	for _, item := range facts {
		if item.boolValue != nil {
			bools[item.key] = *item.boolValue
		}
	}
	quiet := signalScore(fields["Quietness"].Value, []string{"quiet", "tenang", "kondusif", "hening"}, []string{"noisy", "berisik", "ramai"})
	wifi := signalScore(fields["Wi-Fi quality"].Value, []string{"fast", "stable", "cepat", "stabil"}, []string{"difficult", "slow", "susah", "lambat"})
	seating := signalScore(fields["Seating comfort"].Value+" "+fields["Table size"].Value, []string{"comfortable", "nyaman", "spacious", "besar"}, []string{"uncomfortable", "sempit", "kecil"})
	out["quiet"] = score{quiet, confidenceOf(fields["Quietness"], .55)}
	if bools["wifi"] && wifi < .65 {
		wifi = .65
	}
	if !bools["wifi"] && wifi == .5 {
		wifi = .35
	}
	wfc := (quiet + wifi + seating) / 3
	if bools["outlets"] {
		wfc = (wfc*3 + 1) / 4
	}
	out["wfc"] = score{wfc, averageConfidence(fields, "Quietness", "Wi-Fi quality", "Seating comfort", "Power outlets")}
	if bools["open_24h"] {
		out["open_24h"] = score{1, .95}
		out["late"] = score{1, .95}
	}
	if hours := strings.ToLower(fields["Opening hours"].Value); strings.Contains(hours, "23:") || strings.Contains(hours, "00:") {
		out["late"] = score{.9, fields["Opening hours"].Confidence}
	}
	return out
}

func (store *Store) ListPublished(ctx context.Context) ([]recommendation.Cafe, error) {
	rows, err := store.db.QueryContext(ctx, `SELECT p.id::text, p.name, l.area, l.address,
		ST_Y(l.geog::geometry), ST_X(l.geog::geometry), p.updated_at, coalesce(p.google_place_id, '')
		FROM places p JOIN place_locations l ON l.place_id = p.id WHERE p.status = 'published' ORDER BY p.name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	cafes := []recommendation.Cafe{}
	byID := map[string]*recommendation.Cafe{}
	for rows.Next() {
		var cafe recommendation.Cafe
		var updated time.Time
		if err := rows.Scan(&cafe.ID, &cafe.Name, &cafe.Area, &cafe.Address, &cafe.Lat, &cafe.Lng, &updated, &cafe.GooglePlaceID); err != nil {
			return nil, err
		}
		cafe.Scores, cafe.Evidence, cafe.Facts = map[string]float64{}, map[string]float64{}, map[string]bool{}
		cafe.Accent = accentFor(cafe.ID)
		cafe.FreshnessDays = max(0, int(time.Since(updated).Hours()/24))
		cafes = append(cafes, cafe)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for index := range cafes {
		byID[cafes[index].ID] = &cafes[index]
	}
	if len(cafes) == 0 {
		return cafes, nil
	}

	if err := store.loadFacts(ctx, byID); err != nil {
		return nil, err
	}
	if err := store.loadScores(ctx, byID); err != nil {
		return nil, err
	}
	if err := store.loadLinks(ctx, byID); err != nil {
		return nil, err
	}
	if err := store.loadMenus(ctx, byID); err != nil {
		return nil, err
	}
	for index := range cafes {
		cafe := &cafes[index]
		if cafe.Description == "" {
			cafe.Description = "Published café evidence reviewed by the Where to WFC catalogue admin."
		}
		cafe.EvidenceSignals = len(cafe.Evidence)
	}
	return cafes, nil
}

func (store *Store) loadFacts(ctx context.Context, cafes map[string]*recommendation.Cafe) error {
	rows, err := store.db.QueryContext(ctx, `SELECT place_id::text, key, bool_value, numeric_value::float8, text_value, confidence::float8 FROM place_facts`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id, key string
		var boolean sql.NullBool
		var number sql.NullFloat64
		var text sql.NullString
		var confidence float64
		if err := rows.Scan(&id, &key, &boolean, &number, &text, &confidence); err != nil {
			return err
		}
		cafe := cafes[id]
		if cafe == nil {
			continue
		}
		cafe.Evidence[key] = confidence
		if boolean.Valid {
			cafe.Facts[key] = boolean.Bool
		}
		if number.Valid && key == "price_min" {
			cafe.PriceMin = int(number.Float64)
		}
		if number.Valid && key == "price_max" {
			cafe.PriceMax = int(number.Float64)
		}
		if text.Valid {
			applyTextFact(cafe, key, text.String)
		}
	}
	return rows.Err()
}

func (store *Store) loadScores(ctx context.Context, cafes map[string]*recommendation.Cafe) error {
	rows, err := store.db.QueryContext(ctx, `SELECT place_id::text, dimension, score::float8, confidence::float8 FROM place_scores`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id, dimension string
		var value, confidence float64
		if err := rows.Scan(&id, &dimension, &value, &confidence); err != nil {
			return err
		}
		if cafe := cafes[id]; cafe != nil {
			cafe.Scores[dimension], cafe.Evidence[dimension] = value, confidence
		}
	}
	return rows.Err()
}

func (store *Store) loadLinks(ctx context.Context, cafes map[string]*recommendation.Cafe) error {
	rows, err := store.db.QueryContext(ctx, `SELECT place_id::text, platform, url FROM external_links`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id, platform, url string
		if err := rows.Scan(&id, &platform, &url); err != nil {
			return err
		}
		if cafe := cafes[id]; cafe != nil {
			if platform == "google_maps" {
				cafe.MapsURL = url
			}
			if platform == "google_review" {
				cafe.ReviewURL = url
			}
		}
	}
	return rows.Err()
}

func (store *Store) loadMenus(ctx context.Context, cafes map[string]*recommendation.Cafe) error {
	rows, err := store.db.QueryContext(ctx, `SELECT place_id::text, normalized_name FROM menu_items`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id, name string
		if err := rows.Scan(&id, &name); err != nil {
			return err
		}
		if cafe := cafes[id]; cafe != nil {
			cafe.Tags = append(cafe.Tags, name)
			if strings.Contains(strings.ToLower(name), "matcha") {
				cafe.Facts["matcha"] = true
				cafe.Evidence["matcha"] = .8
			}
		}
	}
	return rows.Err()
}

func applyTextFact(cafe *recommendation.Cafe, key, value string) {
	switch key {
	case "opening_hours":
		cafe.OpenUntil = latestClosingTime(value)
	case "quietness":
		cafe.Tags = append(cafe.Tags, value)
		if cafe.Description == "" {
			cafe.Description = value
		}
	case "seating_comfort", "table_size", "ambience":
		cafe.Tags = append(cafe.Tags, value)
		if cafe.Description == "" {
			cafe.Description = value
		}
	case "other_note":
		cafe.Tags = append(cafe.Tags, value)
	case "caveat", "wifi_quality", "parking_notes", "laptop_policy", "long_stay":
		cafe.Caveats = append(cafe.Caveats, value)
	}
}

var timePattern = regexp.MustCompile(`(?:[01]?\d|2[0-3])[:.]\d{2}`)
var digitsPattern = regexp.MustCompile(`\d+`)

func latestClosingTime(value string) string {
	matches := timePattern.FindAllString(value, -1)
	if len(matches) == 0 {
		return ""
	}
	sort.Strings(matches)
	return strings.ReplaceAll(matches[len(matches)-1], ".", ":")
}
func parsePrice(value string) (int, bool) {
	digits := strings.Join(digitsPattern.FindAllString(value, -1), "")
	parsed, err := strconv.Atoi(digits)
	return parsed, err == nil
}
func parseBool(value string) (bool, bool) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "yes", "true", "available", "ada", "tersedia", "allowed":
		return true, true
	case "no", "false", "not available", "tidak ada", "tidak tersedia", "not allowed":
		return false, true
	}
	return false, false
}
func parseTime(value string, fallback time.Time) time.Time {
	if parsed, err := time.Parse(time.RFC3339, value); err == nil {
		return parsed
	}
	if fallback.IsZero() {
		return time.Now().UTC()
	}
	return fallback
}
func sourceType(field ingestion.EvidenceField) string {
	if field.Method == "manual" {
		return "manual"
	}
	if strings.Contains(strings.ToLower(field.Source), "google") {
		return "google_maps"
	}
	return first(field.Method, "web")
}
func first(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
func clamp(value float64) float64 {
	if value < 0 {
		return 0
	}
	if value > 1 {
		return 1
	}
	return value
}
func confidenceOf(field ingestion.EvidenceField, fallback float64) float64 {
	if field.Confidence > 0 {
		return field.Confidence
	}
	return fallback
}
func averageConfidence(fields map[string]ingestion.EvidenceField, keys ...string) float64 {
	total, count := 0.0, 0.0
	for _, key := range keys {
		if value := fields[key].Confidence; value > 0 {
			total += value
			count++
		}
	}
	if count == 0 {
		return .55
	}
	return total / count
}
func signalScore(value string, positives, negatives []string) float64 {
	normalized := strings.ToLower(value)
	for _, phrase := range negatives {
		if strings.Contains(normalized, phrase) {
			return .25
		}
	}
	for _, phrase := range positives {
		if strings.Contains(normalized, phrase) {
			return .9
		}
	}
	return .5
}
func publicationSlug(name, identity string) string {
	base := slugify(name)
	hash := fnv.New32a()
	_, _ = hash.Write([]byte(first(identity, name)))
	return fmt.Sprintf("%s-%06x", base, hash.Sum32()&0xffffff)
}
func slugify(value string) string {
	var out strings.Builder
	dash := false
	for _, r := range strings.ToLower(value) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			out.WriteRune(r)
			dash = false
		} else if out.Len() > 0 && !dash {
			out.WriteByte('-')
			dash = true
		}
	}
	return strings.Trim(out.String(), "-")
}
func accentFor(id string) string {
	accents := []string{"sage", "sky", "lavender", "butter", "peach", "rose"}
	hash := fnv.New32a()
	_, _ = hash.Write([]byte(id))
	return accents[int(hash.Sum32())%len(accents)]
}
func parseMenuItem(value string) menuItem {
	parts := strings.Split(value, " · ")
	item := menuItem{sourceLabel: value, name: strings.TrimSpace(parts[0]), category: "other", attributes: []string{}}
	for _, part := range parts[1:] {
		lower := strings.ToLower(part)
		if strings.HasPrefix(lower, "category:") {
			item.category = strings.TrimSpace(strings.TrimPrefix(part, "category:"))
		} else if price, ok := parsePrice(part); ok && strings.Contains(lower, "rp") {
			amount := float64(price)
			item.price = &amount
		} else {
			item.attributes = append(item.attributes, part)
		}
	}
	return item
}

var _ ingestion.Publisher = (*Store)(nil)
var _ recommendation.Catalogue = (*Store)(nil)

// PhotoRef returns the photo reference captured for a published place, if any.
//
// It is deliberately a *hint*: Google's photo names expire, so a stored name is
// worth exactly one cheap media attempt before the caller has to re-resolve it.
func (store *Store) PhotoRef(ctx context.Context, googlePlaceID string) (googleplaces.PhotoRef, bool, error) {
	googlePlaceID = strings.TrimSpace(googlePlaceID)
	if googlePlaceID == "" {
		return googleplaces.PhotoRef{}, false, nil
	}
	var reference googleplaces.PhotoRef
	err := store.db.QueryRowContext(ctx, `
		SELECT photo_name, photo_attribution_name, photo_attribution_uri
		FROM places WHERE google_place_id = $1 AND photo_name <> ''`, googlePlaceID).
		Scan(&reference.Name, &reference.AttributionName, &reference.AttributionURI)
	if errors.Is(err, sql.ErrNoRows) {
		return googleplaces.PhotoRef{}, false, nil
	}
	if err != nil {
		return googleplaces.PhotoRef{}, false, fmt.Errorf("read photo reference: %w", err)
	}
	return reference, true, nil
}

// SavePhotoRef records a reference that Google accepted, so the next render skips
// the Place Details lookup.
func (store *Store) SavePhotoRef(ctx context.Context, googlePlaceID string, reference googleplaces.PhotoRef) error {
	googlePlaceID = strings.TrimSpace(googlePlaceID)
	if googlePlaceID == "" || strings.TrimSpace(reference.Name) == "" {
		return nil
	}
	_, err := store.db.ExecContext(ctx, `
		UPDATE places SET photo_name = $2, photo_attribution_name = $3, photo_attribution_uri = $4, photo_refreshed_at = now()
		WHERE google_place_id = $1`, googlePlaceID, reference.Name, reference.AttributionName, reference.AttributionURI)
	if err != nil {
		return fmt.Errorf("save photo reference: %w", err)
	}
	return nil
}

// PhotoOverride returns the admin-attached cover photo for a place, if one is set.
// It takes precedence over Google's photo: nothing is billed and nothing expires.
func (store *Store) PhotoOverride(ctx context.Context, googlePlaceID string) (ingestion.PhotoOverride, bool, error) {
	googlePlaceID = strings.TrimSpace(googlePlaceID)
	if googlePlaceID == "" {
		return ingestion.PhotoOverride{}, false, nil
	}
	var override ingestion.PhotoOverride
	err := store.db.QueryRowContext(ctx, `
		SELECT photo_override_url, photo_override_attribution
		FROM places WHERE google_place_id = $1 AND photo_override_url <> ''`, googlePlaceID).
		Scan(&override.URL, &override.Attribution)
	if errors.Is(err, sql.ErrNoRows) {
		return ingestion.PhotoOverride{}, false, nil
	}
	if err != nil {
		return ingestion.PhotoOverride{}, false, fmt.Errorf("read photo override: %w", err)
	}
	return override, true, nil
}
