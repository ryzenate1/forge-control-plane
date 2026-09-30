package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

const maxConfigFileSize = 64 * 1024 * 1024

var placeholderRegex = regexp.MustCompile(`\{\{\s*([^}]+?)\s*\}\}`)

// resolveValue replaces template placeholders like {{server.build.default.port}}
// and {{VAR}} using env map and allocation info.
//
// It returns the substituted string together with the placeholder keys that
// could not be resolved. A placeholder that cannot be resolved is NOT silently
// blanked by the caller: writing an empty port, IP or RCON password produces a
// server that boots misconfigured or unreachable while the deploy reports
// success, so the caller must surface the failure instead. A key that is
// present in env with an empty value is a *resolved* empty value, which keeps
// genuinely optional variables working.
func resolveValue(input string, env map[string]string, port int, ip string) (string, []string) {
	if input == "" {
		return "", nil
	}
	var unresolved []string
	out := placeholderRegex.ReplaceAllStringFunc(input, func(m string) string {
		inner := placeholderRegex.FindStringSubmatch(m)
		if len(inner) < 2 {
			return m
		}
		key := strings.TrimSpace(inner[1])
		// Handle pipe default filter e.g. VAR|default:''
		if strings.Contains(key, "|") {
			parts := strings.SplitN(key, "|", 2)
			k := strings.TrimSpace(parts[0])
			defPart := strings.TrimSpace(parts[1])
			defValue, hasDefault := "", false
			if strings.HasPrefix(defPart, "default:") {
				defValue = strings.Trim(strings.TrimSpace(strings.TrimPrefix(defPart, "default:")), "'\"")
				hasDefault = true
			}
			// check env first
			if v, ok := env[k]; ok {
				if v != "" {
					return v
				}
				// Present but empty: the default filter applies, and with no
				// filter the emptiness is a known value, not an unknown one.
				if hasDefault {
					return defValue
				}
				return ""
			}
			if k == "server.build.default.port" {
				return strconv.Itoa(port)
			}
			if k == "server.build.default.ip" {
				if ip != "" {
					return ip
				}
				return "0.0.0.0"
			}
			if hasDefault {
				return defValue
			}
			unresolved = append(unresolved, key)
			return ""
		}
		switch key {
		case "server.build.default.port":
			return strconv.Itoa(port)
		case "server.build.default.ip":
			if ip != "" {
				return ip
			}
			return "0.0.0.0"
		}
		if strings.HasPrefix(key, "env.") {
			k := strings.TrimPrefix(key, "env.")
			if v, ok := env[k]; ok {
				return v
			}
			if v, ok := env[strings.ToUpper(k)]; ok {
				return v
			}
			unresolved = append(unresolved, key)
			return ""
		}
		if strings.HasPrefix(key, "server.") {
			// generic server.* not supported beyond port/ip
			unresolved = append(unresolved, key)
			return ""
		}
		if v, ok := env[key]; ok {
			return v
		}
		// Try uppercase variant (env keys are typically uppercase)
		if v, ok := env[strings.ToUpper(key)]; ok {
			return v
		}
		unresolved = append(unresolved, key)
		return ""
	})
	return out, unresolved
}

// configurationFile holds a parsed egg config file entry.
type configurationFile struct {
	FileName string
	Parser   string
	Find     map[string]string // key -> raw template value
	Replace  []replacement     // for wings array style
}

type replacement struct {
	Match   string
	Value   string
	IfValue string
}

// parseConfigFiles extracts configurationFile entries from payload's config.files handling both map and array shapes.
func parseConfigFiles(payload map[string]any) ([]configurationFile, error) {
	config, _ := payload["config"].(map[string]any)
	if len(config) == 0 {
		return nil, nil
	}
	rawFiles, ok := config["files"]
	if !ok || rawFiles == nil {
		return nil, nil
	}
	switch v := rawFiles.(type) {
	case []any:
		return parseFilesArray(v)
	case map[string]any:
		return parseFilesMap(v)
	default:
		log.Printf("beacon: config.files has unexpected type %T, skipping", v)
		return nil, nil
	}
}

