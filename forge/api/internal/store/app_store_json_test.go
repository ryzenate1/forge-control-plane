package store

import (
	"encoding/json"
	"testing"
)

func TestAppStoreParamsAreJSONObjects(t *testing.T) {
	for name, value := range map[string]any{
		"catalog":      AppStoreApp{Params: json.RawMessage(`{"PORT":{"label":"Port","type":"number","default":8080}}`)},
		"installation": AppStoreInstall{Params: json.RawMessage(`{"PORT":"8080"}`)},
	} {
		t.Run(name, func(t *testing.T) {
			body, err := json.Marshal(value)
			if err != nil {
				t.Fatal(err)
			}
			var decoded struct {
				Params map[string]json.RawMessage `json:"params"`
			}
			if err := json.Unmarshal(body, &decoded); err != nil {
				t.Fatalf("params must be an object, not base64: %s: %v", body, err)
			}
			if _, ok := decoded.Params["PORT"]; !ok {
				t.Fatalf("missing PORT: %s", body)
			}
		})
	}
}
