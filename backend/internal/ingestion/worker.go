package ingestion

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/hibiken/asynq"
)

type Processor struct {
	store             Store
	extractor         Extractor
	placeResolver     PlaceResolver
	discoverer        Discoverer
	evidenceExtractor EvidenceExtractor
}

func NewProcessor(store Store, extractor Extractor, placeResolver PlaceResolver, discoverer Discoverer, evidenceExtractor EvidenceExtractor) *Processor {
	return &Processor{store: store, extractor: extractor, placeResolver: placeResolver, discoverer: discoverer, evidenceExtractor: evidenceExtractor}
}

func (p *Processor) Handle(ctx context.Context, task *asynq.Task) error {
	var payload taskPayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return fmt.Errorf("decode ingestion task: %v: %w", err, asynq.SkipRetry)
	}
	run, err := p.store.Get(ctx, payload.RunID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return fmt.Errorf("ingestion run was deleted: %w", asynq.SkipRetry)
		}
		return fmt.Errorf("load ingestion run: %w", err)
	}
	run.State, run.Stage, run.Progress = "enriching", "fetch_source", 25
	run.UpdatedAt = time.Now().UTC()
	if err := p.save(ctx, run); err != nil {
		return err
	}

	record, err := p.extractor.Extract(ctx, payload.URL)
	if err != nil {
		run.State, run.Stage, run.Progress = "failed", "fetch_failed", 25
		run.Issues = []string{err.Error()}
		run.UpdatedAt = time.Now().UTC()
		if saveErr := p.save(ctx, run); saveErr != nil {
			return saveErr
		}
		return err
	}
	run.Stage, run.Progress = "extract_evidence", 55
	run.Name, run.Area, run.Confidence = record.Name, record.Area, record.Confidence
	run.Issues, run.Fields = record.Issues, record.Fields
	run.UpdatedAt = time.Now().UTC()
	if err := p.save(ctx, run); err != nil {
		return err
	}

	if p.placeResolver != nil && record.Name != "" {
		run.Stage, run.Progress = "resolve_google_place", 62
		run.UpdatedAt = time.Now().UTC()
		if err := p.save(ctx, run); err != nil {
			return err
		}
		placeID, resolveErr := p.placeResolver.ResolvePlaceID(ctx, record.Name, record.Area)
		if resolveErr != nil {
			run.Warnings = append(run.Warnings, "Google Places identity resolution was unavailable: "+resolveErr.Error())
		} else {
			run.GooglePlaceID = placeID
			if provider, ok := p.placeResolver.(placeDetailsProvider); ok {
				run.Stage, run.Progress = "fetch_google_reviews", 66
				place, placeErr := provider.GetPlace(ctx, placeID)
				if placeErr != nil {
					run.Warnings = append(run.Warnings, "Google Places reviews were unavailable: "+placeErr.Error())
				} else {
					record = mergeGooglePlaceEvidence(record, place, time.Now().UTC())
					run.Fields = record.Fields
					run.Photo = place.PrimaryPhoto()
				}
			}
		}
		run.UpdatedAt = time.Now().UTC()
		if err := p.save(ctx, run); err != nil {
			return err
		}
	}

	if p.discoverer != nil {
		run.Stage, run.Progress = "discover_sources", 70
		run.UpdatedAt = time.Now().UTC()
		if err := p.save(ctx, run); err != nil {
			return err
		}
		sources, discoveryErr := p.discoverer.Discover(ctx, DiscoverySeed{Name: record.Name, Area: record.Area, URL: payload.URL})
		if discoveryErr != nil {
			run.Warnings = append(run.Warnings, "Exa source discovery was unavailable: "+discoveryErr.Error())
		} else {
			run.Stage, run.Progress = "merge_evidence", 78
			capturedAt := time.Now().UTC()
			record = mergeDiscoveredSources(record, sources, capturedAt)
			record.Documents = append(record.Documents, discoveredSourceDocuments(sources, capturedAt)...)
			run.Confidence, run.Fields = record.Confidence, record.Fields
		}
		run.UpdatedAt = time.Now().UTC()
		if err := p.save(ctx, run); err != nil {
			return err
		}
	}

	if p.evidenceExtractor != nil {
		run.Stage, run.Progress = "structured_extraction", 82
		run.UpdatedAt = time.Now().UTC()
		if err := p.save(ctx, run); err != nil {
			return err
		}
		extraction, extractionErr := p.evidenceExtractor.ExtractEvidence(ctx, ExtractionInput{
			RunID: run.ID, Name: record.Name, Area: record.Area, SourceURL: payload.URL, Documents: record.Documents,
		})
		if extractionErr != nil {
			run.Warnings = append(run.Warnings, "Structured extraction was unavailable: "+extractionErr.Error())
		} else {
			run.Stage, run.Progress = "normalize_evidence", 94
			var accepted int
			record, accepted = mergeModelExtraction(record, extraction, time.Now().UTC())
			if accepted == 0 {
				run.Warnings = append(run.Warnings, "Structured extraction returned no locally verifiable evidence")
			}
			run.Name, run.Area, run.Confidence = record.Name, record.Area, record.Confidence
			run.Issues, run.Fields = record.Issues, record.Fields
		}
		run.UpdatedAt = time.Now().UTC()
		if err := p.save(ctx, run); err != nil {
			return err
		}
	}

	run.Confidence = evidenceCoverage(record.Fields)
	run.State, run.Stage, run.Progress = "needs_review", "ready_for_review", 100
	run.UpdatedAt = time.Now().UTC()
	return p.save(ctx, run)
}

func (p *Processor) save(ctx context.Context, run Run) error {
	err := p.store.Save(ctx, run)
	if errors.Is(err, ErrNotFound) {
		return fmt.Errorf("ingestion run was deleted: %w", asynq.SkipRetry)
	}
	return err
}
