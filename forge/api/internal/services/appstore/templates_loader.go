package appstore

import (
	"embed"
	"fmt"
	"log/slog"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// The bundled compose catalog is copied from the Coolify template library
// (reference/app-platforms/coolify/templates/compose) into this package's
// assets directory so it can be embedded with //go:embed.
//
// Placement decision: go:embed cannot reach across package boundaries, so the
// assets live under internal/services/appstore/assets/ (inside the appstore
// package) rather than a top-level forge/api/assets/ folder. This keeps the
// compiled daemon self-contained (no runtime filesystem lookups) and mirrors
// the eggseeder package, which embeds its JSON templates the same way.
//
// Both *.yaml and *.yml are embedded: the library ships 368 .yaml plus 3 .yml
// files (371 services total). The service-templates*.json manifests are copied
// alongside for reference but are deliberately NOT embedded — the glob matches
// only the YAML bodies.
//
//go:embed assets/appstore-templates/*.yaml assets/appstore-templates/*.yml
var bundledTemplatesFS embed.FS

const bundledTemplatesDir = "assets/appstore-templates"

// BundledTemplate is one parsed Coolify compose template: its catalog metadata
// plus the raw compose body, ready to be stored as an app's ComposeContent.
type BundledTemplate struct {
	Key         string
	Name        string
	Slogan      string
	Description string
	Category    string
	Tags        []string
	Logo        string
	Port        int
	ComposeYAML string
}

// LoadBundledTemplates parses every embedded compose template into a
// BundledTemplate. A file whose body is not valid YAML (or has no "services"
// key) is skipped and logged, so one malformed upstream template cannot abort
// catalog population.
func LoadBundledTemplates() ([]BundledTemplate, error) {
	entries, err := bundledTemplatesFS.ReadDir(bundledTemplatesDir)
	if err != nil {
		return nil, fmt.Errorf("read bundled template assets: %w", err)
	}

	templates := make([]BundledTemplate, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		switch strings.ToLower(filepath.Ext(name)) {
		case ".yaml", ".yml":
		default:
			continue
		}

		raw, err := bundledTemplatesFS.ReadFile(bundledTemplatesDir + "/" + name)
		if err != nil {
			slog.Warn("bundled template: read failed", "file", name, "error", err)
			continue
		}
		tmpl, err := parseBundledTemplate(name, string(raw))
		if err != nil {
			slog.Warn("bundled template: skipped", "file", name, "error", err)
			continue
		}
		templates = append(templates, tmpl)
	}

	sort.Slice(templates, func(i, j int) bool { return templates[i].Key < templates[j].Key })
	return templates, nil
}

// parseBundledTemplate splits a template file into its leading `# key: value`
// comment header and the YAML body. The header is read line-by-line until the
// first non-comment line, so trailing comment blocks inside the body (e.g.
// "# IMPORTANT:" notes) stay in the compose text rather than being mistaken
// for metadata. The body is validated with gopkg.in/yaml.v3 before use.
func parseBundledTemplate(fileName, content string) (BundledTemplate, error) {
	key := strings.TrimSuffix(fileName, filepath.Ext(fileName))
	lines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")

	meta := map[string]string{}
	idx := 0
	for idx < len(lines) {
		line := strings.TrimSpace(lines[idx])
		if !strings.HasPrefix(line, "#") {
			break
		}
		if k, v, ok := parseHeaderComment(line); ok {
			meta[k] = v
		}
		idx++
	}
	// Skip the blank separator between the header block and the compose body.
	for idx < len(lines) && strings.TrimSpace(lines[idx]) == "" {
		idx++
	}

	body := strings.TrimSpace(strings.Join(lines[idx:], "\n"))
	if body == "" {
		return BundledTemplate{}, fmt.Errorf("empty compose body")
	}
	var doc map[string]any
	if err := yaml.Unmarshal([]byte(body), &doc); err != nil {
		return BundledTemplate{}, fmt.Errorf("invalid compose yaml: %w", err)
	}
	if _, ok := doc["services"]; !ok {
		return BundledTemplate{}, fmt.Errorf("compose body has no 'services' key")
	}

	slogan := meta["slogan"]
	description := slogan
	if description == "" {
		description = meta["documentation"]
	}

	return BundledTemplate{
		Key:         key,
		Name:        humanizeTemplateKey(key),
		Slogan:      slogan,
		Description: description,
		Category:    meta["category"],
		Tags:        parseTagList(meta["tags"]),
		Logo:        meta["logo"],
		Port:        atoiSafe(meta["port"]),
		ComposeYAML: body,
	}, nil
}

// parseHeaderComment parses a single leading comment line of the form
// "# key: value" (Coolify trims the leading "#" then the first colon splits the
// key from the value, so values may themselves contain colons, e.g. a docs
// URL). Lines that are not a "key: value" comment are reported as not-matched.
func parseHeaderComment(line string) (string, string, bool) {
	trimmed := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "#"))
	if trimmed == "" {
		return "", "", false
	}
	k, v, ok := strings.Cut(trimmed, ":")
	if !ok {
		return "", "", false
	}
	return strings.ToLower(strings.TrimSpace(k)), strings.TrimSpace(v), true
}

// parseTagList splits a comma-separated tag header into a trimmed, non-empty
// string slice. Tags may contain spaces ("no code"), which are preserved.
func parseTagList(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	raw := strings.Split(s, ",")
	tags := make([]string, 0, len(raw))
	for _, t := range raw {
		if t = strings.TrimSpace(t); t != "" {
			tags = append(tags, t)
		}
	}
	return tags
}

// humanizeTemplateKey derives a display name from a file-name key, e.g.
// "bookstack" -> "Bookstack" and "minio-community-edition" -> "Minio Community
// Edition". Coolify headers carry no explicit name, so the key is the source.
func humanizeTemplateKey(key string) string {
	parts := strings.FieldsFunc(key, func(r rune) bool {
		return r == '-' || r == '_' || r == '.' || r == ' '
	})
	if len(parts) == 0 {
		return key
	}
	for i, p := range parts {
		parts[i] = capitalizeFirst(p)
	}
	return strings.Join(parts, " ")
}

func capitalizeFirst(s string) string {
	for _, r := range s {
		// Slicing by the first rune's byte width, not by 1, so a multi-byte
		// leading character (e.g. "éclair") is not split mid-rune.
		first := string(r)
		return strings.ToUpper(first) + s[len(first):]
	}
	return s
}

func atoiSafe(s string) int {
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return 0
	}
	return n
}
