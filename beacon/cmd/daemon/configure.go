package main

import (
	"bufio"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/term"
	"gopkg.in/yaml.v3"

	"gamepanel/beacon/config"
)

var (
	colorReset  = configureColor("\033[0m")
	colorRed    = configureColor("\033[31m")
	colorGreen  = configureColor("\033[32m")
	colorYellow = configureColor("\033[33m")
	colorCyan   = configureColor("\033[36m")
	colorBold   = configureColor("\033[1m")
)

func configureColor(code string) string {
	if runtime.GOOS == "windows" || os.Getenv("NO_COLOR") != "" ||
		!term.IsTerminal(int(os.Stdout.Fd())) || !term.IsTerminal(int(os.Stderr.Fd())) {
		return ""
	}
	return code
}

// Forge node identifiers are UUIDs. Numeric IDs remain supported for legacy
// Pterodactyl/Wings imports.
var nodeIDRegex = regexp.MustCompile(`^(?:\d+|[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12})$`)

var configureArgs struct {
	PanelURL      string
	Token         string
	Node          string
	ConfigPath    string
	Format        string
	Override      bool
	AllowInsecure bool
	Quiet         bool
}

var configureCmd = &cobra.Command{
	Use:   "configure",
	Short: "Generate Beacon configuration interactively or from panel",
	RunE:  configureCmdRun,
}

func init() {
	configureCmd.PersistentFlags().StringVarP(&configureArgs.PanelURL, "panel-url", "p", "", "Panel base URL (e.g. https://panel.example.com)")
	configureCmd.PersistentFlags().StringVarP(&configureArgs.Token, "token", "t", "", "Node credential from Forge (token-id.secret)")
	configureCmd.PersistentFlags().StringVarP(&configureArgs.Node, "node", "n", "", "Forge node UUID (numeric legacy IDs also accepted)")
	configureCmd.PersistentFlags().StringVarP(&configureArgs.ConfigPath, "output", "o", "", "Output config path (default: /etc/beacon/config.yml)")
	configureCmd.PersistentFlags().StringVarP(&configureArgs.Format, "format", "f", "yaml", "Config format: yaml or env")
	configureCmd.PersistentFlags().BoolVar(&configureArgs.Override, "override", false, "Override existing config file without asking")
	configureCmd.PersistentFlags().BoolVar(&configureArgs.AllowInsecure, "allow-insecure", false, "Skip TLS verification for panel requests")
	configureCmd.PersistentFlags().BoolVar(&configureArgs.Quiet, "quiet", false, "Non-interactive mode (all flags must be provided)")
}

