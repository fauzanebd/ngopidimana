package recommendation

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	typesafe "github.com/atharvamhaske/typesafe-sdk-go"
)

const jevRequestTimeout = 900 * time.Millisecond

var budgetPattern = regexp.MustCompile(`(?i)(?:rp\s*)?(\d{2,3})(?:\s*(?:k|rb|ribu))`)
var open24HoursPattern = regexp.MustCompile(`(?i)(?:\b24\s*(?:jam|hours?|hrs?|h)\b|\b24\s*/\s*7\b|\bbuka\s+24\s*jam\b)`)

type Interpreter interface {
	Interpret(context.Context, string) (Interpretation, error)
}

type deterministicInterpreter struct{}

func (deterministicInterpreter) Interpret(_ context.Context, query string) (Interpretation, error) {
	return InterpretDeterministically(query), nil
}

type jevInterpreter struct{ client *typesafe.Client }

type jevDimension struct {
	Key, Label, Question string
	HardFilter           bool
}

var jevDimensions = []jevDimension{
	{Key: "wfc", Label: "WFC friendly", Question: "How important is suitability for working or studying from this cafe in the user's request?"},
	{Key: "musholla", Label: "Musholla", HardFilter: true, Question: "How important is having an on-site musholla or prayer room in the user's request?"},
	{Key: "quiet", Label: "Quiet", Question: "How important is a quiet or low-noise environment in the user's request?"},
	{Key: "minimalist", Label: "Minimalist", Question: "How important is a minimalist interior or aesthetic in the user's request?"},
	{Key: "matcha", Label: "Matcha", HardFilter: true, Question: "How important is matcha availability in the user's request?"},
	{Key: "wifi", Label: "Fast Wi-Fi", HardFilter: true, Question: "How important is reliable Wi-Fi in the user's request?"},
	{Key: "outlets", Label: "Power outlets", HardFilter: true, Question: "How important is access to electrical power outlets in the user's request?"},
	{Key: "outdoor", Label: "Outdoor seating", Question: "How important is outdoor or garden seating in the user's request?"},
	{Key: "parking", Label: "Easy parking", Question: "How important is convenient parking in the user's request?"},
	{Key: "late", Label: "Open late", Question: "How important is staying open late at night in the user's request?"},
	{Key: "open_24h", Label: "Open 24 hours", HardFilter: true, Question: "How important is specifically operating continuously for 24 hours, rather than merely closing late, in the user's request?"},
	{Key: "breakfast", Label: "Breakfast", Question: "How important is breakfast or brunch availability in the user's request?"},
	{Key: "pet_friendly", Label: "Pet friendly", HardFilter: true, Question: "How important is allowing pets in the user's request?"},
}

var importanceScale = []string{
	"0 — not mentioned or irrelevant",
	"1 — optional nice-to-have",
	"2 — preferred",
	"3 — important",
	"4 — mandatory; the user explicitly requires it",
}

func NewInterpreter(apiKey, model string) (Interpreter, string) {
	if strings.TrimSpace(apiKey) == "" {
		return deterministicInterpreter{}, "deterministic (set TYPESAFE_API_KEY to enable Jev)"
	}
	client, err := typesafe.NewClient(
		typesafe.WithAPIKey(apiKey),
		typesafe.WithModel(model),
		typesafe.WithHTTPClient(&http.Client{Timeout: jevRequestTimeout}),
		typesafe.WithMaxRetries(0),
	)
	if err != nil {
		return deterministicInterpreter{}, "deterministic (Jev configuration invalid)"
	}
	return jevInterpreter{client: client}, "jev"
}