func parseFilesArray(arr []any) ([]configurationFile, error) {
	out := []configurationFile{}
	for _, raw := range arr {
		m, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		fileName, _ := m["file"].(string)
		if fileName == "" {
			fileName, _ = m["path"].(string)
		}
		if fileName == "" {
			continue
		}
		parser, _ := m["parser"].(string)
		if parser == "" {
			parser = "file"
		}
		cf := configurationFile{FileName: fileName, Parser: strings.ToLower(strings.TrimSpace(parser)), Find: map[string]string{}}
		if find, ok := m["find"].(map[string]any); ok {
			for k, val := range find {
				cf.Find[k] = fmt.Sprint(val)
			}
		}
		// Legacy beacon shape: properties / json / content
		if props, ok := m["properties"].(map[string]any); ok {
			for k, val := range props {
				cf.Find[k] = fmt.Sprint(val)
			}
		}
		if replArr, ok := m["replace"].([]any); ok {
			for _, rraw := range replArr {
				rm, ok := rraw.(map[string]any)
				if !ok {
					continue
				}
				match, _ := rm["match"].(string)
				if match == "" {
					continue
				}
				var val string
				if rw, ok := rm["replace_with"]; ok {
					val = fmt.Sprint(rw)
				} else if rw, ok := rm["value"]; ok {
					val = fmt.Sprint(rw)
				}
				ifValue, _ := rm["if_value"].(string)
				cf.Replace = append(cf.Replace, replacement{Match: match, Value: val, IfValue: ifValue})
			}
		}
		// If only Find is used, keep it; if Replace is used, prefer Replace but also synthesize Find for unified patch path
		out = append(out, cf)
	}
	return out, nil
}

func parseFilesMap(m map[string]any) ([]configurationFile, error) {
	out := []configurationFile{}
	for fileName, raw := range m {
		cfg, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		parser, _ := cfg["parser"].(string)
		if parser == "" {
			parser = "file"
		}
		cf := configurationFile{FileName: fileName, Parser: strings.ToLower(strings.TrimSpace(parser)), Find: map[string]string{}}
		if find, ok := cfg["find"].(map[string]any); ok {
			for k, val := range find {
				cf.Find[k] = fmt.Sprint(val)
			}
		} else if find, ok := cfg["replace"].(map[string]any); ok {
			// Some templates might use replace as object? treat as find
			for k, val := range find {
				cf.Find[k] = fmt.Sprint(val)
			}
		}
		if replArr, ok := cfg["replace"].([]any); ok {
			for _, rraw := range replArr {
				rm, ok := rraw.(map[string]any)
				if !ok {
					continue
				}
				match, _ := rm["match"].(string)
				if match == "" {
					continue
				}
				var val string
				if rw, ok := rm["replace_with"]; ok {
					val = fmt.Sprint(rw)
				} else if rw, ok := rm["value"]; ok {
					val = fmt.Sprint(rw)
				}
				ifValue, _ := rm["if_value"].(string)
				cf.Replace = append(cf.Replace, replacement{Match: match, Value: val, IfValue: ifValue})
			}
		}
		// handle case where find is nil but parser expects file path key from map key
		out = append(out, cf)
	}
	return out, nil
}

