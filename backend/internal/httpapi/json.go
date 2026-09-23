package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
)

func decodeJSON(request *http.Request, target any) error {
	defer request.Body.Close()
	if contentType := request.Header.Get("Content-Type"); contentType != "" {
		mediaType, _, err := mime.ParseMediaType(contentType)
		if err != nil || mediaType != "application/json" {
			return errors.New("content type must be application/json")
		}
	} else if request.ContentLength != 0 {
		// An HTML form on another origin can post a text/plain body that happens to
		// parse as JSON, and a cross-site form cannot set a non-safelisted content
		// type without a CORS preflight. Requiring application/json therefore closes
		// that CSRF path for cookie-authenticated endpoints.
		return errors.New("content type must be application/json")
	}
	decoder := json.NewDecoder(io.LimitReader(request.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return errors.New("invalid JSON payload")
	}
	return nil
}

func writeJSON(writer http.ResponseWriter, status int, value any) {
	writer.Header().Set("Content-Type", "application/json; charset=utf-8")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(value)
}

func writeError(writer http.ResponseWriter, status int, message string) {
	writeJSON(writer, status, map[string]string{"error": message})
}