func (j jevInterpreter) Interpret(ctx context.Context, query string) (Interpretation, error) {
	started := time.Now()
	requestCtx, cancel := context.WithTimeout(ctx, jevRequestTimeout)
	defer cancel()

	questions := make(map[string]typesafe.Question, len(jevDimensions)+1)
	for _, dimension := range jevDimensions {
		questions[dimension.Key+"_importance"] = typesafe.Score{Instructions: dimension.Question, Criteria: importanceScale}
	}
	questions["visit_purpose"] = typesafe.Choice{
		Instructions: "What is the primary visit purpose expressed by the user?",
		Criteria: map[string]string{
			"focused_work": "Focused laptop work or a long coding session", "casual_work": "Light work with flexibility for some activity or noise",
			"study": "Studying, reading, or completing assignments", "meeting": "A work meeting, discussion, or call",
			"social": "Socializing with friends or family", "date": "A date or intimate conversation", "unspecified": "No primary purpose is clear",
		},
	}
	response, err := j.client.SystemOne(requestCtx, query, questions)
	if err != nil {
		return Interpretation{}, fmt.Errorf("jev request failed: %w", err)
	}
	scores := response.Scores()
	if len(scores) == 0 {
		return Interpretation{}, errors.New("jev returned no preference scores")
	}

	profile := Interpretation{Location: detectArea(strings.ToLower(query)), Provider: "jev", Model: response.Model, LatencyMS: time.Since(started).Milliseconds()}
	if profile.Location == "" {
		profile.Location = "Jakarta"
	}
	if match := budgetPattern.FindStringSubmatch(strings.ToLower(query)); len(match) == 2 {
		if value, scanErr := strconv.Atoi(match[1]); scanErr == nil {
			profile.Budget = value * 1000
		}
	}
	confidenceTotal, included := 0.0, 0
	for _, dimension := range jevDimensions {
		answer, ok := scores[dimension.Key+"_importance"]
		if !ok {
			continue
		}
		weight := clamp(answer.Score/4, 0, 1)
		if weight < 0.34 {
			continue
		}
		req := Requirement{Key: dimension.Key, Label: dimension.Label, Kind: "soft", Weight: round(weight), Confidence: round(answer.Confidence)}
		if dimension.HardFilter && answer.Score >= 3.5 && answer.Confidence >= 0.82 {
			req.Kind = "hard"
			profile.HardConstraints = append(profile.HardConstraints, req)
		} else {
			profile.SoftPreferences = append(profile.SoftPreferences, req)
		}
		confidenceTotal += answer.Confidence
		included++
	}
	if included == 0 {
		return Interpretation{}, errors.New("jev did not identify a usable preference")
	}
	sort.SliceStable(profile.HardConstraints, func(a, b int) bool { return profile.HardConstraints[a].Weight > profile.HardConstraints[b].Weight })
	sort.SliceStable(profile.SoftPreferences, func(a, b int) bool { return profile.SoftPreferences[a].Weight > profile.SoftPreferences[b].Weight })
	profile.Confidence = round(confidenceTotal / float64(included))
	profile.Summary = interpretationSummary(profile)
	return profile, nil
}