func configureCmdRun(_ *cobra.Command, _ []string) error {
	configureArgs.Format = strings.ToLower(strings.TrimSpace(configureArgs.Format))
	if configureArgs.Format != "yaml" && configureArgs.Format != "env" {
		fmt.Fprintf(os.Stderr, "%s Error: invalid format %q - must be 'yaml' or 'env'%s\n", colorRed, configureArgs.Format, colorReset)
		return fmt.Errorf("invalid format %q: must be yaml or env", configureArgs.Format)
	}

	outputPath := configureArgs.ConfigPath
	if outputPath == "" {
		outputPath = defaultConfigPath
	}

	// Quiet mode: non-interactive, all flags required
	if configureArgs.Quiet {
		if _, err := os.Stat(outputPath); err == nil && !configureArgs.Override {
			return fmt.Errorf("config already exists at %s; pass --override to replace it", outputPath)
		}
		return quietConfigure(outputPath)
	}

	// Check existing config
	if _, err := os.Stat(outputPath); err == nil && !configureArgs.Override {
		fmt.Printf("Config already exists at %s\n", outputPath)
		fmt.Print("Override? [y/N]: ")
		var resp string
		fmt.Scanln(&resp)
		if strings.ToLower(strings.TrimSpace(resp)) != "y" {
			fmt.Println("Aborting.")
			return nil
		}
	}

	// If all flags provided, validate and fetch directly
	if configureArgs.PanelURL != "" && configureArgs.Token != "" && configureArgs.Node != "" {
		if err := validatePanelURL(configureArgs.PanelURL); err != nil {
			fmt.Fprintf(os.Stderr, "%s Error: invalid panel URL: %v%s\n", colorRed, err, colorReset)
			return fmt.Errorf("invalid panel URL: %w", err)
		}
		if err := validateToken(configureArgs.Token); err != nil {
			fmt.Fprintf(os.Stderr, "%s Error: invalid token: %v%s\n", colorRed, err, colorReset)
			return fmt.Errorf("invalid token: %w", err)
		}
		if !nodeIDRegex.MatchString(configureArgs.Node) {
			fmt.Fprintf(os.Stderr, "%s Error: node ID must be a Forge UUID (or legacy numeric ID), got %q%s\n", colorRed, configureArgs.Node, colorReset)
			return fmt.Errorf("invalid node ID %q", configureArgs.Node)
		}
		return fetchAndWriteConfig(outputPath)
	}

	// Interactive mode
	reader := bufio.NewReader(os.Stdin)

	fmt.Printf("\n  %s=== Beacon Configuration Generator ===%s\n\n", colorBold, colorReset)

	// Node ID
	nodeID := configureArgs.Node
	if nodeID == "" {
		for {
			nodeID = prompt(reader, "Node ID", "Forge node UUID")
			if nodeIDRegex.MatchString(nodeID) {
				break
			}
			fmt.Printf("  %s Node ID must be a Forge UUID (or a legacy numeric ID)%s\n", colorRed, colorReset)
		}
	}

	// Panel URL
	panelURL := configureArgs.PanelURL
	if panelURL == "" {
		for {
			panelURL = prompt(reader, "Panel URL", "https://panel.example.com")
			panelURL = strings.TrimSpace(panelURL)
			if panelURL == "" {
				fmt.Printf("  %s Panel URL cannot be empty%s\n", colorRed, colorReset)
				continue
			}
			if !strings.HasPrefix(panelURL, "http://") && !strings.HasPrefix(panelURL, "https://") {
				panelURL = "https://" + panelURL
			}
			if err := validatePanelURL(panelURL); err != nil {
				fmt.Printf("  %s %v%s\n", colorRed, err, colorReset)
				continue
			}
			break
		}
	}

	// API Token
	token := configureArgs.Token
	if token == "" {
		for {
			token = promptSecret(reader, "Node credential", "token-id.secret (shown once by Forge)")
			token = strings.TrimSpace(token)
			if err := validateToken(token); err != nil {
				fmt.Printf("  %s %v%s\n", colorRed, err, colorReset)
				continue
			}
			if !strings.Contains(token, ".") {
				fmt.Printf("  %s Warning: node credentials normally use token-id.secret format%s\n", colorYellow, colorReset)
			}
			break
		}
	}

	// Runtime provider
	runtime := strings.TrimSpace(prompt(reader, "Runtime provider", "docker"))
	runtime = strings.ToLower(runtime)
	runtimeMap := map[string]bool{"docker": true, "podman": true, "kubernetes": true}
	if !runtimeMap[runtime] {
		fmt.Printf("  %s Unknown runtime %q, defaulting to docker%s\n", colorYellow, runtime, colorReset)
		runtime = "docker"
	}

	// Data directory
	dataDir := prompt(reader, "Server data directory", config.Default().System.DataDirectory)

	// HTTP port
	httpPort := prompt(reader, "HTTP API port", fmt.Sprintf("%d", config.Default().System.API.Port))

	// SFTP port
	sftpPort := prompt(reader, "SFTP port", fmt.Sprintf("%d", config.Default().System.Sftp.Port))

	// Write config
	return writeLocalConfig(outputPath, configureArgs.Format, nodeID, token, panelURL, runtime, dataDir, httpPort, sftpPort)
}

func quietConfigure(outputPath string) error {
	if configureArgs.PanelURL == "" || configureArgs.Token == "" || configureArgs.Node == "" {
		fmt.Fprintf(os.Stderr, "%s Error: --quiet requires --panel-url, --token, and --node%s\n", colorRed, colorReset)
		return errors.New("--quiet requires --panel-url, --token, and --node")
	}
	if err := validatePanelURL(configureArgs.PanelURL); err != nil {
		fmt.Fprintf(os.Stderr, "%s Error: invalid panel URL: %v%s\n", colorRed, err, colorReset)
		return fmt.Errorf("invalid panel URL: %w", err)
	}
	if err := validateToken(configureArgs.Token); err != nil {
		fmt.Fprintf(os.Stderr, "%s Error: invalid token: %v%s\n", colorRed, err, colorReset)
		return fmt.Errorf("invalid token: %w", err)
	}
	if !nodeIDRegex.MatchString(configureArgs.Node) {
		fmt.Fprintf(os.Stderr, "%s Error: node ID must be a Forge UUID (or legacy numeric ID), got %q%s\n", colorRed, configureArgs.Node, colorReset)
		return fmt.Errorf("invalid node ID %q", configureArgs.Node)
	}
	return writeLocalConfig(outputPath, configureArgs.Format, configureArgs.Node, configureArgs.Token, configureArgs.PanelURL, "docker", "/srv/game-panel/servers", "9090", "2022")
}

