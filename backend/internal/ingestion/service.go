package ingestion

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"
)

var ErrConflicts = errors.New("resolve conflicting evidence before publishing")
var ErrInvalidAction = errors.New("action must be review, publish, refresh, or archive")
var ErrInvalidManualEvidence = errors.New("invalid manual evidence")
var ErrInvalidEvidence = errors.New("invalid evidence update")
var ErrInvalidPhotoOverride = errors.New("a cover photo needs an absolute http(s) URL, at most 800 characters")

type Publisher interface {
	Publish(context.Context, Run) error
	Remove(context.Context, Run) error
}

var manualEvidenceKeys = map[string]bool{
	"Other note": true, "Manual caveat": true, "Seating comfort": true, "Table size": true, "Wi-Fi quality": true,
	"Power outlets": true, "Quietness": true, "Crowd level": true, "Laptop policy": true,
	"Long-stay friendliness": true, "Opening hours": true, "Minimum price": true,
	"Maximum price": true, "Accessibility": true, "Parking notes": true, "Ambience": true,
}

type Service struct {
	store     Store
	queue     Queue
	publisher Publisher
}

func NewService(store Store, queue Queue, publishers ...Publisher) *Service {
	service := &Service{store: store, queue: queue}
	if len(publishers) > 0 {
		service.publisher = publishers[0]
	}
	return service
}

func (s *Service) List(ctx context.Context) ([]Run, error) {
	runs, err := s.store.List(ctx)
	if err != nil {
		return nil, err
	}
	for index := range runs {
		runs[index] = normalizeRunEvidence(runs[index])
	}
	return runs, nil
}

func (s *Service) Get(ctx context.Context, id string) (Run, error) {
	run, err := s.store.Get(ctx, id)
	if err != nil {
		return Run{}, err
	}
	return normalizeRunEvidence(run), nil
}

func (s *Service) Delete(ctx context.Context, id string) error {
	run, err := s.Get(ctx, id)
	if err != nil {
		return err
	}
	if s.publisher != nil && !s.anotherRecordExists(ctx, run) {
		if err := s.publisher.Remove(ctx, run); err != nil {
			return fmt.Errorf("remove catalogue record: %w", err)
		}
	}
	return s.store.Delete(ctx, id)
}

// anotherRecordExists reports whether any other record still represents this run's place.
//
// The catalogue row is keyed on the Google Place ID rather than on the record, so a
// second record for the same place — a duplicate crawl, or a re-ingest after the venue
// changed — would otherwise let deleting the unrelated duplicate unpublish a live café.
// The entry leaves the catalogue with the last record representing it, and not before.
//
// Listing every run answers this in one pass and is nothing at catalogue scale; if the
// run store ever reaches thousands of records, this wants an index of Place ID to run
// rather than a full scan on every delete.
func (s *Service) anotherRecordExists(ctx context.Context, run Run) bool {
	if strings.TrimSpace(run.GooglePlaceID) == "" {
		return false
	}
	runs, err := s.store.List(ctx)
	if err != nil {
		// Keep the place published when the check cannot be answered: skipping an
		// unpublish is recoverable by removing the remaining record, an unrequested
		// unpublish is not.
		return true
	}
	for _, candidate := range runs {
		if candidate.ID != run.ID && candidate.GooglePlaceID == run.GooglePlaceID {
			return true
		}
	}
	return false
}

func (s *Service) AddManualEvidence(ctx context.Context, id string, input ManualEvidenceInput) (Run, error) {
	run, err := s.Get(ctx, id)
	if err != nil {
		return Run{}, err
	}
	key := strings.Join(strings.Fields(input.Key), " ")
	value := strings.Join(strings.Fields(input.Value), " ")
	if !manualEvidenceKeys[key] || value == "" || len([]rune(value)) > 800 {
		return Run{}, ErrInvalidManualEvidence
	}
	if run.State == "enriching" || run.State == "archived" {
		return Run{}, fmt.Errorf("manual evidence cannot be changed while a record is %s", run.State)
	}
	now := time.Now().UTC()
	run.Fields = append(run.Fields, EvidenceField{
		ID: newManualEvidenceID(), Key: key, Value: value, Source: "Manually added", CapturedAt: now.Format(time.RFC3339),
		Method: "manual", Extractor: "admin", Confidence: .9,
	})
	return s.saveReviewedEvidence(ctx, run)
}

