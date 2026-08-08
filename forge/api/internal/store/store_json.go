package store

import "encoding/json"

// mustMarshalStringSlice returns a JSON-encoded []string, or an empty array
// if the input is empty. Panics on JSON errors (which can only happen for
// strings containing unescaped quotes etc — we never expect those from
// well-formed scope names).
func mustMarshalStringSlice(in []string) []byte {
	if in == nil {
		return []byte("[]")
	}
	b, _ := json.Marshal(in)
	return b
}

func unmarshalStringSlice(b []byte) []string {
	if len(b) == 0 {
		return []string{}
	}
	var out []string
	_ = json.Unmarshal(b, &out)
	return out
}