func validatePanelURL(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid panel URL: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("URL must use http or https scheme, got %q", u.Scheme)
	}
	if u.Host == "" {
		return fmt.Errorf("URL must include a hostname (e.g. panel.example.com)")
	}
	if u.User != nil {
		return errors.New("URL must not contain credentials")
	}
	if u.RawQuery != "" || u.Fragment != "" {
		return errors.New("URL must not contain a query string or fragment")
	}
	hostIP := net.ParseIP(u.Hostname())
	isLoopback := strings.EqualFold(u.Hostname(), "localhost") || hostIP != nil && hostIP.IsLoopback()
	if u.Scheme != "https" && !isLoopback && !configureArgs.AllowInsecure {
		return errors.New("remote panel URLs must use HTTPS (or explicitly pass --allow-insecure)")
	}
	return nil
}

func validateToken(token string) error {
	if token == "" {
		return fmt.Errorf("token cannot be empty")
	}
	if len(token) < 20 {
		return fmt.Errorf("token is too short (%d characters, expected at least 20)", len(token))
	}
	return nil
}

func fetchAndWriteConfig(outputPath string) error {
	if err := fetchRemoteConfig(outputPath, configureArgs.PanelURL, configureArgs.Token, configureArgs.Node); err != nil {
		fmt.Fprintf(os.Stderr, "%s Error: %v%s\n", colorRed, err, colorReset)
		return err
	}
	printSuccess(outputPath)
	return nil
}

func fetchRemoteConfig(outputPath, panelURL, token, nodeID string) error {
	u, err := url.Parse(strings.TrimRight(panelURL, "/"))
	if err != nil {
		return fmt.Errorf("invalid panel URL: %w", err)
	}
	u.Path = path.Join(u.Path, fmt.Sprintf("api/v1/nodes/%s/configuration", nodeID))

	req, err := http.NewRequest(http.MethodGet, u.String(), nil)
	if err != nil {
		return fmt.Errorf("failed to create HTTP request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.forge.v1+json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	transport := http.DefaultTransport.(*http.Transport).Clone()
	if configureArgs.AllowInsecure {
		transport.TLSClientConfig = &tls.Config{MinVersion: tls.VersionTLS12, InsecureSkipVerify: true} // #nosec G402 -- explicit one-request CLI override
	} else {
		transport.TLSClientConfig = &tls.Config{MinVersion: tls.VersionTLS12}
	}
	client := &http.Client{
		Transport: transport,
		Timeout:   30 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return fmt.Errorf("too many redirects")
			}
			if !sameOrigin(u, req.URL) {
				return errors.New("panel request refused a cross-origin redirect")
			}
			return nil
		},
	}
	res, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("panel request failed (check panel URL and network): %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode == http.StatusForbidden || res.StatusCode == http.StatusUnauthorized {
		return fmt.Errorf("authentication failed (status %d) - check your API token", res.StatusCode)
	}
	if res.StatusCode != http.StatusOK {
		body, readErr := io.ReadAll(io.LimitReader(res.Body, 16*1024))
		if readErr != nil {
			return fmt.Errorf("panel returned status %d and its response could not be read: %w", res.StatusCode, readErr)
		}
		return fmt.Errorf("panel returned status %d: %s", res.StatusCode, strings.TrimSpace(string(body)))
	}

	body, err := io.ReadAll(io.LimitReader(res.Body, 4<<20))
	if err != nil {
		return fmt.Errorf("failed to read panel response: %w", err)
	}

	var panelCfg map[string]interface{}
	if err := json.Unmarshal(body, &panelCfg); err != nil {
		return fmt.Errorf("failed to decode panel response: %w", err)
	}

	// Merge with defaults and write
	cfg := config.Default()
	cfg.UUID, _ = panelCfg["uuid"].(string)
	cfg.TokenID, _ = panelCfg["token_id"].(string)
	if cfg.TokenID == "" {
		cfg.TokenID, _, _ = strings.Cut(token, ".")
	}
	tokenReference, err := writeConfigToken(outputPath, token)
	if err != nil {
		return fmt.Errorf("write node credential: %w", err)
	}
	cfg.Token = tokenReference
	cfg.PanelURL = panelURL
	cfg.Remote = panelURL

	// System fields
	if v, ok := panelCfg["data_directory"].(string); ok && v != "" {
		cfg.System.DataDirectory = v
	}
	if v, ok := panelCfg["root_directory"].(string); ok && v != "" {
		cfg.System.RootDirectory = v
	}
	if v, ok := panelCfg["log_directory"].(string); ok && v != "" {
		cfg.System.LogDirectory = v
	}
	if v, ok := panelCfg["archive_directory"].(string); ok && v != "" {
		cfg.System.ArchiveDirectory = v
	}
	if v, ok := panelCfg["backup_directory"].(string); ok && v != "" {
		cfg.System.BackupDirectory = v
	}
	if v, ok := panelCfg["username"].(string); ok && v != "" {
		cfg.System.Username = v
	}

	// Docker fields
	if netCfg := panelSubMap(panelSubMap(panelCfg, "docker"), "network"); netCfg != nil {
		if v, ok := netCfg["interface"].(string); ok && v != "" {
			cfg.Docker.Network.Interface = v
		}
		if v, ok := netCfg["name"].(string); ok && v != "" {
			cfg.Docker.Network.Name = v
		}
		if v, ok := netCfg["network_mode"].(string); ok && v != "" {
			cfg.Docker.Network.Mode = v
		}
	}

	// API
	if apiCfg := panelSubMap(panelCfg, "api"); apiCfg != nil {
		if v, ok := apiCfg["host"].(string); ok && v != "" {
			cfg.System.API.Host = v
		}
		if v, ok := apiCfg["port"].(float64); ok && v > 0 {
			cfg.System.API.Port = int(v)
		}
	}

	if err := os.MkdirAll(filepath.Dir(outputPath), 0o700); err != nil {
		return fmt.Errorf("failed to create config directory %s: %w", filepath.Dir(outputPath), err)
	}

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("failed to marshal configuration: %w", err)
	}

	if err := writePrivateFileAtomic(outputPath, data); err != nil {
		return fmt.Errorf("failed to write config to %s: %w", outputPath, err)
	}

	return nil
}

