package recommendation

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	typesafe "github.com/atharvamhaske/typesafe-sdk-go"
)

func TestJevInterpreterUsesOneBatchedRequest(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		calls.Add(1)
		if request.Header.Get("Authorization") != "Bearer test-key" {
			t.Fatal("missing SDK authorization header")
		}
		var input struct {
			Questions map[string]json.RawMessage `json:"questions"`
		}
		if err := json.NewDecoder(request.Body).Decode(&input); err != nil {
			t.Fatal(err)
		}
		if len(input.Questions) != len(jevDimensions)+1 {
			t.Fatalf("questions = %d, want %d", len(input.Questions), len(jevDimensions)+1)
		}
		answers := map[string]any{}
		for _, dimension := range jevDimensions {
			score, confidence := 0.0, .91
			if dimension.Key == "musholla" {
				score, confidence = 4, .96
			} else if dimension.Key == "wfc" || dimension.Key == "quiet" {
				score = 3.2
			}
			answers[dimension.Key+"_importance"] = map[string]any{"type": "score", "score": score, "confidence": confidence, "legend": map[string]string{}, "probabilities": map[string]float64{}}
		}
		answers["visit_purpose"] = map[string]any{"type": "choice", "choice": "focused_work", "confidence": .93, "probabilities": map[string]float64{"focused_work": .93}}
		writeTestJSON(writer, map[string]any{"model": "jev-contract-test", "answers": answers, "usage": map[string]int{"input_tokens": 100, "output_tokens": 20}})
	}))
	defer server.Close()

	client, err := typesafe.NewClient(typesafe.WithAPIKey("test-key"), typesafe.WithBaseURL(server.URL), typesafe.WithModel("jev-latest"), typesafe.WithMaxRetries(0))
	if err != nil {
		t.Fatal(err)
	}
	profile, err := (jevInterpreter{client: client}).Interpret(t.Context(), "WFC harus ada musholla")
	if err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 1 || profile.Provider != "jev" || len(profile.HardConstraints) != 1 || profile.HardConstraints[0].Key != "musholla" {
		t.Fatalf("unexpected Jev contract result: calls=%d profile=%#v", calls.Load(), profile)
	}
}

func writeTestJSON(writer http.ResponseWriter, value any) {
	writer.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(writer).Encode(value)
}