// SetPhotoOverride attaches an image the project has rights to, which the public
// API serves instead of Google's. An empty URL clears it, and that is recorded as
// an explicit "none" rather than "unspecified".
func (s *Service) SetPhotoOverride(ctx context.Context, id string, input PhotoOverrideInput) (Run, error) {
	run, err := s.Get(ctx, id)
	if err != nil {
		return Run{}, err
	}
	if run.State == "enriching" || run.State == "archived" {
		return Run{}, fmt.Errorf("the cover photo cannot be changed while a record is %s", run.State)
	}
	imageURL := strings.TrimSpace(input.URL)
	attribution := strings.Join(strings.Fields(input.Attribution), " ")
	if imageURL == "" {
		run.PhotoOverride = &PhotoOverride{}
	} else {
		parsed, parseErr := url.Parse(imageURL)
		if parseErr != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") ||
			len([]rune(imageURL)) > 800 || len([]rune(attribution)) > 200 {
			return Run{}, ErrInvalidPhotoOverride
		}
		run.PhotoOverride = &PhotoOverride{URL: imageURL, Attribution: attribution}
	}
	return s.saveReviewedEvidence(ctx, run)
}

func (s *Service) RemoveManualEvidence(ctx context.Context, id, evidenceID string) (Run, error) {
	run, err := s.Get(ctx, id)
	if err != nil {
		return Run{}, err
	}
	removed := false
	fields := make([]EvidenceField, 0, len(run.Fields))
	for _, field := range run.Fields {
		if field.ID == evidenceID && field.Method == "manual" {
			removed = true
			continue
		}
		fields = append(fields, field)
	}
	if !removed {
		return Run{}, ErrNotFound
	}
	run.Fields = fields
	return s.saveReviewedEvidence(ctx, run)
}

func (s *Service) SetEvidenceExcluded(ctx context.Context, id, evidenceID string, excluded bool) (Run, error) {
	run, err := s.Get(ctx, id)
	if err != nil {
		return Run{}, err
	}
	if !evidenceCanBeEdited(run) {
		return Run{}, ErrInvalidEvidence
	}
	found := false
	for index := range run.Fields {
		if run.Fields[index].ID == evidenceID {
			run.Fields[index].Excluded = excluded
			found = true
			break
		}
	}
	if !found {
		return Run{}, ErrNotFound
	}
	return s.saveReviewedEvidence(ctx, run)
}

func (s *Service) ChooseEvidence(ctx context.Context, id, evidenceID string) (Run, error) {
	run, err := s.Get(ctx, id)
	if err != nil {
		return Run{}, err
	}
	if !evidenceCanBeEdited(run) {
		return Run{}, ErrInvalidEvidence
	}
	var chosen *EvidenceField
	for index := range run.Fields {
		if run.Fields[index].ID == evidenceID {
			chosen = &run.Fields[index]
			break
		}
	}
	if chosen == nil || !conflictSensitiveFields[chosen.Key] {
		return Run{}, ErrInvalidEvidence
	}
	chosenValue := canonicalConflictValue(chosen.Key, chosen.Value)
	for index := range run.Fields {
		field := &run.Fields[index]
		if field.Key == chosen.Key {
			field.Excluded = canonicalConflictValue(field.Key, field.Value) != chosenValue
		}
	}
	return s.saveReviewedEvidence(ctx, run)
}

