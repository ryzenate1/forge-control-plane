package store

import (
	"testing"
)

func TestValidateVariableValue_RegexSlash(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		rules   string
		wantErr bool
	}{
		// PTDL import: minecraft-paper.json:58
		{name: "paper jar valid", value: "server.jar", rules: "required|regex:/^([\\w\\d._-]+)(\\.jar)$/", wantErr: false},
		{name: "paper jar invalid no jar", value: "server.txt", rules: "required|regex:/^([\\w\\d._-]+)(\\.jar)$/", wantErr: true},
		{name: "paper jar required empty", value: "", rules: "required|regex:/^([\\w\\d._-]+)(\\.jar)$/", wantErr: true},
		{name: "paper jar nullable empty", value: "", rules: "nullable|regex:/^([\\w\\d._-]+)(\\.jar)$/", wantErr: false},

		// Slash delimiters stripped: /pattern/ and /pattern/flags
		{name: "slash delimiters simple", value: "hello", rules: "regex:/^hello$/", wantErr: false},
		{name: "slash delimiters case-insensitive flag", value: "Hello", rules: "regex:/^hello$/i", wantErr: false},
		{name: "slash delimiters case-sensitive fails", value: "Hello", rules: "regex:/^hello$/", wantErr: true},
		{name: "slash delimiters case-insensitive flag upper", value: "HELLO", rules: "regex:/^hello$/i", wantErr: false},

		// Alternation inside regex: | should NOT split rule
		{name: "regex alternation valid foo", value: "foo", rules: "required|regex:/^(foo|bar)$/", wantErr: false},
		{name: "regex alternation valid bar", value: "bar", rules: "required|regex:/^(foo|bar)$/", wantErr: false},
		{name: "regex alternation invalid baz", value: "baz", rules: "required|regex:/^(foo|bar)$/", wantErr: true},
		{name: "regex alternation with outer string rule", value: "foo", rules: "required|regex:/^(foo|bar)$/|string", wantErr: false},

		// Char class with | inside: should not split
		{name: "char class pipe valid a", value: "a", rules: "regex:/^[a|b]+$/", wantErr: false},
		{name: "char class pipe valid b", value: "b", rules: "regex:/^[a|b]+$/", wantErr: false},
		{name: "char class pipe valid pipe char", value: "|", rules: "regex:/^[a|b]+$/", wantErr: false},
		{name: "char class pipe invalid c", value: "c", rules: "regex:/^[a|b]+$/", wantErr: true},

		// Multiple regex with flags and char class
		{name: "palworld decimal regex valid", value: "1.000000", rules: "required|regex:/^\\d+\\.\\d+$/", wantErr: false},
		{name: "palworld decimal regex invalid", value: "abc", rules: "required|regex:/^\\d+\\.\\d+$/", wantErr: true},

		// Ensure plain string without slash still works (no delimiters)
		{name: "regex without slashes", value: "abc", rules: "regex:^[a-z]+$", wantErr: false},
		{name: "regex without slashes fail", value: "123", rules: "regex:^[a-z]+$", wantErr: true},

		// Validate that strings.Split bug is fixed: rule with regex containing '|' followed by another rule
		{name: "regex pipe plus in rule not split", value: "foo", rules: "required|regex:/^(foo|bar)$/|max:10", wantErr: false},
		{name: "regex pipe plus in rule with max fail", value: "toolongvalue123", rules: "required|regex:/^(foo|bar)$/|max:5", wantErr: true}, // passes regex? actually toolongvalue != foo|bar so fails regex first

		// Integer handling (additive, ensure existing eggs still work)
		{name: "integer valid", value: "20", rules: "required|integer|min:1|max:100", wantErr: false},
		{name: "integer invalid not number", value: "abc", rules: "required|integer|min:1|max:100", wantErr: true},
		{name: "integer min fail", value: "0", rules: "required|integer|min:1|max:100", wantErr: true},
		{name: "integer max fail", value: "101", rules: "required|integer|min:1|max:100", wantErr: true},

		// Normal string max/min not affected
		{name: "string max ok", value: "hello", rules: "required|string|max:10", wantErr: false},
		{name: "string max fail", value: "toolongstringhere", rules: "required|string|max:5", wantErr: true},
		{name: "in rule valid", value: "easy", rules: "required|string|in:easy,normal,hard,peaceful", wantErr: false},
		{name: "in rule invalid", value: "invalid", rules: "required|string|in:easy,normal,hard,peaceful", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateVariableValue(tt.value, tt.rules)
			if (err != nil) != tt.wantErr {
				t.Fatalf("validateVariableValue(%q, %q) error = %v, wantErr %v", tt.value, tt.rules, err, tt.wantErr)
			}
		})
	}
}

