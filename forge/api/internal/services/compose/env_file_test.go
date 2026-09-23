package compose

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// NOTE: these tests used to pin the FORGE_ENV_FILE_STRICT gate driven by
// isEnvFileEmpty: strict mode rejected any env_file (string, list, or
// include-level) with "env_file not supported, inline env vars", non-strict
// mode emitted an env_file warning. The refactor deleted the gate, the
// helper, and every env_file diagnostic — rawService keeps an EnvFile field
// only so the key survives round-tripping. The behaviour that remains is
// pinned below: env_file is accepted and silently ignored in all forms.

const composeWithEnvFile = `
services:
  web:
    image: nginx:latest
    env_file: ./app.env
    environment:
      INLINE_VAR: value
`

const composeWithEnvFileList = `
services:
  web:
    image: nginx:latest
    env_file:
      - ./frontend.env
      - ./backend.env
  api:
    image: myapp:latest
    env_file: .env
`

const composeWithEnvFileInclude = `
include:
  - path: ./common.yml
    env_file: ./env/common.env
services:
  web:
    image: nginx:latest
`

func TestEnvFile_IsAcceptedAndIgnoredInAllForms(t *testing.T) {
	svc := &Service{}

	for _, tc := range []struct {
		name     string
		content  string
		services int
	}{
		{"string form", composeWithEnvFile, 1},
		{"list form", composeWithEnvFileList, 2},
		{"include form", composeWithEnvFileInclude, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			parsed, err := svc.Parse([]byte(tc.content), "/app")
			require.NoError(t, err, "env_file must no longer fail parsing")
			require.NotNil(t, parsed)
			assert.Len(t, parsed.Services, tc.services)

			result := svc.Validate([]byte(tc.content), "/app")
			assert.True(t, result.Valid, "env_file must not affect validity")
			for _, issue := range append(append([]ValidationError{}, result.Errors...), result.Warnings...) {
				assert.NotContains(t, strings.ToLower(issue.Field+issue.Message), "env_file",
					"the env_file diagnostic was removed, nothing should mention it")
			}
		})
	}
}

func TestEnvFile_StrictEnvVarHasNoEffect(t *testing.T) {
	svc := &Service{}

	for _, value := range []string{"true", "false", ""} {
		t.Run("FORGE_ENV_FILE_STRICT="+value, func(t *testing.T) {
			t.Setenv("FORGE_ENV_FILE_STRICT", value)
			_, err := svc.Parse([]byte(composeWithEnvFile), "/app")
			require.NoError(t, err)
			assert.True(t, svc.Validate([]byte(composeWithEnvFile), "/app").Valid)
		})
	}
}

func TestEnvFile_ComposeWithoutEnvFileStillValidates(t *testing.T) {
	svc := &Service{}
	content := `
services:
  web:
    image: nginx:latest
    environment:
      FOO: bar
`
	parsed, err := svc.Parse([]byte(content), "/app")
	require.NoError(t, err)
	assert.Len(t, parsed.Services, 1)
	result := svc.Validate([]byte(content), "/app")
	assert.True(t, result.Valid)
	assert.Empty(t, result.Errors)
}
