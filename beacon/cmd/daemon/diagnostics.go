package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	goruntime "runtime"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/sys/unix"
	"golang.org/x/term"

	daemonhttp "gamepanel/beacon/internal/server"
)

type checkResult struct {
	name   string
	status string
	detail string
}

var diagnosticsCmd = &cobra.Command{
	Use:   "diagnostics",
	Short: "Run system diagnostics and connectivity checks",
	RunE:  diagnosticsCmdRun,
}

var (
	diagnosticsTLSHostname string
	diagnosticsHastebinURL string
	diagnosticsLogLines    int
	diagnosticsUpload      bool
)

func init() {
	diagnosticsCmd.Flags().StringVar(&diagnosticsTLSHostname, "tls-hostname", "", "Test TLS connectivity to hostname")
	diagnosticsCmd.Flags().StringVar(&diagnosticsHastebinURL, "hastebin-url", "https://logs.pelican.dev", "URL of the hastebin instance to use")
	diagnosticsCmd.Flags().IntVar(&diagnosticsLogLines, "log-lines", 200, "Number of log lines to include in the report")
	diagnosticsCmd.Flags().BoolVar(&diagnosticsUpload, "upload", false, "Automatically upload report to hastebin without asking")
}

func diagnosticsCmdRun(_ *cobra.Command, _ []string) error {
	var reportText strings.Builder
	results := []checkResult{}

	// 1. System info
	hostname, _ := os.Hostname()
	results = append(results, checkResult{
		name:   "System Info",
		status: "INFO",
		detail: fmt.Sprintf("%s | %s/%s | Go %s | %d CPUs",
			hostname, goruntime.GOOS, goruntime.GOARCH, goruntime.Version(), goruntime.NumCPU()),
	})

	// 2. OS check
	if goruntime.GOOS != "linux" {
		results = append(results, checkResult{"OS", "WARN", "Expected linux, got " + goruntime.GOOS})
	} else {
		results = append(results, checkResult{"OS", "PASS", goruntime.GOOS})
	}

	// 3. Docker binary
	dockerPath, _ := exec.LookPath("docker")
	if dockerPath != "" {
		results = append(results, checkResult{"Docker binary", "PASS", dockerPath})
	} else {
		results = append(results, checkResult{"Docker binary", "FAIL", "not found in PATH"})
	}

	// 4. Docker socket
	if _, err := os.Stat("/var/run/docker.sock"); err == nil {
		results = append(results, checkResult{"Docker socket", "PASS", "/var/run/docker.sock"})
	} else {
		results = append(results, checkResult{"Docker socket", "WARN", "/var/run/docker.sock not accessible: " + err.Error()})
	}

	// 5. Disk space
	dataDir := os.Getenv("DAEMON_DATA_DIR")
	if dataDir == "" {
		dataDir = "/srv/game-panel/servers"
	}
	var stat unix.Statfs_t
	if err := unix.Statfs(dataDir, &stat); err == nil {
		available := stat.Bavail * uint64(stat.Bsize) / (1024 * 1024 * 1024)
		total := stat.Blocks * uint64(stat.Bsize) / (1024 * 1024 * 1024)
		status := "PASS"
		if available < 10 {
			status = "WARN"
		}
		if available < 1 {
			status = "FAIL"
		}
		results = append(results, checkResult{name: "Disk space", status: status, detail: fmt.Sprintf("%s: %d GB available / %d GB total", dataDir, available, total)})
	} else {
		results = append(results, checkResult{"Disk space", "WARN", fmt.Sprintf("cannot stat %s: %v", dataDir, err)})
	}

	// 6. Memory
	totalRAM := readMemoryMB()
	status := "PASS"
	if totalRAM <= 0 {
		status = "WARN"
	} else if totalRAM < 512 {
		status = "FAIL"
	} else if totalRAM < 1024 {
		status = "WARN"
	}
	results = append(results, checkResult{"System memory", status, fmt.Sprintf("%d MB", totalRAM)})

	// 7. Network connectivity
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	networkClient := &http.Client{Timeout: 5 * time.Second}
	networkReq, _ := http.NewRequestWithContext(ctx, http.MethodHead, "https://api.github.com", nil)
	if response, err := networkClient.Do(networkReq); err == nil {
		_ = response.Body.Close()
		results = append(results, checkResult{"Internet", "PASS", "HTTPS connectivity available"})
	} else {
		results = append(results, checkResult{"Internet", "WARN", "HTTPS connectivity check failed: " + err.Error()})
	}

	// 8. DNS resolution
	if ips, err := net.DefaultResolver.LookupHost(ctx, "api.github.com"); err == nil && len(ips) > 0 {
		results = append(results, checkResult{"DNS", "PASS", fmt.Sprintf("api.github.com → %s", ips[0])})
	} else {
		results = append(results, checkResult{"DNS", "FAIL", fmt.Sprintf("cannot resolve api.github.com: %v", err)})
	}

	// 9. Panel connectivity (if configured). The probe reuses the daemon API's
	// hardened diagnostics client: HTTPS-or-loopback policy, restricted-address
	// rejection with dial pinning, a five-second timeout, and no credentials.
	for _, env := range []string{"PANEL_API_URL", "WINGS_PANEL_URL"} {
		panelURL := os.Getenv(env)
		if panelURL == "" {
			continue
		}
		status, err := daemonhttp.ProbePanelHealth(ctx, panelURL)
		if err != nil {
			results = append(results, checkResult{"Panel API", "FAIL", err.Error()})
		} else {
			results = append(results, checkResult{"Panel API", "PASS", fmt.Sprintf("%s/api/v1/health → %d", strings.TrimRight(panelURL, "/"), status)})
		}
		break
	}

	// 10. TLS connectivity (if --tls-hostname)
	if diagnosticsTLSHostname != "" {
		conn, err := tls.DialWithDialer(&net.Dialer{Timeout: 5 * time.Second}, "tcp", diagnosticsTLSHostname+":443", &tls.Config{
			MinVersion: tls.VersionTLS12,
			ServerName: diagnosticsTLSHostname,
		})
		if err == nil {
			conn.Close()
			results = append(results, checkResult{"TLS", "PASS", fmt.Sprintf("%s:443 TLS handshake OK", diagnosticsTLSHostname)})
		} else {
			results = append(results, checkResult{"TLS", "FAIL", err.Error()})
		}
	}

	// 11. systemd
	if _, err := os.Stat("/run/systemd/system"); err == nil {
		results = append(results, checkResult{"systemd", "PASS", "Running under systemd"})
	} else {
		results = append(results, checkResult{"systemd", "INFO", "Not running under systemd (expected in Docker)"})
	}

	// Print results
	reportWriter := io.MultiWriter(os.Stdout, &reportText)

	fmt.Fprintln(reportWriter, "\n=== Beacon Diagnostics ===")
	fmt.Fprintln(reportWriter)
	hasFatal := false
	for _, r := range results {
		switch r.status {
		case "PASS":
			fmt.Fprintf(reportWriter, "  %s %s: %s\n", green("✓"), r.name, r.detail)
		case "INFO":
			fmt.Fprintf(reportWriter, "  %s %s: %s\n", blue("ℹ"), r.name, r.detail)
		case "WARN":
			fmt.Fprintf(reportWriter, "  %s %s: %s\n", yellow("⚠"), r.name, r.detail)
		case "FAIL":
			fmt.Fprintf(reportWriter, "  %s %s: %s\n", red("✗"), r.name, r.detail)
			hasFatal = true
		}
	}
	fmt.Fprintln(reportWriter)

	if hasFatal {
		fmt.Fprintln(reportWriter, red("✗ Some checks failed. Resolve issues before starting Beacon."))
	}

	// Upload handling
	shouldUpload := diagnosticsUpload
	if !shouldUpload {
		fmt.Print("Upload report to " + diagnosticsHastebinURL + "? [y/N]: ")
		var response string
		if _, err := fmt.Scanln(&response); err != nil && !errors.Is(err, io.EOF) {
			return fmt.Errorf("read upload choice: %w", err)
		}
		shouldUpload = response == "y" || response == "Y"
	}

	if shouldUpload {
		pasteURL, err := uploadToHastebin(diagnosticsHastebinURL, reportText.String())
		if err != nil {
			return fmt.Errorf("upload diagnostics report: %w", err)
		}
		fmt.Println("Your report is available here:", pasteURL)
	}

	if hasFatal {
		return errors.New("one or more diagnostic checks failed")
	}
	fmt.Println(green("✓ All checks passed!"))
	return nil
}

