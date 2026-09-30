package daemon

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestBuildLogsParsesPlainTextAndEventStreams(t *testing.T) {
	tests := []struct {
		name        string
		contentType string
		body        string
		want        []string
	}{
		{
			name:        "completed build plain text",
			contentType: "text/plain",
			body:        "clone complete\nimage built\n",
			want:        []string{"clone complete", "image built"},
		},
		{
			name:        "running build event stream",
			contentType: "text/event-stream",
			body:        "data: compiling\n\ndata: complete\n\nevent: done\ndata: completed\n\n",
			want:        []string{"compiling", "complete", "completed"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", tt.contentType)
				_, _ = w.Write([]byte(tt.body))
			}))
			defer server.Close()

			client, err := NewClient(server.URL, "node-token")
			if err != nil {
				t.Fatal(err)
			}
			lines, err := client.BuildLogs(context.Background(), server.URL, "node-token", "build-1", false)
			if err != nil {
				t.Fatal(err)
			}
			if len(lines) != len(tt.want) {
				t.Fatalf("got %d log lines, want %d: %#v", len(lines), len(tt.want), lines)
			}
			for index := range tt.want {
				if lines[index].Line != tt.want[index] {
					t.Fatalf("line %d = %q, want %q", index, lines[index].Line, tt.want[index])
				}
			}
		})
	}
}
