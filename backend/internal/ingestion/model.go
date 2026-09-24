package ingestion

import (
	"context"
	"time"

	"github.com/fauzanebd/wheretowfc/backend/internal/googleplaces"
)

type EvidenceField struct {
	ID          string  `json:"id,omitempty"`
	Key         string  `json:"key"`
	Value       string  `json:"value"`
	Source      string  `json:"source"`
	SourceID    string  `json:"source_id,omitempty"`
	SourceURL   string  `json:"source_url,omitempty"`
	Excerpt     string  `json:"excerpt,omitempty"`
	CapturedAt  string  `json:"captured_at,omitempty"`
	PublishedAt string  `json:"published_at,omitempty"`
	Method      string  `json:"method,omitempty"`
	Extractor   string  `json:"extractor,omitempty"`
	AuthorName  string  `json:"author_name,omitempty"`
	AuthorURL   string  `json:"author_url,omitempty"`
	AuthorPhoto string  `json:"author_photo,omitempty"`
	Confidence  float64 `json:"confidence"`
	Conflict    bool    `json:"conflict"`
	Excluded    bool    `json:"excluded,omitempty"`
}

type ManualEvidenceInput struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type Run struct {
	ID            string                 `json:"id"`
	URL           string                 `json:"url"`
	Source        string                 `json:"source"`
	State         string                 `json:"state"`
	Stage         string                 `json:"stage"`
	Progress      int                    `json:"progress"`
	Name          string                 `json:"name"`
	Area          string                 `json:"area,omitempty"`
	Confidence    float64                `json:"confidence"`
	Issues        []string               `json:"issues"`
	Warnings      []string               `json:"warnings"`
	Fields        []EvidenceField        `json:"fields"`
	GooglePlaceID string                 `json:"google_place_id,omitempty"`
	Photo         *googleplaces.PhotoRef `json:"photo,omitempty"`
	CreatedAt     time.Time              `json:"created_at"`
	UpdatedAt     time.Time              `json:"updated_at"`
}

type DiscoverySeed struct {
	Name string
	Area string
	URL  string
}

type DiscoveredSource struct {
	Title       string
	URL         string
	Excerpt     string
	PublishedAt string
	Kind        string
}

type SourceDocument struct {
	ID          string
	URL         string
	Kind        string
	Title       string
	Content     string
	CapturedAt  string
	PublishedAt string
}

type ExtractedRecord struct {
	Name       string
	Area       string
	Confidence float64
	Issues     []string
	Fields     []EvidenceField
	Documents  []SourceDocument
}

type PlaceResolver interface {
	ResolvePlaceID(ctx context.Context, name, area string) (string, error)
}
