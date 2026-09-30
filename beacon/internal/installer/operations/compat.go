package operations

import (
	"encoding/json"
	"fmt"
)

// compat aliases: Puffer-style lowercase names -> canonical Go operation names.
// This file runs in init after all operation packages have registered their factories
// (Go init order is per-package; since this file is in the same package as registry,
// its init runs after registry but before external packages? To ensure delegation works
// at execution time we look up factories lazily inside the wrapper.)

func init() {
	aliasMap := map[string]string{
		"download":  "downloadFile",
		"writefile": "writeFile",
		"command":   "runCommand",
		"fabricdl":  "fabricDl",
		"paperdl":   "paperDl",
		"forgedl":   "forgeDl",
		// mojangdl already registers both cases in its own package
		// "extract" is provided directly by extract package; no alias needed
	}
	for alias, canonical := range aliasMap {
		target := canonical
		aliasName := alias
		// capture loop vars
		func(a, c string) {
			Register(a, func(args json.RawMessage) (Operation, error) {
				// Special handling for "download" Puffer style where args may be {"files":["url"]} instead of {"url":"..."}
				if a == "download" {
					args = normalizeDownloadArgs(args)
				}
				if a == "command" {
					args = normalizeCommandArgs(args)
				}
				if a == "writefile" {
					args = normalizeWriteFileArgs(args)
				}
				f, ok := GetFactory(c)
				if !ok {
					return nil, fmt.Errorf("compat alias %q: canonical %q not registered", a, c)
				}
				return f(args)
			})
		}(aliasName, target)
	}
}

func normalizeDownloadArgs(raw json.RawMessage) json.RawMessage {
	// Accept both Puffer "files": ["url"] and Forge {"url":"...","dest":"..."}.
	// If "files" present, synthesize url/dest from first entry.
	var probe map[string]json.RawMessage
	if err := json.Unmarshal(raw, &probe); err != nil {
		return raw
	}
	if _, hasFiles := probe["files"]; hasFiles {
		var files struct {
			Files []string `json:"files"`
			Dest  string   `json:"dest"`
			URL   string   `json:"url"`
		}
		if err := json.Unmarshal(raw, &files); err == nil && len(files.Files) > 0 {
			if files.URL == "" {
				files.URL = files.Files[0]
			}
			if files.Dest == "" {
				files.Dest = "downloaded.file"
			}
			// Preserve expectedSha256 if present in original.
			var orig map[string]interface{}
			_ = json.Unmarshal(raw, &orig)
			out := map[string]interface{}{
				"url":  files.URL,
				"dest": files.Dest,
			}
			if v, ok := orig["expectedSha256"]; ok {
				out["expectedSha256"] = v
			}
			if v, ok := orig["expectedSHA256"]; ok {
				out["expectedSha256"] = v
			}
			if v, ok := orig["maxBytes"]; ok {
				out["maxBytes"] = v
			}
			b, _ := json.Marshal(out)
			return b
		}
	}
	return raw
}

func normalizeCommandArgs(raw json.RawMessage) json.RawMessage {
	// Puffer "command": {"commands": "echo hi"} or {"commands": ["cmd","arg"]} vs Forge {"command":"echo","args":["hi"]}
	var probe map[string]json.RawMessage
	if err := json.Unmarshal(raw, &probe); err != nil {
		return raw
	}
	if _, hasCommands := probe["commands"]; hasCommands {
		var src struct {
			Commands interface{} `json:"commands"`
		}
		if err := json.Unmarshal(raw, &src); err == nil {
			switch v := src.Commands.(type) {
			case string:
				parts := splitCommand(v)
				if len(parts) == 0 {
					return raw
				}
				out := map[string]interface{}{"command": parts[0]}
				if len(parts) > 1 {
					out["args"] = parts[1:]
				}
				b, _ := json.Marshal(out)
				return b
			case []interface{}:
				var cmds []string
				for _, e := range v {
					if s, ok := e.(string); ok {
						cmds = append(cmds, s)
					}
				}
				if len(cmds) == 0 {
					return raw
				}
				// Puffer may have single string with spaces or array of full commands.
				// Take first as command, rest as args; if array holds multiple commands, join with &&.
				if len(cmds) == 1 {
					parts := splitCommand(cmds[0])
					if len(parts) == 0 {
						return raw
					}
					out := map[string]interface{}{"command": parts[0]}
					if len(parts) > 1 {
						out["args"] = parts[1:]
					}
					b, _ := json.Marshal(out)
					return b
				}
				out := map[string]interface{}{"command": cmds[0], "args": cmds[1:]}
				b, _ := json.Marshal(out)
				return b
			}
		}
	}
	return raw
}

func normalizeWriteFileArgs(raw json.RawMessage) json.RawMessage {
	// Puffer writefile: {"target":"path","text":"content"} vs Forge {"dest":"path","content":"..."}
	var probe map[string]json.RawMessage
	if err := json.Unmarshal(raw, &probe); err != nil {
		return raw
	}
	if _, hasTarget := probe["target"]; hasTarget {
		var src map[string]interface{}
		_ = json.Unmarshal(raw, &src)
		out := map[string]interface{}{}
		if v, ok := src["target"]; ok {
			out["dest"] = v
		}
		if v, ok := src["dest"]; ok {
			out["dest"] = v
		}
		if v, ok := src["text"]; ok {
			out["content"] = v
		}
		if v, ok := src["content"]; ok {
			out["content"] = v
		}
		if v, ok := src["mode"]; ok {
			out["mode"] = v
		}
		b, _ := json.Marshal(out)
		return b
	}
	return raw
}

func splitCommand(s string) []string {
	// Very small shell-like split: split on spaces, respecting quoted segments.
	// Enough for "echo hello" -> ["echo","hello"]; complex cases fall back to ["sh","-c",s].
	// To keep allowlist safety, we avoid sh -c trick; instead split naively.
	var res []string
	var cur string
	inQuote := false
	var quoteChar rune
	for _, r := range s {
		switch {
		case inQuote:
			if r == quoteChar {
				inQuote = false
			} else {
				cur += string(r)
			}
		case r == '"' || r == '\'':
			inQuote = true
			quoteChar = r
		case r == ' ' || r == '\t':
			if cur != "" {
				res = append(res, cur)
				cur = ""
			}
		default:
			cur += string(r)
		}
	}
	if cur != "" {
		res = append(res, cur)
	}
	return res
}