func TestSplitValidationRules_NoSplitInsideRegex(t *testing.T) {
	tests := []struct {
		rules string
		want  int
	}{
		{rules: "required|regex:/^([\\w\\d._-]+)(\\.jar)$/", want: 2},
		{rules: "required|regex:/^(foo|bar)$/|string", want: 3},
		{rules: "regex:/^[a|b]+$/|required", want: 2},
		{rules: "required|regex:/^\\d+\\.\\d+$/|nullable", want: 3},
	}
	for _, tt := range tests {
		got := splitValidationRules(tt.rules)
		if len(got) != tt.want {
			t.Fatalf("splitValidationRules(%q) = %q (len %d), want len %d", tt.rules, got, len(got), tt.want)
		}
	}
}

func TestValidateVariableValue_PaperImport(t *testing.T) {
	// Simulate minecraft-paper.json:58 SERVER_JARFILE import
	rules := "required|regex:/^([\\w\\d._-]+)(\\.jar)$/"
	if err := validateVariableValue("server.jar", rules); err != nil {
		t.Fatalf("expected server.jar to pass PTDL regex, got %v", err)
	}
	if err := validateVariableValue("my-server_123.jar", rules); err != nil {
		t.Fatalf("expected my-server_123.jar to pass, got %v", err)
	}
	if err := validateVariableValue("server", rules); err == nil {
		t.Fatalf("expected server (no .jar) to fail")
	}
	// Directly test stripped inner: ensure slash-less version also passes
	if err := validateVariableValue("server.jar", "regex:^[\\w\\d._-]+\\.jar$"); err != nil {
		t.Fatalf("slash-less regex should also pass: %v", err)
	}
}

func TestSeedGameTemplates_CountAndValidation(t *testing.T) {
	templates, err := loadGameTemplates()
	if err != nil {
		t.Fatalf("loadGameTemplates: %v", err)
	}
	if len(templates) != 14 {
		t.Fatalf("expected 14 game templates, got %d", len(templates))
	}
	// Ensure each template name is unique and each variable passes the fixed validator.
	seen := map[string]bool{}
	for _, tpl := range templates {
		if seen[tpl.Name] {
			t.Fatalf("duplicate template name %q", tpl.Name)
		}
		seen[tpl.Name] = true
		if tpl.Name == "" || tpl.Image == "" || tpl.Startup == "" {
			t.Fatalf("template %q missing required fields", tpl.ID)
		}
		for _, v := range tpl.Env {
			if err := validateVariableValue(v.DefaultValue, v.Rules); err != nil {
				// Same exemption as the seed path: empty secret variables
				// ship no shared default; the value is supplied at server
				// creation (see secretSeedVar).
				if v.DefaultValue != "" || !secretSeedVar(v.EnvVariable) {
					t.Fatalf("template %q variable %q default %q rules %q failed validation: %v", tpl.Name, v.EnvVariable, v.DefaultValue, v.Rules, err)
				}
			}
		}
	}
	// Specific regression for GH-14: minecraft-paper and palworld regex must pass.
	for _, tpl := range templates {
		for _, v := range tpl.Env {
			if v.Rules == "required|regex:/^([\\w\\d._-]+)(\\.jar)$/" {
				if err := validateVariableValue("server.jar", v.Rules); err != nil {
					t.Fatalf("regex slash bug not fixed for %s %s: %v", tpl.Name, v.EnvVariable, err)
				}
			}
			if v.Rules == "required|regex:/^\\d+\\.\\d+$/" {
				if err := validateVariableValue("1.000000", v.Rules); err != nil {
					t.Fatalf("palworld regex failed for %s: %v", tpl.Name, err)
				}
			}
		}
	}
}

func TestDefaultSeeder_IncludesGameTemplates(t *testing.T) {
	// Verify DefaultSeeder registers the game-templates seeder (TMPL-01).
	s := DefaultSeeder(nil)
	found := false
	for _, e := range s.entries {
		if e.Name == "game-templates" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("DefaultSeeder missing game-templates entry; entries: %v", func() []string {
			var names []string
			for _, e := range s.entries {
				names = append(names, e.Name)
			}
			return names
		}())
	}
	if len(s.entries) < 3 {
		t.Fatalf("expected at least 3 seeder entries (roles, settings, game-templates), got %d", len(s.entries))
	}
}