func writeLocalConfig(outputPath, format, nodeID, token, panelURL, runtime, dataDir, httpPort, sftpPort string) error {
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o700); err != nil {
		fmt.Fprintf(os.Stderr, "%s Error: failed to create config directory %s: %v%s\n", colorRed, filepath.Dir(outputPath), err, colorReset)
		return fmt.Errorf("create config directory %s: %w", filepath.Dir(outputPath), err)
	}

	timestamp := time.Now().Format(time.RFC3339)

	switch format {
	case "env":
		content := fmt.Sprintf(`# Beacon Configuration
# Generated by 'beacon configure' on %s

DAEMON_NODE_ID=%s
DAEMON_NODE_TOKEN=%s
PANEL_API_URL=%s
DAEMON_RUNTIME_PROVIDER=%s
DAEMON_DATA_DIR=%s
DAEMON_ADDR=127.0.0.1:%s
DAEMON_SFTP_ADDR=127.0.0.1:%s
`, timestamp, nodeID, token, panelURL, runtime, dataDir, httpPort, sftpPort)

		if err := writePrivateFileAtomic(outputPath, []byte(content)); err != nil {
			fmt.Fprintf(os.Stderr, "%s Error: failed to write config to %s: %v%s\n", colorRed, outputPath, err, colorReset)
			return fmt.Errorf("write config to %s: %w", outputPath, err)
		}

	default: // yaml
		cfg := config.Default()
		cfg.UUID = nodeID
		cfg.TokenID, _, _ = strings.Cut(token, ".")
		tokenReference, err := writeConfigToken(outputPath, token)
		if err != nil {
			return fmt.Errorf("write node credential: %w", err)
		}
		cfg.Token = tokenReference
		cfg.PanelURL = panelURL
		cfg.Remote = panelURL

		switch runtime {
		case "podman":
			cfg.Docker.Timezone = ""
		case "kubernetes":
			cfg.Docker.Timezone = ""
		default: // docker
		}

		// Apply user-specified overrides
		if dataDir != "" {
			cfg.System.DataDirectory = dataDir
		}
		if port := strings.TrimPrefix(httpPort, ":"); port != "" {
			value, err := parsePort(port)
			if err != nil {
				fmt.Fprintf(os.Stderr, "%s Error: invalid HTTP API port: %v%s\n", colorRed, err, colorReset)
				return fmt.Errorf("invalid HTTP API port: %w", err)
			}
			cfg.System.API.Port = value
		}
		if port := strings.TrimPrefix(sftpPort, ":"); port != "" {
			value, err := parsePort(port)
			if err != nil {
				fmt.Fprintf(os.Stderr, "%s Error: invalid SFTP port: %v%s\n", colorRed, err, colorReset)
				return fmt.Errorf("invalid SFTP port: %w", err)
			}
			cfg.System.Sftp.Port = value
		}

		data, err := yaml.Marshal(cfg)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s Error: failed to marshal configuration: %v%s\n", colorRed, err, colorReset)
			return fmt.Errorf("marshal configuration: %w", err)
		}

		if err := writePrivateFileAtomic(outputPath, data); err != nil {
			fmt.Fprintf(os.Stderr, "%s Error: failed to write config to %s: %v%s\n", colorRed, outputPath, err, colorReset)
			return fmt.Errorf("write config to %s: %w", outputPath, err)
		}
	}

	printSuccess(outputPath)
	return nil
}