func uploadToHastebin(hbUrl, content string) (string, error) {
	u, err := url.Parse(hbUrl)
	if err != nil {
		return "", err
	}
	if u.Scheme != "https" || u.Hostname() == "" || u.User != nil {
		return "", errors.New("hastebin URL must be credential-free HTTPS")
	}

	formData := new(bytes.Buffer)
	formWriter := multipart.NewWriter(formData)
	if err := formWriter.WriteField("c", content); err != nil {
		return "", err
	}
	if err := formWriter.WriteField("e", "14d"); err != nil {
		return "", err
	}
	if err := formWriter.Close(); err != nil {
		return "", err
	}

	req, err := http.NewRequest(http.MethodPost, u.String(), formData)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", formWriter.FormDataContentType())
	res, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()

	if res.StatusCode != 200 {
		return "", fmt.Errorf("unexpected status code: %d", res.StatusCode)
	}

	pres := make(map[string]interface{})
	body, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if err != nil {
		return "", err
	}

	if err := json.Unmarshal(body, &pres); err != nil {
		return "", err
	}
	if pasteUrl, ok := pres["url"].(string); ok {
		return pasteUrl, nil
	}
	return "", errors.New("failed to find key in response")
}

func green(s string) string  { return terminalColor("\033[32m", s) }
func red(s string) string    { return terminalColor("\033[31m", s) }
func yellow(s string) string { return terminalColor("\033[33m", s) }
func blue(s string) string   { return terminalColor("\033[34m", s) }

func terminalColor(color, value string) string {
	if goruntime.GOOS == "windows" || os.Getenv("NO_COLOR") != "" || !term.IsTerminal(int(os.Stdout.Fd())) {
		return value
	}
	return color + value + "\033[0m"
}