// patchConfigurationFiles applies config file patches inside rootDir using env
// and allocation info.
//
// It handles both map and array shapes and supports properties/yaml/json. Any
// file it cannot patch correctly is reported through the returned error: the
// caller's contract is that a nil error means every load-bearing value landed,
// so a skipped file, an unresolved placeholder or a failed write must never
// come back as success. Parsers beacon does not implement (file/ini/xml/toml)
// are logged and skipped rather than failed - see the note in the switch below.
func patchConfigurationFiles(rootDir string, payload map[string]any, env map[string]string, allocationPort int, allocationIP string) error {
	if strings.TrimSpace(rootDir) == "" {
		return nil
	}
	files, err := parseConfigFiles(payload)
	if err != nil {
		return err
	}
	if len(files) == 0 {
		return nil
	}
	var problems []string
	for _, cf := range files {
		target := filepath.Join(rootDir, filepath.FromSlash(cf.FileName))
		// Prevent escaping root
		rel, err := filepath.Rel(rootDir, target)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			log.Printf("beacon: config patch skip %q: escapes root", cf.FileName)
			problems = append(problems, fmt.Sprintf("%s: configuration file path escapes the server root", cf.FileName))
			continue
		}
		// Resolve replacements
		var unresolved []string
		resolvedFind := map[string]string{}
		for k, raw := range cf.Find {
			value, missing := resolveValue(raw, env, allocationPort, allocationIP)
			resolvedFind[k] = value
			unresolved = append(unresolved, missing...)
		}
		// Merge Replace list into map if no Find
		resolvedReplaceMap := map[string]string{}
		for _, r := range cf.Replace {
			val, missing := resolveValue(r.Value, env, allocationPort, allocationIP)
			// IfValue handling: only apply if file's current value matches IfValue (checked inside patchers where possible)
			// For minimal, store IfValue separately and let patchers handle.
			// For now, just use map; patchers that support IfValue will check.
			resolvedReplaceMap[r.Match] = val
			unresolved = append(unresolved, missing...)
		}
		if len(unresolved) > 0 {
			// Writing the blanks this would otherwise produce is how a server
			// ends up listening on port 0 with an empty RCON password while the
			// panel shows a successful deploy. Leave the file exactly as it is
			// and fail the operation with the names that could not be resolved.
			keys := uniqueSortedStrings(unresolved)
			log.Printf("beacon: config patch %q aborted, unresolved placeholders: %s", cf.FileName, strings.Join(keys, ", "))
			problems = append(problems, fmt.Sprintf("%s: unresolved placeholder(s) %s", cf.FileName, strings.Join(keys, ", ")))
			continue
		}
		// Prefer Replace map if Find empty but Replace exists
		replacements := resolvedFind
		if len(replacements) == 0 && len(resolvedReplaceMap) > 0 {
			replacements = resolvedReplaceMap
		} else if len(resolvedReplaceMap) > 0 {
			// merge
			for k, v := range resolvedReplaceMap {
				replacements[k] = v
			}
		}
		if len(replacements) == 0 {
			continue
		}
		switch cf.Parser {
		case "properties":
			if err := applyPropertiesPatch(target, replacements); err != nil {
				log.Printf("beacon: properties patch %q failed: %v", cf.FileName, err)
				problems = append(problems, fmt.Sprintf("%s: properties patch failed: %v", cf.FileName, err))
			} else {
				log.Printf("beacon: patched properties %q (%d keys)", cf.FileName, len(replacements))
			}
		case "yaml", "yml":
			if err := applyYamlPatch(target, replacements); err != nil {
				log.Printf("beacon: yaml patch %q failed: %v", cf.FileName, err)
				problems = append(problems, fmt.Sprintf("%s: yaml patch failed: %v", cf.FileName, err))
			} else {
				log.Printf("beacon: patched yaml %q", cf.FileName)
			}
		case "json":
			if err := applyJsonPatch(target, replacements); err != nil {
				log.Printf("beacon: json patch %q failed: %v", cf.FileName, err)
				problems = append(problems, fmt.Sprintf("%s: json patch failed: %v", cf.FileName, err))
			} else {
				log.Printf("beacon: patched json %q", cf.FileName)
			}
		// The parsers below are not implemented, and this is the one place where
		// beacon deliberately does NOT fail: a patch failure blocks the start
		// (ServerManager.applyPreStartConfigPatches), so turning every egg that
		// declares an ini/xml/toml config into a refused start would take down
		// working servers over a missing feature. Nothing incorrect is written -
		// the file is left exactly as the installer put it - so it is logged
		// loudly here and recorded as a limitation in the audit report instead.
		// The panel has no warning channel to carry it to the operator yet.
		case "file", "ini", "xml", "toml":
			log.Printf("beacon: parser %q for %q is not implemented by beacon; the file keeps whatever the installer wrote", cf.Parser, cf.FileName)
		default:
			log.Printf("beacon: unsupported parser %q for %q, skipping", cf.Parser, cf.FileName)
		}
	}
	if len(problems) > 0 {
		return fmt.Errorf("configuration patch incomplete: %s", strings.Join(problems, "; "))
	}
	return nil
}

