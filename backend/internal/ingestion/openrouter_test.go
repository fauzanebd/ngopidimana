package ingestion

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestOpenRouterExtractorUsesStrictStructuredOutput(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost || request.URL.Path != "/chat/completions" {
			t.Errorf("unexpected OpenRouter request: %s %s", request.Method, request.URL.Path)
			response.WriteHeader(http.StatusNotFound)
			return
		}
		if request.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("missing OpenRouter bearer token")
			response.WriteHeader(http.StatusUnauthorized)
			return
		}
		var body map[string]any
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Errorf("decode request: %v", err)
			response.WriteHeader(http.StatusBadRequest)
			return
		}
		format, _ := body["response_format"].(map[string]any)
		provider, _ := body["provider"].(map[string]any)
		if format["type"] != "json_schema" || provider["require_parameters"] != true {
			t.Errorf("strict structured-output routing missing: %#v", body)
		}
		if _, present := body["temperature"]; present {
			t.Errorf("temperature must be omitted because some structured-output endpoints do not support it: %#v", body)
		}
		structured, _ := json.Marshal(ModelExtraction{
			Claims:    []ModelClaim{{Field: "facilities.wifi", Value: "yes", SourceID: "source_1", Excerpt: "Wi-Fi tersedia", Confidence: .9}},
			MenuItems: []ModelMenuItem{}, Summary: "Wi-Fi is supported.",
		})
		response.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(response).Encode(map[string]any{
			"id": "gen_test", "object": "chat.completion", "created": 1, "model": "test/model", "system_fingerprint": nil,
			"choices": []any{map[string]any{"index": 0, "finish_reason": "stop", "message": map[string]any{"role": "assistant", "content": string(structured)}}},
		})
	}))
	defer server.Close()

	extractor := NewOpenRouterEvidenceExtractor("test-key", "test/model", server.URL, "", "Where to WFC", 5*time.Second, 8000, 1000)
	result, err := extractor.ExtractEvidence(t.Context(), ExtractionInput{
		SourceURL: "https://kopi.example", Documents: []SourceDocument{{ID: "source_1", URL: "https://kopi.example", Content: "Wi-Fi tersedia"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Model != "test/model" || len(result.Claims) != 1 || result.Claims[0].Field != "facilities.wifi" {
		t.Fatalf("unexpected structured result: %#v", result)
	}
}
