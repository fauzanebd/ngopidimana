package ingestion

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/net/html"
)

const maxSourceBytes = 2 << 20
const maxSourceTextCharacters = 16000
const missingCafeNameIssue = "Could not verify a café name from the source"

type Extractor interface {
	Extract(context.Context, string) (ExtractedRecord, error)
}

type WebExtractor struct{ client *http.Client }

func NewWebExtractor() *WebExtractor {
	transport := &http.Transport{
		DialContext:           publicDialContext,
		ForceAttemptHTTP2:     true,
		ResponseHeaderTimeout: 10 * time.Second,
	}
	return &WebExtractor{client: &http.Client{
		Transport: transport,
		Timeout:   20 * time.Second,
		CheckRedirect: func(request *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return errors.New("too many redirects")
			}
			return validatePublicHost(request.Context(), request.URL.Hostname())
		},
	}}
}

func (e *WebExtractor) Extract(ctx context.Context, rawURL string) (ExtractedRecord, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return ExtractedRecord{}, fmt.Errorf("parse source URL: %w", err)
	}
	if err := validatePublicHost(ctx, parsed.Hostname()); err != nil {
		return ExtractedRecord{}, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, parsed.String(), nil)
	if err != nil {
		return ExtractedRecord{}, fmt.Errorf("build source request: %w", err)
	}
	request.Header.Set("User-Agent", "WhereToWFC-Ingestion/0.1 (+catalogue review)")
	request.Header.Set("Accept", "text/html,application/xhtml+xml")
	response, err := e.client.Do(request)
	if err != nil {
		return ExtractedRecord{}, fmt.Errorf("fetch source: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return ExtractedRecord{}, fmt.Errorf("source returned HTTP %d", response.StatusCode)
	}
	limited := io.LimitReader(response.Body, maxSourceBytes+1)
	payload, err := io.ReadAll(limited)
	if err != nil {
		return ExtractedRecord{}, fmt.Errorf("read source: %w", err)
	}
	if len(payload) > maxSourceBytes {
		return ExtractedRecord{}, errors.New("source exceeds the 2 MiB ingestion limit")
	}
	record := extractHTML(payload, response.Request.URL.String())
	if record.Name == "" {
		record.Name = parsed.Hostname()
		record.Confidence = .45
		record.Issues = append(record.Issues, missingCafeNameIssue)
	}
	return record, nil
}

