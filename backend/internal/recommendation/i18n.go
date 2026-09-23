package recommendation

import (
	"fmt"
	"strings"
)

type Locale string

const (
	LocaleEnglish    Locale = "en"
	LocaleIndonesian Locale = "id"
)

// NormalizeLocale accepts any case and any region suffix; only the language
// subtag decides the locale, so "id-ID" and "in" both mean Indonesian.
func NormalizeLocale(value string) Locale {
	language := strings.ToLower(strings.TrimSpace(value))
	if index := strings.IndexAny(language, "-_"); index >= 0 {
		language = language[:index]
	}
	if language == "id" || language == "in" {
		return LocaleIndonesian
	}
	return LocaleEnglish
}

type catalog struct {
	labels            map[string]string
	summaryBudget     string
	summaryMustHave   string
	summaryPreference string
	nearYou           string
	matchedFallback   string
	staleCaveat       string
	budgetCaveat      string
	evidenceSummary   string
	fallbackReason    string
}

func (c catalog) label(key, fallback string) string {
	if label, ok := c.labels[key]; ok {
		return label
	}
	return fallback
}

var catalogs = map[Locale]catalog{
	// English is the language the interpreter already generates, so its catalogue
	// carries no label overrides: every generated English label is already final.
	LocaleEnglish: {
		summaryBudget:     "around Rp%dk",
		summaryMustHave:   "%d must-have",
		summaryPreference: "%d preference",
		nearYou:           "Near you",
		matchedFallback:   "Good overall fit",
		staleCaveat:       "Some evidence was last checked %d days ago",
		budgetCaveat:      "Some menu items may exceed your budget",
		evidenceSummary:   "%d verified signals · checked %d days ago",
		fallbackReason:    "Jev was unavailable; deterministic interpretation used",
	},
	LocaleIndonesian: {
		labels: map[string]string{
			"wfc": "Buat kerja", "musholla": "Musholla", "quiet": "Tenang", "minimalist": "Minimalis",
			"matcha": "Matcha", "wifi": "Wi-Fi cepat", "outlets": "Colokan listrik", "outdoor": "Area outdoor",
			"parking": "Parkir mudah", "late": "Buka sampai malam", "open_24h": "Buka 24 jam",
			"breakfast": "Sarapan", "pet_friendly": "Ramah hewan",
		},
		summaryBudget:     "sekitar Rp%dk",
		summaryMustHave:   "%d wajib",
		summaryPreference: "%d preferensi",
		nearYou:           "Di dekat kamu",
		matchedFallback:   "Cocok secara keseluruhan",
		staleCaveat:       "Sebagian bukti terakhir dicek %d hari lalu",
		budgetCaveat:      "Beberapa menu bisa di atas budget kamu",
		evidenceSummary:   "%d sinyal terverifikasi · dicek %d hari lalu",
		fallbackReason:    "Jev tidak tersedia; interpretasi deterministik dipakai",
	},
}

func catalogFor(locale Locale) catalog {
	if entry, ok := catalogs[locale]; ok {
		return entry
	}
	return catalogs[LocaleEnglish]
}

func interpretationSummary(profile Interpretation, locale Locale) string {
	c := catalogFor(locale)
	parts := []string{profile.Location}
	if profile.Budget > 0 {
		parts = append(parts, fmt.Sprintf(c.summaryBudget, profile.Budget/1000))
	}
	if len(profile.HardConstraints) > 0 {
		parts = append(parts, fmt.Sprintf(c.summaryMustHave, len(profile.HardConstraints)))
	}
	parts = append(parts, fmt.Sprintf(c.summaryPreference, len(profile.SoftPreferences)))
	return strings.Join(parts, " · ")
}

func localizeInterpretation(profile Interpretation, locale Locale) Interpretation {
	c := catalogFor(locale)
	profile.HardConstraints = localizeRequirements(profile.HardConstraints, c)
	profile.SoftPreferences = localizeRequirements(profile.SoftPreferences, c)
	if profile.Location == catalogs[LocaleEnglish].nearYou {
		profile.Location = c.nearYou
	}
	profile.Summary = interpretationSummary(profile, locale)
	return profile
}

func localizeRequirements(requirements []Requirement, c catalog) []Requirement {
	for index := range requirements {
		requirements[index].Label = c.label(requirements[index].Key, requirements[index].Label)
	}
	return requirements
}