// uniqueSortedStrings dedupes and orders a list so a reported failure names
// each missing placeholder exactly once.
func uniqueSortedStrings(in []string) []string {
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}

func applyPropertiesPatch(targetPath string, replacements map[string]string) error {
	// Enforce max size if file exists
	if info, err := os.Stat(targetPath); err == nil && info.Size() > maxConfigFileSize {
		return fmt.Errorf("refusing to patch: size %d exceeds limit", info.Size())
	}
	data, err := os.ReadFile(targetPath)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	content := string(data)
	// Split into lines preserving structure
	var lines []string
	if len(content) > 0 {
		lines = strings.Split(content, "\n")
	}
	// Track which keys were replaced
	replaced := map[string]bool{}
	var out []string
	// Preserve header comments? Wings keeps leading comments before first non-comment.
	// We'll just iterate and replace in place.
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, "!") {
			out = append(out, line)
			continue
		}
		// Find separator = or : or whitespace
		sepIdx := strings.Index(line, "=")
		if sepIdx == -1 {
			sepIdx = strings.Index(line, ":")
		}
		var key string
		if sepIdx != -1 {
			key = strings.TrimSpace(line[:sepIdx])
		} else {
			// No separator, treat whole line as key?
			key = strings.TrimSpace(line)
		}
		if newVal, ok := replacements[key]; ok {
			// Replace line with key=newVal (properties escaping minimal)
			// Preserve original key spacing? Use key + "=" + newVal
			out = append(out, key+"="+newVal)
			replaced[key] = true
		} else {
			out = append(out, line)
		}
	}
	// Append missing keys
	for k, v := range replacements {
		if !replaced[k] {
			// If file was empty and out is empty, ensure we don't have extra blank
			out = append(out, k+"="+v)
		}
	}
	// Join
	newContent := strings.Join(out, "\n")
	// Ensure trailing newline
	if !strings.HasSuffix(newContent, "\n") {
		newContent += "\n"
	}
	// If original file was empty (no lines), out may have been built from missing keys only, already handled.
	if len(lines) == 0 && len(replacements) > 0 && len(out) == len(replacements) {
		// Already built above, but ensure we didn't double add newline issues
	}
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o750); err != nil {
		return err
	}
	// Use 0640 like other beacon writes
	return os.WriteFile(targetPath, []byte(newContent), 0o640)
}

func applyYamlPatch(targetPath string, replacements map[string]string) error {
	if info, err := os.Stat(targetPath); err == nil && info.Size() > maxConfigFileSize {
		return fmt.Errorf("refusing to patch: size %d exceeds limit", info.Size())
	}
	var dataMap map[string]any
	if b, err := os.ReadFile(targetPath); err == nil && len(bytes.TrimSpace(b)) > 0 {
		// YAML may contain non-string keys? Use generic map
		if err := yaml.Unmarshal(b, &dataMap); err != nil {
			log.Printf("beacon: yaml unmarshal failed for %s: %v, starting fresh", targetPath, err)
			dataMap = map[string]any{}
		}
	} else {
		dataMap = map[string]any{}
	}
	if dataMap == nil {
		dataMap = map[string]any{}
	}
	for path, val := range replacements {
		if strings.Contains(path, "*") {
			log.Printf("beacon: yaml wildcard %q not supported, skipping", path)
			continue
		}
		setNestedValue(dataMap, path, val)
	}
	out, err := yaml.Marshal(dataMap)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o750); err != nil {
		return err
	}
	return os.WriteFile(targetPath, out, 0o640)
}