func extractHTML(payload []byte, sourceURL string) ExtractedRecord {
	tokenizer := html.NewTokenizer(bufio.NewReader(strings.NewReader(string(payload))))
	var title, ogTitle, description string
	jsonLD := make([]string, 0, 2)
	visibleText := strings.Builder{}
	inTitle, inJSONLD, hiddenDepth := false, false, 0
	for {
		typeOfToken := tokenizer.Next()
		switch typeOfToken {
		case html.ErrorToken:
			goto done
		case html.StartTagToken, html.SelfClosingTagToken:
			token := tokenizer.Token()
			switch token.Data {
			case "title":
				inTitle = true
			case "meta":
				attrs := attributes(token.Attr)
				switch strings.ToLower(firstNonEmpty(attrs["property"], attrs["name"])) {
				case "og:title":
					ogTitle = attrs["content"]
				case "description", "og:description":
					if description == "" {
						description = attrs["content"]
					}
				}
			case "script":
				inJSONLD = strings.EqualFold(attributes(token.Attr)["type"], "application/ld+json")
				if typeOfToken == html.StartTagToken {
					hiddenDepth++
				}
			case "style", "noscript", "svg":
				if typeOfToken == html.StartTagToken {
					hiddenDepth++
				}
			}
		case html.TextToken:
			text := strings.TrimSpace(string(tokenizer.Text()))
			if inTitle && title == "" {
				title = text
			}
			if inJSONLD && text != "" {
				jsonLD = append(jsonLD, text)
			}
			if hiddenDepth == 0 && text != "" && visibleText.Len() < maxSourceTextCharacters {
				if visibleText.Len() > 0 {
					visibleText.WriteByte(' ')
				}
				remaining := maxSourceTextCharacters - visibleText.Len()
				if remaining > 0 {
					visibleText.WriteString(truncateText(text, remaining))
				}
			}
		case html.EndTagToken:
			switch tokenizer.Token().Data {
			case "title":
				inTitle = false
			case "script":
				inJSONLD = false
				if hiddenDepth > 0 {
					hiddenDepth--
				}
			case "style", "noscript", "svg":
				if hiddenDepth > 0 {
					hiddenDepth--
				}
			}
		}
	}

done:
	isMapsSource := isGoogleMapsURL(sourceURL)
	name := cleanTitle(firstNonEmpty(ogTitle, title))
	address, openingHours, ldName := extractStructuredData(jsonLD)
	identitySource, identityExtractor, identityConfidence := "Page metadata", "html_metadata", .88
	if mapsName := googleMapsPlaceName(sourceURL); mapsName != "" {
		name = mapsName
		identitySource, identityExtractor, identityConfidence = "Google Maps URL", "google_maps_url", .94
	} else if isMapsSource && isGenericGoogleMapsTitle(name) {
		name = ""
	}
	if name == "" && !isGenericGoogleMapsTitle(ldName) {
		name = ldName
		identitySource, identityExtractor, identityConfidence = "JSON-LD", "json_ld", .92
	}
	if isMapsSource {
		// Google Maps serves a generic localized description rather than a
		// description of the selected place.
		description = ""
	}
	fields := []EvidenceField{{Key: "Source URL", Value: sourceURL, Source: "Submitted URL", SourceID: "source_1", SourceURL: sourceURL, Method: "submitted", Extractor: "html", Confidence: 1}}
	if name != "" {
		fields = append([]EvidenceField{{Key: "Identity", Value: name, Source: identitySource, SourceID: "source_1", SourceURL: sourceURL, Method: "deterministic", Extractor: identityExtractor, Confidence: identityConfidence}}, fields...)
	}
	if description != "" {
		fields = append(fields, EvidenceField{Key: "Description", Value: description, Source: "Meta description", SourceID: "source_1", SourceURL: sourceURL, Excerpt: description, Method: "deterministic", Extractor: "html_metadata", Confidence: .76})
	}
	if address != "" {
		fields = append(fields, EvidenceField{Key: "Address", Value: address, Source: "JSON-LD", SourceID: "source_1", SourceURL: sourceURL, Method: "deterministic", Extractor: "json_ld", Confidence: .92})
	}
	if openingHours != "" {
		fields = append(fields, EvidenceField{Key: "Opening hours", Value: openingHours, Source: "JSON-LD", SourceID: "source_1", SourceURL: sourceURL, Method: "deterministic", Extractor: "json_ld", Confidence: .9})
	}
	confidence := .68
	if name != "" {
		confidence = .82
	}
	if address != "" || openingHours != "" {
		confidence = .9
	}
	documentParts := []string{}
	if isMapsSource && name != "" {
		documentParts = append(documentParts, "Google Maps place from resolved URL: "+name)
	} else if pageTitle := firstNonEmpty(ogTitle, title); pageTitle != "" {
		documentParts = append(documentParts, "Page title: "+pageTitle)
	}
	if description != "" {
		documentParts = append(documentParts, "Description: "+description)
	}
	if visibleText.Len() > 0 {
		documentParts = append(documentParts, "Visible page text: "+truncateText(strings.Join(strings.Fields(visibleText.String()), " "), maxSourceTextCharacters))
	}
	if len(jsonLD) > 0 {
		documentParts = append(documentParts, "Structured data: "+truncateText(strings.Join(jsonLD, " "), 5000))
	}
	now := time.Now().UTC().Format(time.RFC3339)
	documents := []SourceDocument{{ID: "source_1", URL: sourceURL, Kind: "submitted_html", Title: name, Content: strings.Join(documentParts, "\n"), CapturedAt: now}}
	return ExtractedRecord{Name: name, Confidence: confidence, Issues: []string{}, Fields: fields, Documents: documents}
}

