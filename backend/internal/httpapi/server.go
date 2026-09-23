package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/fauzanebd/wheretowfc/backend/internal/googleplaces"
	"github.com/fauzanebd/wheretowfc/backend/internal/ingestion"
	"github.com/fauzanebd/wheretowfc/backend/internal/recommendation"
)

type HealthChecker interface{ Ping(context.Context) error }

type GooglePlacesClient interface {
	Enabled() bool
	GetPlace(context.Context, string) (googleplaces.Place, error)
}

type Server struct {
	recommendations *recommendation.Service
	ingestion       *ingestion.Service
	googlePlaces    GooglePlacesClient
	health          HealthChecker
	auth            AuthService
	cookies         CookiePolicy
}

// Options is everything NewServer needs. It is a struct rather than a parameter
// list because the API surface keeps growing and positional arguments stopped
// being readable.
type Options struct {
	Recommendations *recommendation.Service
	Ingestion       *ingestion.Service
	GooglePlaces    GooglePlacesClient
	Health          HealthChecker
	CORSOrigins     string
	Auth            AuthService
	Cookies         CookiePolicy
}

func NewServer(options Options) http.Handler {
	server := &Server{
		recommendations: options.Recommendations, ingestion: options.Ingestion, googlePlaces: options.GooglePlaces,
		health: options.Health, auth: options.Auth, cookies: options.Cookies,
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", server.healthz)
	mux.HandleFunc("POST /v1/recommendations", server.recommend)
	mux.HandleFunc("POST /v1/auth/request-link", server.requestLink)
	mux.HandleFunc("POST /v1/auth/verify", server.verifyLogin)
	mux.HandleFunc("GET /v1/auth/session", server.session)
	mux.HandleFunc("POST /v1/auth/logout", server.logout)

	admin := http.NewServeMux()
	admin.HandleFunc("GET /v1/admin/ingestion-runs", server.listIngestionRuns)
	admin.HandleFunc("POST /v1/admin/ingestion-runs", server.createIngestionRun)
	admin.HandleFunc("PATCH /v1/admin/ingestion-runs/{id}", server.updateIngestionRun)
	admin.HandleFunc("DELETE /v1/admin/ingestion-runs/{id}", server.deleteIngestionRun)
	admin.HandleFunc("GET /v1/admin/ingestion-runs/{id}/google-place", server.getGooglePlace)
	admin.HandleFunc("POST /v1/admin/ingestion-runs/{id}/manual-evidence", server.addManualEvidence)
	admin.HandleFunc("DELETE /v1/admin/ingestion-runs/{id}/manual-evidence/{evidenceID}", server.removeManualEvidence)
	admin.HandleFunc("PATCH /v1/admin/ingestion-runs/{id}/evidence/{evidenceID}", server.replaceEvidence)
	admin.HandleFunc("DELETE /v1/admin/ingestion-runs/{id}/evidence/{evidenceID}", server.excludeEvidence)
	admin.HandleFunc("POST /v1/admin/ingestion-runs/{id}/evidence/{evidenceID}/restore", server.restoreEvidence)
	admin.HandleFunc("POST /v1/admin/ingestion-runs/{id}/evidence/{evidenceID}/choose", server.chooseEvidence)
	mux.Handle("/v1/admin/", server.requireContributor(admin))

	return withLogging(withCORS(options.CORSOrigins, mux))
}

func (s *Server) replaceEvidence(writer http.ResponseWriter, request *http.Request) {
	var input struct {
		Value string `json:"value"`
	}
	if err := decodeJSON(request, &input); err != nil {
		writeError(writer, http.StatusBadRequest, err.Error())
		return
	}
	run, err := s.ingestion.ReplaceEvidence(request.Context(), request.PathValue("id"), request.PathValue("evidenceID"), input.Value)
	writeEvidenceMutation(writer, run, err)
}

func (s *Server) excludeEvidence(writer http.ResponseWriter, request *http.Request) {
	run, err := s.ingestion.SetEvidenceExcluded(request.Context(), request.PathValue("id"), request.PathValue("evidenceID"), true)
	writeEvidenceMutation(writer, run, err)
}

func (s *Server) restoreEvidence(writer http.ResponseWriter, request *http.Request) {
	run, err := s.ingestion.SetEvidenceExcluded(request.Context(), request.PathValue("id"), request.PathValue("evidenceID"), false)
	writeEvidenceMutation(writer, run, err)
}

func (s *Server) chooseEvidence(writer http.ResponseWriter, request *http.Request) {
	run, err := s.ingestion.ChooseEvidence(request.Context(), request.PathValue("id"), request.PathValue("evidenceID"))
	writeEvidenceMutation(writer, run, err)
}

func writeEvidenceMutation(writer http.ResponseWriter, run ingestion.Run, err error) {
	if err == nil {
		writeJSON(writer, http.StatusOK, run)
		return
	}
	switch {
	case errors.Is(err, ingestion.ErrNotFound):
		writeError(writer, http.StatusNotFound, err.Error())
	case errors.Is(err, ingestion.ErrInvalidEvidence):
		writeError(writer, http.StatusBadRequest, err.Error())
	default:
		writeError(writer, http.StatusServiceUnavailable, "could not update evidence")
	}
}

func (s *Server) addManualEvidence(writer http.ResponseWriter, request *http.Request) {
	var input ingestion.ManualEvidenceInput
	if err := decodeJSON(request, &input); err != nil {
		writeError(writer, http.StatusBadRequest, err.Error())
		return
	}
	run, err := s.ingestion.AddManualEvidence(request.Context(), request.PathValue("id"), input)
	if err != nil {
		switch {
		case errors.Is(err, ingestion.ErrNotFound):
			writeError(writer, http.StatusNotFound, err.Error())
		case errors.Is(err, ingestion.ErrInvalidManualEvidence), strings.Contains(err.Error(), "cannot be changed"):
			writeError(writer, http.StatusBadRequest, err.Error())
		default:
			writeError(writer, http.StatusServiceUnavailable, "could not add manual evidence")
		}
		return
	}
	writeJSON(writer, http.StatusCreated, run)
}

func (s *Server) removeManualEvidence(writer http.ResponseWriter, request *http.Request) {
	run, err := s.ingestion.RemoveManualEvidence(request.Context(), request.PathValue("id"), request.PathValue("evidenceID"))
	if err != nil {
		if errors.Is(err, ingestion.ErrNotFound) {
			writeError(writer, http.StatusNotFound, err.Error())
			return
		}
		writeError(writer, http.StatusServiceUnavailable, "could not remove manual evidence")
		return
	}
	writeJSON(writer, http.StatusOK, run)
}

func (s *Server) getGooglePlace(writer http.ResponseWriter, request *http.Request) {
	writer.Header().Set("Cache-Control", "no-store, max-age=0")
	writer.Header().Set("Pragma", "no-cache")
	if s.googlePlaces == nil || !s.googlePlaces.Enabled() {
		writeError(writer, http.StatusServiceUnavailable, "Google Places is not configured")
		return
	}
	run, err := s.ingestion.Get(request.Context(), request.PathValue("id"))
	if err != nil {
		if errors.Is(err, ingestion.ErrNotFound) {
			writeError(writer, http.StatusNotFound, err.Error())
			return
		}
		writeError(writer, http.StatusServiceUnavailable, "could not load the ingestion run")
		return
	}
	if run.GooglePlaceID == "" {
		writeError(writer, http.StatusNotFound, "this record has not been linked to a Google Place; re-run extraction after configuring GOOGLE_PLACES_API_KEY")
		return
	}
	place, err := s.googlePlaces.GetPlace(request.Context(), run.GooglePlaceID)
	if err != nil {
		if errors.Is(err, googleplaces.ErrPlaceNotFound) {
			writeError(writer, http.StatusNotFound, err.Error())
			return
		}
		writeError(writer, http.StatusBadGateway, "Google Place Details is temporarily unavailable")
		return
	}
	writeJSON(writer, http.StatusOK, place)
}

func (s *Server) healthz(writer http.ResponseWriter, request *http.Request) {
	if err := s.health.Ping(request.Context()); err != nil {
		writeJSON(writer, http.StatusServiceUnavailable, map[string]any{"status": "degraded", "redis": "unavailable"})
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{"status": "ok", "redis": "ok", "catalogue_size": s.recommendations.CatalogueSize()})
}

func (s *Server) recommend(writer http.ResponseWriter, request *http.Request) {
	var input recommendation.Request
	if err := decodeJSON(request, &input); err != nil {
		writeError(writer, http.StatusBadRequest, err.Error())
		return
	}
	input.Query = recommendation.NormalizeQuery(input.Query)
	if len([]rune(input.Query)) < 3 {
		writeError(writer, http.StatusBadRequest, "query must contain at least 3 characters")
		return
	}
	if input.Limit == 0 {
		input.Limit = 15
	}
	if input.Limit < 10 || input.Limit > 20 {
		writeError(writer, http.StatusBadRequest, "limit must be between 10 and 20")
		return
	}
	if recommendation.RequiresUserLocation(input.Query) && input.UserLocation == nil {
		writeError(writer, http.StatusUnprocessableEntity, "this search needs your location; allow location access and try again")
		return
	}
	if input.UserLocation != nil && !recommendation.ValidPoint(*input.UserLocation) {
		writeError(writer, http.StatusBadRequest, "user_location must contain valid latitude and longitude")
		return
	}
	writeJSON(writer, http.StatusOK, s.recommendations.Recommend(request.Context(), input))
}

func (s *Server) listIngestionRuns(writer http.ResponseWriter, request *http.Request) {
	runs, err := s.ingestion.List(request.Context())
	if err != nil {
		writeError(writer, http.StatusServiceUnavailable, "could not load the ingestion queue")
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{"runs": runs})
}

func (s *Server) createIngestionRun(writer http.ResponseWriter, request *http.Request) {
	var body struct {
		URL string `json:"url"`
	}
	if err := decodeJSON(request, &body); err != nil {
		writeError(writer, http.StatusBadRequest, err.Error())
		return
	}
	run, err := s.ingestion.Create(request.Context(), body.URL)
	if err != nil {
		status := http.StatusServiceUnavailable
		if strings.Contains(err.Error(), "valid http") || strings.Contains(err.Error(), "credentials") {
			status = http.StatusBadRequest
		}
		writeError(writer, status, err.Error())
		return
	}
	writeJSON(writer, http.StatusAccepted, run)
}

func (s *Server) updateIngestionRun(writer http.ResponseWriter, request *http.Request) {
	var body struct {
		Action string `json:"action"`
	}
	if err := decodeJSON(request, &body); err != nil {
		writeError(writer, http.StatusBadRequest, err.Error())
		return
	}
	run, err := s.ingestion.Update(request.Context(), request.PathValue("id"), body.Action)
	if err != nil {
		switch {
		case errors.Is(err, ingestion.ErrNotFound):
			writeError(writer, http.StatusNotFound, err.Error())
		case errors.Is(err, ingestion.ErrConflicts):
			writeError(writer, http.StatusConflict, err.Error())
		case errors.Is(err, ingestion.ErrInvalidAction), strings.Contains(err.Error(), "only reviewed"):
			writeError(writer, http.StatusBadRequest, err.Error())
		case strings.Contains(err.Error(), "publish catalogue record"):
			writeError(writer, http.StatusUnprocessableEntity, err.Error())
		default:
			writeError(writer, http.StatusServiceUnavailable, "could not update the ingestion run")
		}
		return
	}
	writeJSON(writer, http.StatusOK, run)
}

func (s *Server) deleteIngestionRun(writer http.ResponseWriter, request *http.Request) {
	if err := s.ingestion.Delete(request.Context(), request.PathValue("id")); err != nil {
		if errors.Is(err, ingestion.ErrNotFound) {
			writeError(writer, http.StatusNotFound, err.Error())
			return
		}
		writeError(writer, http.StatusServiceUnavailable, "could not delete the ingestion run")
		return
	}
	writer.WriteHeader(http.StatusNoContent)
}