func (s *Service) ReplaceEvidence(ctx context.Context, id, evidenceID, value string) (Run, error) {
	run, err := s.Get(ctx, id)
	if err != nil {
		return Run{}, err
	}
	if !evidenceCanBeEdited(run) {
		return Run{}, ErrInvalidEvidence
	}
	value = strings.Join(strings.Fields(value), " ")
	if value == "" || len([]rune(value)) > 800 {
		return Run{}, ErrInvalidEvidence
	}
	for index := range run.Fields {
		if run.Fields[index].ID != evidenceID {
			continue
		}
		key := run.Fields[index].Key
		if run.Fields[index].Method == "manual" {
			run.Fields[index].Value = value
			run.Fields[index].CapturedAt = time.Now().UTC().Format(time.RFC3339)
		} else {
			for otherIndex := range run.Fields {
				if run.Fields[otherIndex].Key == key {
					run.Fields[otherIndex].Excluded = true
				}
			}
			run.Fields = append(run.Fields, EvidenceField{
				ID: newManualEvidenceID(), Key: key, Value: value, Source: "Manually added",
				CapturedAt: time.Now().UTC().Format(time.RFC3339), Method: "manual", Extractor: "admin_correction", Confidence: .9,
			})
		}
		return s.saveReviewedEvidence(ctx, run)
	}
	return Run{}, ErrNotFound
}

func evidenceCanBeEdited(run Run) bool { return run.State != "enriching" && run.State != "archived" }

func (s *Service) saveReviewedEvidence(ctx context.Context, run Run) (Run, error) {
	run = normalizeRunEvidence(run)
	run.Confidence = evidenceCoverage(run.Fields)
	run.State, run.Stage, run.Progress = "needs_review", "ready_for_review", 100
	run.UpdatedAt = time.Now().UTC()
	if err := s.store.Save(ctx, run); err != nil {
		return Run{}, err
	}
	return run, nil
}

func normalizeRunEvidence(run Run) Run {
	for index := range run.Fields {
		field := &run.Fields[index]
		if field.Key == "Area" {
			field.Value = cleanAreaName(field.Value)
		}
		// Older runs classified generic listing-site spend bands as menu price
		// extrema. Migrate them as records are read so users do not need to ingest
		// the same venue again.
		if genericVenuePriceBand(field.Excerpt) {
			switch field.Key {
			case "Minimum price":
				field.Key = "Typical spend minimum"
			case "Maximum price":
				field.Key = "Typical spend maximum"
			}
		}
		if field.ID == "" {
			sum := sha256.Sum256([]byte(fmt.Sprintf("%d|%s|%s|%s|%s|%s", index, field.Key, field.Value, field.SourceID, field.SourceURL, field.Method)))
			field.ID = "evidence_" + hex.EncodeToString(sum[:8])
		}
	}
	record := detectEvidenceConflicts(ExtractedRecord{Name: run.Name, Area: run.Area, Issues: append([]string{}, run.Issues...), Fields: run.Fields})
	record = chooseRecordIdentity(record)
	run.Name, run.Area, run.Issues, run.Fields = record.Name, record.Area, record.Issues, record.Fields
	run.Confidence = evidenceCoverage(run.Fields)
	return run
}

// ErrDuplicateURL reports that a record already exists for the submitted URL.
//
// It carries the existing record because the caller's useful response is that record,
// not the failure: the admin offers to open it instead of queueing a second crawl.
type ErrDuplicateURL struct{ Existing Run }

func (err ErrDuplicateURL) Error() string {
	return fmt.Sprintf("a record for %s already exists", err.Existing.URL)
}

func (s *Service) Create(ctx context.Context, rawURL string, force bool) (Run, error) {
	parsed, err := parseSourceURL(rawURL)
	if err != nil {
		return Run{}, err
	}
	// The client checks this too, to avoid the round trip, but the client can be stale or
	// out of date in a tab left open across a deploy, so the refusal has to live here.
	if !force {
		existing, found, err := s.findByURL(ctx, parsed.String())
		if err != nil {
			return Run{}, err
		}
		if found {
			return Run{}, ErrDuplicateURL{Existing: existing}
		}
	}
	now := time.Now().UTC()
	run := Run{
		ID: newID(), URL: parsed.String(), Source: ClassifySource(parsed.Host), State: "enriching",
		Stage: "queued", Progress: 5, Name: "New café candidate", Issues: []string{}, Warnings: []string{}, Fields: []EvidenceField{},
		CreatedAt: now, UpdatedAt: now,
	}
	if err := s.store.Save(ctx, run); err != nil {
		return Run{}, err
	}
	if err := s.queue.Enqueue(ctx, run.ID, run.URL); err != nil {
		run.State, run.Stage, run.Progress = "failed", "queue_failed", 0
		run.Issues = []string{err.Error()}
		run.UpdatedAt = time.Now().UTC()
		_ = s.store.Save(ctx, run)
		return Run{}, err
	}
	return run, nil
}