func InterpretDeterministically(raw string) Interpretation {
	q := strings.ToLower(raw)
	profile := Interpretation{Location: "Jakarta", Confidence: 0.92, Provider: "deterministic"}
	if area := detectArea(q); area != "" {
		profile.Location = area
	}
	if match := budgetPattern.FindStringSubmatch(q); len(match) == 2 {
		if value, err := strconv.Atoi(match[1]); err == nil {
			profile.Budget = value * 1000
		}
	}
	features := []struct {
		key, label string
		aliases    []string
		weight     float64
	}{
		{"wfc", "WFC friendly", []string{"wfc", "work from cafe", "kerja", "laptop", "coding", "nugas"}, .88},
		{"musholla", "Musholla", []string{"musholla", "musala", "prayer room", "sholat"}, 1},
		{"quiet", "Quiet", []string{"quiet", "tenang", "hening", "nggak berisik", "tidak berisik"}, .92},
		{"minimalist", "Minimalist", []string{"minimalist", "minimalis", "clean interior"}, .68},
		{"matcha", "Matcha", []string{"matcha"}, .72}, {"wifi", "Fast Wi-Fi", []string{"wifi", "wi-fi", "internet"}, .82},
		{"outlets", "Power outlets", []string{"outlet", "colokan", "power"}, .8}, {"outdoor", "Outdoor seating", []string{"outdoor", "garden", "taman"}, .62},
		{"parking", "Easy parking", []string{"parking", "parkir"}, .58}, {"late", "Open late", []string{"open late", "buka malam", "sampai malam"}, .58},
		{"open_24h", "Open 24 hours", []string{"24 jam", "24h", "24 hours", "24 hour", "24/7", "buka 24 jam"}, 1},
		{"breakfast", "Breakfast", []string{"breakfast", "sarapan", "brunch"}, .52}, {"pet_friendly", "Pet friendly", []string{"pet friendly", "bawa anjing", "pets"}, .48},
	}
	filterable := map[string]bool{"musholla": true, "matcha": true, "wifi": true, "outlets": true, "pet_friendly": true, "open_24h": true}
	for _, feature := range features {
		if !containsAny(q, feature.aliases) {
			continue
		}
		req := Requirement{Key: feature.key, Label: feature.label, Weight: feature.weight, Confidence: .94, Kind: "soft"}
		if filterable[feature.key] && (feature.key == "open_24h" || isHardMention(q, feature.aliases)) {
			req.Kind = "hard"
			profile.HardConstraints = append(profile.HardConstraints, req)
		} else {
			profile.SoftPreferences = append(profile.SoftPreferences, req)
		}
	}
	if len(profile.HardConstraints)+len(profile.SoftPreferences) == 0 {
		profile.SoftPreferences = []Requirement{{Key: "wfc", Label: "WFC friendly", Kind: "soft", Weight: .84, Confidence: .72}, {Key: "quiet", Label: "Comfortable", Kind: "soft", Weight: .5, Confidence: .62}}
		profile.Confidence = .7
	}
	profile.Summary = interpretationSummary(profile)
	return profile
}

func enforceDeterministicConstraints(raw string, profile Interpretation) Interpretation {
	if !open24HoursPattern.MatchString(raw) {
		return profile
	}
	profile.HardConstraints = withoutRequirement(profile.HardConstraints, "open_24h", "late")
	profile.SoftPreferences = withoutRequirement(profile.SoftPreferences, "open_24h", "late")
	profile.HardConstraints = append(profile.HardConstraints, Requirement{Key: "open_24h", Label: "Open 24 hours", Kind: "hard", Weight: 1, Confidence: 1})
	profile.Summary = interpretationSummary(profile)
	return profile
}

func withoutRequirement(requirements []Requirement, keys ...string) []Requirement {
	excluded := make(map[string]bool, len(keys))
	for _, key := range keys {
		excluded[key] = true
	}
	filtered := requirements[:0]
	for _, req := range requirements {
		if !excluded[req.Key] {
			filtered = append(filtered, req)
		}
	}
	return filtered
}

func interpretationSummary(profile Interpretation) string {
	parts := []string{profile.Location}
	if profile.Budget > 0 {
		parts = append(parts, fmt.Sprintf("around Rp%dk", profile.Budget/1000))
	}
	if len(profile.HardConstraints) > 0 {
		parts = append(parts, fmt.Sprintf("%d must-have", len(profile.HardConstraints)))
	}
	parts = append(parts, fmt.Sprintf("%d preference", len(profile.SoftPreferences)))
	return strings.Join(parts, " · ")
}

func detectArea(query string) string {
	if scope, ok := detectNamedLocation(query); ok {
		return scope.Label
	}
	return ""
}

func isHardMention(query string, aliases []string) bool {
	markers := []string{"must", "harus", "wajib", "need", "butuh", "mandatory", "nggak boleh tanpa", "tidak boleh tanpa"}
	for _, alias := range aliases {
		index := strings.Index(query, alias)
		if index >= 0 && containsAny(query[max(0, index-28):min(len(query), index+len(alias)+12)], markers) {
			return true
		}
	}
	return false
}

func containsAny(value string, terms []string) bool {
	for _, term := range terms {
		if strings.Contains(value, term) {
			return true
		}
	}
	return false
}

func clamp(value, low, high float64) float64 { return max(low, min(high, value)) }
func round(value float64) float64            { return float64(int(value*100+.5)) / 100 }