func writeConfigToken(configPath, token string) (string, error) {
	tokenPath := filepath.Join(filepath.Dir(configPath), "node.token")
	if err := writePrivateFileAtomic(tokenPath, []byte(strings.TrimSpace(token)+"\n")); err != nil {
		return "", err
	}
	absolute, err := filepath.Abs(tokenPath)
	if err != nil {
		return "", err
	}
	return "file://" + absolute, nil
}

func printSuccess(outputPath string) {
	fmt.Printf("\n%s Configuration written to %s%s\n", colorGreen, outputPath, colorReset)
	fmt.Println()
	fmt.Println("To verify connectivity, run:")
	fmt.Printf("  %sbeacon diagnostics%s\n", colorBold, colorReset)
	fmt.Println()
	fmt.Println("To start Beacon:")
	fmt.Println("  sudo systemctl enable --now beacon")
	fmt.Println()
	fmt.Println("Or for testing:")
	executable, err := os.Executable()
	if err != nil {
		executable = "beacon"
	}
	fmt.Printf("  sudo %s\n", executable)
}

func prompt(reader *bufio.Reader, question, example string) string {
	if example != "" {
		fmt.Printf("%s [%s%s%s]: ", question, colorCyan, example, colorReset)
	} else {
		fmt.Printf("%s: ", question)
	}

	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(input)

	if input == "" && example != "" {
		return example
	}
	return input
}

func promptSecret(reader *bufio.Reader, question, example string) string {
	if example != "" {
		fmt.Printf("%s [%s%s%s]: ", question, colorCyan, example, colorReset)
	} else {
		fmt.Printf("%s: ", question)
	}

	var input string
	if term.IsTerminal(int(os.Stdin.Fd())) {
		secret, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Println()
		if err == nil {
			input = string(secret)
		}
	} else {
		input, _ = reader.ReadString('\n')
	}
	input = strings.TrimSpace(input)

	return input
}

// panelSubMap safely extracts a nested object from a decoded JSON map. It
// returns nil when the key is absent or is not an object, letting callers skip
// optional configuration sections without panicking on type assertions.
func panelSubMap(m map[string]interface{}, key string) map[string]interface{} {
	if m == nil {
		return nil
	}
	sub, _ := m[key].(map[string]interface{})
	return sub
}

func sameOrigin(left, right *url.URL) bool {
	return left != nil && right != nil &&
		strings.EqualFold(left.Scheme, right.Scheme) &&
		strings.EqualFold(left.Host, right.Host)
}

func parsePort(value string) (int, error) {
	port, err := strconv.Atoi(value)
	if err != nil || port < 1 || port > 65535 {
		return 0, fmt.Errorf("port must be an integer from 1 to 65535")
	}
	return port, nil
}

func writePrivateFileAtomic(destination string, data []byte) error {
	dir := filepath.Dir(destination)
	temp, err := os.CreateTemp(dir, "."+filepath.Base(destination)+".tmp-*")
	if err != nil {
		return err
	}
	tempPath := temp.Name()
	success := false
	defer func() {
		_ = temp.Close()
		if !success {
			_ = os.Remove(tempPath)
		}
	}()
	if err := temp.Chmod(0o600); err != nil {
		return err
	}
	if _, err := temp.Write(data); err != nil {
		return err
	}
	if err := temp.Sync(); err != nil {
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tempPath, destination); err != nil {
		return err
	}
	if err := syncDirectory(dir); err != nil {
		return err
	}
	success = true
	return nil
}