// findByURL returns the most recent record for an already-canonical URL. A trailing slash is
// folded so that "/cafe" and "/cafe/" — the same page — are treated as the same record; the
// stored URL itself is never rewritten.
func (s *Service) findByURL(ctx context.Context, canonical string) (Run, bool, error) {
	runs, err := s.store.List(ctx)
	if err != nil {
		return Run{}, false, err
	}
	target := strings.TrimSuffix(canonical, "/")
	var newest Run
	found := false
	for _, candidate := range runs {
		if strings.TrimSuffix(candidate.URL, "/") != target {
			continue
		}
		if !found || candidate.CreatedAt.After(newest.CreatedAt) {
			newest, found = candidate, true
		}
	}
	return newest, found, nil
}

func (s *Service) Update(ctx context.Context, id, action string) (Run, error) {
	run, err := s.Get(ctx, id)
	if err != nil {
		return Run{}, err
	}
	switch action {
	case "review":
		run.State, run.Stage, run.Progress = "needs_review", "ready_for_review", 100
	case "publish":
		if len(run.Issues) > 0 {
			return Run{}, ErrConflicts
		}
		if run.State != "needs_review" {
			return Run{}, fmt.Errorf("only reviewed records can be published")
		}
		if s.publisher == nil {
			return Run{}, errors.New("catalogue publisher is not configured")
		}
		if err := s.publisher.Publish(ctx, run); err != nil {
			return Run{}, fmt.Errorf("publish catalogue record: %w", err)
		}
		run.State, run.Stage, run.Progress = "published", "published", 100
	case "refresh":
		run.State, run.Stage, run.Progress = "enriching", "queued", 5
		run.Issues = []string{}
		run.Warnings = []string{}
		run.UpdatedAt = time.Now().UTC()
		if err := s.store.Save(ctx, run); err != nil {
			return Run{}, err
		}
		if err := s.queue.Enqueue(ctx, run.ID, run.URL); err != nil {
			run.State, run.Stage, run.Progress = "failed", "queue_failed", 0
			run.Issues = []string{err.Error()}
			run.UpdatedAt = time.Now().UTC()
			_ = s.store.Save(ctx, run)
			return Run{}, err
		}
		return run, nil
	case "archive":
		if s.publisher != nil && !s.anotherRecordExists(ctx, run) {
			if err := s.publisher.Remove(ctx, run); err != nil {
				return Run{}, fmt.Errorf("remove catalogue record: %w", err)
			}
		}
		run.State, run.Stage = "archived", "archived"
	default:
		return Run{}, ErrInvalidAction
	}
	run.UpdatedAt = time.Now().UTC()
	if err := s.store.Save(ctx, run); err != nil {
		return Run{}, err
	}
	return run, nil
}

func parseSourceURL(raw string) (*url.URL, error) {
	parsed, err := url.ParseRequestURI(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return nil, errors.New("a valid http or https café URL is required")
	}
	if parsed.User != nil {
		return nil, errors.New("source URLs must not contain credentials")
	}
	return parsed, nil
}

func ClassifySource(host string) string {
	host = strings.ToLower(host)
	switch {
	case strings.Contains(host, "google") || strings.Contains(host, "goo.gl"):
		return "google_maps"
	case strings.Contains(host, "instagram"):
		return "instagram"
	case strings.Contains(host, "tiktok"):
		return "tiktok"
	default:
		return "website"
	}
}

func newID() string {
	buffer := make([]byte, 12)
	if _, err := rand.Read(buffer); err != nil {
		return fmt.Sprintf("ing_%d", time.Now().UnixNano())
	}
	return "ing_" + hex.EncodeToString(buffer)
}

func newManualEvidenceID() string {
	buffer := make([]byte, 8)
	if _, err := rand.Read(buffer); err != nil {
		return fmt.Sprintf("manual_%d", time.Now().UnixNano())
	}
	return "manual_" + hex.EncodeToString(buffer)
}