func googleMapsPlaceName(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil || !isGoogleMapsURL(rawURL) {
		return ""
	}
	segments := strings.Split(strings.Trim(parsed.EscapedPath(), "/"), "/")
	for index := 0; index+1 < len(segments); index++ {
		if !strings.EqualFold(segments[index], "place") {
			continue
		}
		name, err := url.PathUnescape(segments[index+1])
		if err != nil {
			return ""
		}
		name = strings.Join(strings.Fields(strings.ReplaceAll(name, "+", " ")), " ")
		if isGenericGoogleMapsTitle(name) {
			return ""
		}
		return name
	}
	return ""
}

func isGoogleMapsURL(rawURL string) bool {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	host := strings.ToLower(parsed.Hostname())
	googleHost := host == "google.com" || strings.HasPrefix(host, "google.") || strings.Contains(host, ".google.")
	return googleHost && (parsed.Path == "/maps" || strings.HasPrefix(parsed.Path, "/maps/"))
}

func isGenericGoogleMapsTitle(value string) bool {
	value = strings.ToLower(strings.TrimSpace(value))
	return value == "" || value == "google maps" || value == "maps"
}

func extractStructuredData(documents []string) (address, hours, name string) {
	for _, document := range documents {
		var value any
		if json.Unmarshal([]byte(document), &value) != nil {
			continue
		}
		walkJSON(value, func(object map[string]any) {
			if name == "" {
				name, _ = object["name"].(string)
			}
			if hours == "" {
				switch value := object["openingHours"].(type) {
				case string:
					hours = value
				case []any:
					parts := make([]string, 0, len(value))
					for _, item := range value {
						if text, ok := item.(string); ok {
							parts = append(parts, text)
						}
					}
					hours = strings.Join(parts, ", ")
				}
			}
			if address == "" {
				switch value := object["address"].(type) {
				case string:
					address = value
				case map[string]any:
					parts := []string{}
					for _, key := range []string{"streetAddress", "addressLocality", "addressRegion", "postalCode"} {
						if text, ok := value[key].(string); ok && text != "" {
							parts = append(parts, text)
						}
					}
					address = strings.Join(parts, ", ")
				}
			}
		})
	}
	return address, hours, cleanTitle(name)
}

func walkJSON(value any, visit func(map[string]any)) {
	switch typed := value.(type) {
	case map[string]any:
		visit(typed)
		for _, child := range typed {
			walkJSON(child, visit)
		}
	case []any:
		for _, child := range typed {
			walkJSON(child, visit)
		}
	}
}

func publicDialContext(ctx context.Context, network, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, err
	}
	addresses, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, err
	}
	for _, address := range addresses {
		if isPublicIP(address.IP) {
			return (&net.Dialer{Timeout: 8 * time.Second}).DialContext(ctx, network, net.JoinHostPort(address.IP.String(), port))
		}
	}
	return nil, errors.New("source host resolves only to private or local addresses")
}

func validatePublicHost(ctx context.Context, host string) error {
	if host == "" || strings.EqualFold(host, "localhost") || strings.HasSuffix(strings.ToLower(host), ".local") {
		return errors.New("local source URLs are not allowed")
	}
	addresses, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return fmt.Errorf("resolve source host: %w", err)
	}
	for _, address := range addresses {
		if isPublicIP(address.IP) {
			return nil
		}
	}
	return errors.New("source host resolves only to private or local addresses")
}

func isPublicIP(ip net.IP) bool {
	return ip != nil && !ip.IsPrivate() && !ip.IsLoopback() && !ip.IsLinkLocalUnicast() && !ip.IsLinkLocalMulticast() && !ip.IsUnspecified() && !ip.IsMulticast()
}

func attributes(attrs []html.Attribute) map[string]string {
	values := make(map[string]string, len(attrs))
	for _, attr := range attrs {
		values[strings.ToLower(attr.Key)] = strings.TrimSpace(attr.Val)
	}
	return values
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func cleanTitle(value string) string {
	value = strings.Join(strings.Fields(value), " ")
	for _, separator := range []string{" | ", " — ", " – "} {
		if before, _, found := strings.Cut(value, separator); found && len([]rune(before)) >= 3 {
			return before
		}
	}
	return value
}