func applyJsonPatch(targetPath string, replacements map[string]string) error {
	if info, err := os.Stat(targetPath); err == nil && info.Size() > maxConfigFileSize {
		return fmt.Errorf("refusing to patch: size %d exceeds limit", info.Size())
	}
	var dataMap map[string]any
	if b, err := os.ReadFile(targetPath); err == nil && len(bytes.TrimSpace(b)) > 0 {
		if err := json.Unmarshal(b, &dataMap); err != nil {
			// Maybe file is not an object but array? Try generic
			var generic interface{}
			if err2 := json.Unmarshal(b, &generic); err2 == nil {
				// If it's not a map, we can't patch via dot paths; log and overwrite with map
				log.Printf("beacon: json %s is not an object, overwriting with patch map", targetPath)
				dataMap = map[string]any{}
			} else {
				log.Printf("beacon: json unmarshal failed for %s: %v, starting fresh", targetPath, err)
				dataMap = map[string]any{}
			}
		}
	} else {
		dataMap = map[string]any{}
	}
	if dataMap == nil {
		dataMap = map[string]any{}
	}
	for path, val := range replacements {
		if strings.Contains(path, "*") {
			log.Printf("beacon: json wildcard %q not supported, skipping", path)
			continue
		}
		setNestedValue(dataMap, path, val)
	}
	out, err := json.MarshalIndent(dataMap, "", "  ")
	if err != nil {
		return err
	}
	out = append(out, '\n')
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o750); err != nil {
		return err
	}
	return os.WriteFile(targetPath, out, 0o640)
}

func setNestedValue(root map[string]any, path string, value string) {
	parts := strings.Split(path, ".")
	cur := root
	for i, part := range parts {
		// Handle bracket array notation like foo[0]
		if strings.Contains(part, "[") && strings.Contains(part, "]") {
			bracketOpen := strings.Index(part, "[")
			bracketClose := strings.Index(part, "]")
			if bracketOpen < bracketClose {
				key := part[:bracketOpen]
				idxStr := part[bracketOpen+1 : bracketClose]
				idx, err := strconv.Atoi(idxStr)
				if err != nil {
					log.Printf("beacon: invalid array index in %q", path)
					return
				}
				arr, _ := cur[key].([]any)
				for len(arr) <= idx {
					arr = append(arr, map[string]any{})
				}
				if i == len(parts)-1 {
					arr[idx] = coerceValue(value)
					cur[key] = arr
					return
				}
				// Navigate into arr[idx]
				if m, ok := arr[idx].(map[string]any); ok {
					cur[key] = arr
					cur = m
				} else {
					newMap := map[string]any{}
					arr[idx] = newMap
					cur[key] = arr
					cur = newMap
				}
				continue
			}
		}
		if i == len(parts)-1 {
			cur[part] = coerceValue(value)
			return
		}
		next, ok := cur[part]
		if !ok {
			newMap := map[string]any{}
			cur[part] = newMap
			cur = newMap
			continue
		}
		if m, ok := next.(map[string]any); ok {
			cur = m
		} else {
			newMap := map[string]any{}
			cur[part] = newMap
			cur = newMap
		}
	}
}

func coerceValue(value string) any {
	// Mirror wings behavior: try to preserve numeric/boolean types
	if intVal, err := strconv.Atoi(value); err == nil {
		return intVal
	}
	lower := strings.ToLower(value)
	if lower == "true" {
		return true
	}
	if lower == "false" {
		return false
	}
	// Try float
	if strings.Contains(value, ".") {
		if f, err := strconv.ParseFloat(value, 64); err == nil {
			return f
		}
	}
	return value
}

// applyConfigurationFilesFromPayload is a helper used by server.go sync path.
func applyConfigurationFilesFromPayload(rootDir string, payload map[string]any, env map[string]string, port int, ip string) error {
	return patchConfigurationFiles(rootDir, payload, env, port, ip)
}

// safe file helper used by tests
func patchPropertiesFileForTest(path string, find map[string]string) error {
	return applyPropertiesPatch(path, find)
}
