package store

import (
	"testing"
)

func TestValidateNodeEndpoint(t *testing.T) {
	tests := []struct {
		name    string
		nameVal string
		baseURL string
		fqdn    string
		scheme  string
		memory  int
		disk    int
		upload  int
		listen  int
		sftp    int
		wantErr bool
	}{
		{
			name:    "valid",
			nameVal: "my-node",
			baseURL: "https://node.example.com:8080",
			fqdn:    "node.example.com",
			scheme:  "https",
			memory:  16384,
			disk:    102400,
			upload:  100,
			listen:  8080,
			sftp:    2022,
			wantErr: false,
		},
		{
			name:    "valid http scheme",
			nameVal: "my-node",
			baseURL: "http://node.example.com:8080",
			fqdn:    "node.example.com",
			scheme:  "http",
			memory:  16384,
			disk:    102400,
			upload:  100,
			listen:  8080,
			sftp:    2022,
			wantErr: false,
		},
		{
			name:    "missing name",
			nameVal: "",
			baseURL: "https://node.example.com:8080",
			fqdn:    "node.example.com",
			scheme:  "https",
			memory:  16384,
			disk:    102400,
			upload:  100,
			listen:  8080,
			sftp:    2022,
			wantErr: true,
		},
		{
			name:    "invalid scheme",
			nameVal: "my-node",
			baseURL: "ftp://node.example.com",
			fqdn:    "node.example.com",
			scheme:  "ftp",
			memory:  16384,
			disk:    102400,
			upload:  100,
			listen:  8080,
			sftp:    2022,
			wantErr: true,
		},
		{
			name:    "port range violation - listen too low",
			nameVal: "my-node",
			baseURL: "https://node.example.com:0",
			fqdn:    "node.example.com",
			scheme:  "https",
			memory:  16384,
			disk:    102400,
			upload:  100,
			listen:  0,
			sftp:    2022,
			wantErr: true,
		},
		{
			name:    "port range violation - listen too high",
			nameVal: "my-node",
			baseURL: "https://node.example.com:65536",
			fqdn:    "node.example.com",
			scheme:  "https",
			memory:  16384,
			disk:    102400,
			upload:  100,
			listen:  65536,
			sftp:    2022,
			wantErr: true,
		},
		{
			name:    "daemon-SFTP port collision",
			nameVal: "my-node",
			baseURL: "https://node.example.com:2022",
			fqdn:    "node.example.com",
			scheme:  "https",
			memory:  16384,
			disk:    102400,
			upload:  100,
			listen:  2022,
			sftp:    2022,
			wantErr: true,
		},
		{
			name:    "negative memory",
			nameVal: "my-node",
			baseURL: "https://node.example.com:8080",
			fqdn:    "node.example.com",
			scheme:  "https",
			memory:  -1,
			disk:    102400,
			upload:  100,
			listen:  8080,
			sftp:    2022,
			wantErr: true,
		},
		{
			name:    "negative disk",
			nameVal: "my-node",
			baseURL: "https://node.example.com:8080",
			fqdn:    "node.example.com",
			scheme:  "https",
			memory:  16384,
			disk:    -1,
			upload:  100,
			listen:  8080,
			sftp:    2022,
			wantErr: true,
		},
		{
			name:    "FQDN does not match base URL host",
			nameVal: "my-node",
			baseURL: "https://node.example.com:8080",
			fqdn:    "other.example.com",
			scheme:  "https",
			memory:  16384,
			disk:    102400,
			upload:  100,
			listen:  8080,
			sftp:    2022,
			wantErr: true,
		},
		{
			name:    "base URL has credentials",
			nameVal: "my-node",
			baseURL: "https://user:pass@node.example.com:8080",
			fqdn:    "node.example.com",
			scheme:  "https",
			memory:  16384,
			disk:    102400,
			upload:  100,
			listen:  8080,
			sftp:    2022,
			wantErr: true,
		},
		{
			name:    "SFTP port too high",
			nameVal: "my-node",
			baseURL: "https://node.example.com:8080",
			fqdn:    "node.example.com",
			scheme:  "https",
			memory:  16384,
			disk:    102400,
			upload:  100,
			listen:  8080,
			sftp:    65536,
			wantErr: true,
		},
		{
			name:    "name exceeds 100 characters",
			nameVal: "this-node-name-is-way-too-long-and-should-be-rejected-by-the-validation-logic-because-it-exceeds-one-hundred-characters",
			baseURL: "https://node.example.com:8080",
			fqdn:    "node.example.com",
			scheme:  "https",
			memory:  16384,
			disk:    102400,
			upload:  100,
			listen:  8080,
			sftp:    2022,
			wantErr: true,
		},
		{
			name:    "name contains unicode characters",
			nameVal: "nøde-1",
			baseURL: "https://node.example.com:8080",
			fqdn:    "node.example.com",
			scheme:  "https",
			memory:  16384,
			disk:    102400,
			upload:  100,
			listen:  8080,
			sftp:    2022,
			wantErr: true,
		},
		{
			name:    "FQDN is localhost",
			nameVal: "my-node",
			baseURL: "http://localhost:8080",
			fqdn:    "localhost",
			scheme:  "http",
			memory:  16384,
			disk:    102400,
			upload:  100,
			listen:  8080,
			sftp:    2022,
			wantErr: true,
		},
		{
			name:    "FQDN is 127.0.0.1",
			nameVal: "my-node",
			baseURL: "http://127.0.0.1:8080",
			fqdn:    "127.0.0.1",
			scheme:  "http",
			memory:  16384,
			disk:    102400,
			upload:  100,
			listen:  8080,
			sftp:    2022,
			wantErr: true,
		},
		{
			name:    "FQDN is 0.0.0.0",
			nameVal: "my-node",
			baseURL: "http://0.0.0.0:8080",
			fqdn:    "0.0.0.0",
			scheme:  "http",
			memory:  16384,
			disk:    102400,
			upload:  100,
			listen:  8080,
			sftp:    2022,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateNodeEndpoint(tt.nameVal, tt.baseURL, tt.fqdn, tt.scheme, tt.memory, tt.disk, tt.upload, tt.listen, tt.sftp)
			if tt.wantErr && err == nil {
				t.Fatal("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}
