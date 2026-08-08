package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

const (
	maxUpdateBinaryBytes = int64(512 << 20)
	maxChecksumFileBytes = int64(1 << 20)
	maxReleaseJSONBytes  = int64(1 << 20)
)

var (
	repositoryPartPattern = regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)
	updateHTTPClient      = newUpdateHTTPClient()
)

var updateArgs struct {
	repoOwner string
	repoName  string
	force     bool
}

func newUpdateCommand() *cobra.Command {
	command := &cobra.Command{
		Use:   "update",
		Short: "Update beacon to the latest version",
		Run:   selfupdateCmdRun,
	}

	command.Flags().StringVar(&updateArgs.repoOwner, "repo-owner", "anomalyco", "GitHub repository owner")
	command.Flags().StringVar(&updateArgs.repoName, "repo-name", "gamepanel", "GitHub repository name")
	command.Flags().BoolVar(&updateArgs.force, "force", false, "Force update even if on development version")

	return command
}

func selfupdateCmdRun(_ *cobra.Command, _ []string) {
	currentVersion := Version
	if currentVersion == "" {
		fmt.Println("Error: current version is not defined")
		return
	}

	if currentVersion == "beacon-dev" && !updateArgs.force {
		fmt.Println("Running in development mode. Use --force to override.")
		return
	}

	fmt.Printf("Current version: %s\n", currentVersion)

	latestVersionTag, err := fetchLatestGitHubRelease()
	if err != nil {
		fmt.Printf("Failed to fetch latest version: %v\n", err)
		return
	}

	currentVersionTag := "v" + currentVersion
	if currentVersion == "beacon-dev" {
		currentVersionTag = currentVersion
	}

	if currentVersion != "beacon-dev" {
		comparison, err := compareVersions(currentVersionTag, latestVersionTag)
		if err != nil {
			fmt.Printf("Update metadata is invalid: %v\n", err)
			return
		}
		if comparison >= 0 && !updateArgs.force {
			fmt.Printf("You are running the latest version: %s\n", currentVersion)
			return
		}
	}

	binaryName := determineBinaryName()
	if binaryName == "" {
		fmt.Printf("Error: unsupported architecture: %s\n", runtime.GOARCH)
		return
	}

	fmt.Printf("Updating from %s to %s\n", currentVersionTag, latestVersionTag)

	if err := performUpdate(latestVersionTag, binaryName); err != nil {
		fmt.Printf("Update failed: %v\n", err)
		return
	}

	fmt.Println("\nUpdate successful! Please restart the beacon service (e.g., systemctl restart beacon)")
}

func performUpdate(version, binaryName string) error {
	if runtime.GOOS == "windows" {
		return errors.New("in-place self-update is not supported on Windows; install the signed release with the service stopped")
	}
	if !repositoryPartPattern.MatchString(updateArgs.repoOwner) || !repositoryPartPattern.MatchString(updateArgs.repoName) {
		return errors.New("repository owner and name contain invalid characters")
	}
	if _, err := parseVersion(version); err != nil {
		return fmt.Errorf("invalid release version: %w", err)
	}
	if filepath.Base(binaryName) != binaryName || binaryName == "." {
		return errors.New("invalid release binary name")
	}
	downloadURL := fmt.Sprintf("https://github.com/%s/%s/releases/download/%s/%s",
		updateArgs.repoOwner, updateArgs.repoName, version, binaryName)
	checksumURL := fmt.Sprintf("https://github.com/%s/%s/releases/download/%s/checksums.txt",
		updateArgs.repoOwner, updateArgs.repoName, version)
	signatureURL := checksumURL + ".sig"

	tmpDir, err := os.MkdirTemp("", "beacon-update-*")
	if err != nil {
		return fmt.Errorf("failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tmpDir)
	if err := os.Chmod(tmpDir, 0o700); err != nil {
		return fmt.Errorf("secure temp directory: %w", err)
	}

	checksumPath := filepath.Join(tmpDir, "checksums.txt")
	if err := downloadWithProgress(checksumURL, checksumPath, maxChecksumFileBytes); err != nil {
		return fmt.Errorf("failed to download checksums: %v", err)
	}
	signaturePath := filepath.Join(tmpDir, "checksums.txt.sig")
	if err := downloadWithProgress(signatureURL, signaturePath, 1024); err != nil {
		return fmt.Errorf("failed to download checksum signature: %v", err)
	}
	if err := verifyChecksumManifestSignature(checksumPath, signaturePath); err != nil {
		return fmt.Errorf("release signature verification failed: %w", err)
	}

	binaryPath := filepath.Join(tmpDir, binaryName)
	if err := downloadWithProgress(downloadURL, binaryPath, maxUpdateBinaryBytes); err != nil {
		return fmt.Errorf("failed to download binary: %v", err)
	}

	if err := verifyChecksum(binaryPath, checksumPath, binaryName); err != nil {
		return fmt.Errorf("checksum verification failed: %v", err)
	}

	if err := os.Chmod(binaryPath, 0o755); err != nil {
		return fmt.Errorf("failed to set executable permissions: %v", err)
	}

	currentExecutable, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to locate current executable: %v", err)
	}
	currentExecutable, err = filepath.EvalSymlinks(currentExecutable)
	if err != nil {
		return fmt.Errorf("resolve current executable: %w", err)
	}
	expected, err := expectedChecksum(checksumPath, binaryName)
	if err != nil {
		return err
	}
	return installUpdateAtomically(binaryPath, currentExecutable, expected)
}

func verifyChecksumManifestSignature(manifestPath, signaturePath string) error {
	keyEncoded := strings.TrimSpace(UpdatePublicKey)
	if keyEncoded == "" {
		keyEncoded = strings.TrimSpace(os.Getenv("BEACON_UPDATE_PUBLIC_KEY"))
	}
	publicKey, err := base64.StdEncoding.DecodeString(keyEncoded)
	if err != nil || len(publicKey) != ed25519.PublicKeySize {
		return errors.New("a valid base64 Ed25519 update public key is required")
	}
	manifest, err := os.ReadFile(manifestPath)
	if err != nil {
		return err
	}
	if len(manifest) > int(maxChecksumFileBytes) {
		return errors.New("checksum manifest is too large")
	}
	signatureEncoded, err := os.ReadFile(signaturePath)
	if err != nil {
		return err
	}
	signature, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(signatureEncoded)))
	if err != nil || len(signature) != ed25519.SignatureSize {
		return errors.New("checksum signature must be base64-encoded Ed25519")
	}
	if !ed25519.Verify(ed25519.PublicKey(publicKey), manifest, signature) {
		return errors.New("checksum manifest publisher signature is invalid")
	}
	return nil
}

func downloadWithProgress(rawURL, dest string, maxBytes int64) error {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("parse download URL: %w", err)
	}
	if err := validateUpdateURL(parsed); err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodGet, parsed.String(), nil)
	if err != nil {
		return err
	}
	resp, err := updateHTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status: %s", resp.Status)
	}
	if resp.ContentLength > maxBytes {
		return fmt.Errorf("download exceeds %d-byte limit", maxBytes)
	}

	out, err := os.OpenFile(dest, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	success := false
	defer func() {
		_ = out.Close()
		if !success {
			_ = os.Remove(dest)
		}
	}()

	filename := filepath.Base(dest)
	fmt.Printf("Downloading %s (%.2f MB)...\n", filename, float64(resp.ContentLength)/1024/1024)

	pw := &progressWriter{
		Writer:    out,
		Total:     resp.ContentLength,
		StartTime: time.Now(),
	}

	written, err := io.Copy(pw, io.LimitReader(resp.Body, maxBytes+1))
	fmt.Println()
	if err != nil {
		return err
	}
	if written > maxBytes {
		return fmt.Errorf("download exceeds %d-byte limit", maxBytes)
	}
	if err := out.Sync(); err != nil {
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	success = true
	return nil
}

func fetchLatestGitHubRelease() (string, error) {
	if !repositoryPartPattern.MatchString(updateArgs.repoOwner) || !repositoryPartPattern.MatchString(updateArgs.repoName) {
		return "", errors.New("repository owner and name contain invalid characters")
	}
	apiURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/releases/latest", updateArgs.repoOwner, updateArgs.repoName)

	req, err := http.NewRequest(http.MethodGet, apiURL, nil)
	if err != nil {
		return "", err
	}
	if token := strings.TrimSpace(os.Getenv("GITHUB_TOKEN")); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	resp, err := updateHTTPClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var releaseData struct {
		TagName string `json:"tag_name"`
	}
	decoder := json.NewDecoder(io.LimitReader(resp.Body, maxReleaseJSONBytes+1))
	if err := decoder.Decode(&releaseData); err != nil {
		return "", err
	}
	if _, err := parseVersion(releaseData.TagName); err != nil {
		return "", fmt.Errorf("invalid release tag: %w", err)
	}
	return releaseData.TagName, nil
}

func determineBinaryName() string {
	switch runtime.GOOS {
	case "linux", "darwin", "windows":
	default:
		return ""
	}
	switch runtime.GOARCH {
	case "amd64", "arm64":
		name := fmt.Sprintf("beacon_%s_%s", runtime.GOOS, runtime.GOARCH)
		if runtime.GOOS == "windows" {
			name += ".exe"
		}
		return name
	default:
		return ""
	}
}

func verifyChecksum(binaryPath, checksumPath, binaryName string) error {
	expectedChecksum, err := expectedChecksum(checksumPath, binaryName)
	if err != nil {
		return err
	}
	actualChecksum, err := checksumFile(binaryPath)
	if err != nil {
		return err
	}

	if actualChecksum == expectedChecksum {
		fmt.Printf("Checksum verification successful!\n")
	}

	if actualChecksum != expectedChecksum {
		return fmt.Errorf("checksum mismatch: expected %s, got %s", expectedChecksum, actualChecksum)
	}

	return nil
}

func expectedChecksum(checksumPath, binaryName string) (string, error) {
	checksumData, err := os.ReadFile(checksumPath)
	if err != nil {
		return "", err
	}
	if len(checksumData) > int(maxChecksumFileBytes) {
		return "", errors.New("checksum manifest is too large")
	}
	for _, line := range strings.Split(string(checksumData), "\n") {
		parts := strings.Fields(line)
		if len(parts) != 2 || strings.TrimPrefix(parts[1], "*") != binaryName {
			continue
		}
		expected := strings.ToLower(parts[0])
		decoded, err := hex.DecodeString(expected)
		if err != nil || len(decoded) != sha256.Size {
			return "", fmt.Errorf("invalid checksum for %s", binaryName)
		}
		return expected, nil
	}
	return "", fmt.Errorf("checksum not found for %s", binaryName)
}

func checksumFile(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hasher.Sum(nil)), nil
}

func installUpdateAtomically(downloadPath, executablePath, expected string) error {
	execDir := filepath.Dir(executablePath)
	suffix, err := randomSuffix()
	if err != nil {
		return err
	}
	stagedPath := filepath.Join(execDir, "."+filepath.Base(executablePath)+".new-"+suffix)
	if err := copySynced(downloadPath, stagedPath, 0o755); err != nil {
		return fmt.Errorf("stage replacement binary: %w", err)
	}
	defer os.Remove(stagedPath)

	actual, err := checksumFile(stagedPath)
	if err != nil {
		return fmt.Errorf("verify staged replacement: %w", err)
	}
	if !strings.EqualFold(actual, expected) {
		return fmt.Errorf("staged replacement checksum mismatch: expected %s, got %s", expected, actual)
	}

	backupPath := filepath.Join(execDir, "."+filepath.Base(executablePath)+".rollback-"+suffix)
	if err := os.Link(executablePath, backupPath); err != nil {
		info, statErr := os.Stat(executablePath)
		if statErr != nil {
			return fmt.Errorf("inspect current executable: %w", statErr)
		}
		if err := copySynced(executablePath, backupPath, info.Mode().Perm()); err != nil {
			return fmt.Errorf("create rollback copy: %w", err)
		}
	}

	if err := os.Rename(stagedPath, executablePath); err != nil {
		_ = os.Remove(backupPath)
		return fmt.Errorf("atomically replace executable: %w", err)
	}
	if err := syncDirectory(execDir); err != nil {
		rollbackErr := os.Rename(backupPath, executablePath)
		_ = syncDirectory(execDir)
		if rollbackErr != nil {
			return fmt.Errorf("sync replacement directory: %v (automatic rollback failed: %v; rollback copy: %s)", err, rollbackErr, backupPath)
		}
		return fmt.Errorf("sync replacement directory: %w (rolled back)", err)
	}
	fmt.Printf("Rollback copy retained at %s\n", backupPath)
	return nil
}

func copySynced(sourcePath, destinationPath string, mode os.FileMode) error {
	source, err := os.Open(sourcePath)
	if err != nil {
		return err
	}
	defer source.Close()
	destination, err := os.OpenFile(destinationPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return err
	}
	success := false
	defer func() {
		_ = destination.Close()
		if !success {
			_ = os.Remove(destinationPath)
		}
	}()
	if _, err := io.Copy(destination, source); err != nil {
		return err
	}
	if err := destination.Sync(); err != nil {
		return err
	}
	if err := destination.Close(); err != nil {
		return err
	}
	success = true
	return nil
}

func syncDirectory(path string) error {
	dir, err := os.Open(path)
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}

func randomSuffix() (string, error) {
	value := make([]byte, 12)
	if _, err := rand.Read(value); err != nil {
		return "", fmt.Errorf("generate update path: %w", err)
	}
	return hex.EncodeToString(value), nil
}

func newUpdateHTTPClient() *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = &tls.Config{MinVersion: tls.VersionTLS12}
	transport.DialContext = (&net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}).DialContext
	transport.TLSHandshakeTimeout = 10 * time.Second
	transport.ResponseHeaderTimeout = 30 * time.Second
	return &http.Client{
		Transport: transport,
		Timeout:   30 * time.Minute,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return errors.New("too many update redirects")
			}
			return validateUpdateURL(req.URL)
		},
	}
}

func validateUpdateURL(target *url.URL) error {
	if target == nil || target.Scheme != "https" || target.User != nil {
		return errors.New("update URL must be credential-free HTTPS")
	}
	host := strings.ToLower(target.Hostname())
	if host == "github.com" || host == "api.github.com" || strings.HasSuffix(host, ".githubusercontent.com") {
		return nil
	}
	return fmt.Errorf("update URL host %q is not trusted", host)
}

type semanticVersion struct {
	core       [3]int
	prerelease []string
}

func parseVersion(value string) (semanticVersion, error) {
	var parsed semanticVersion
	value = strings.TrimPrefix(strings.TrimSpace(value), "v")
	value = strings.SplitN(value, "+", 2)[0]
	parts := strings.SplitN(value, "-", 2)
	core := strings.Split(parts[0], ".")
	if len(core) != 3 {
		return parsed, errors.New("version must contain major.minor.patch")
	}
	for index, component := range core {
		if component == "" || len(component) > 1 && component[0] == '0' {
			return parsed, errors.New("invalid numeric version component")
		}
		number, err := strconv.Atoi(component)
		if err != nil || number < 0 {
			return parsed, errors.New("invalid numeric version component")
		}
		parsed.core[index] = number
	}
	if len(parts) == 2 {
		if parts[1] == "" {
			return parsed, errors.New("empty prerelease version")
		}
		parsed.prerelease = strings.Split(parts[1], ".")
		for _, identifier := range parsed.prerelease {
			if identifier == "" || !repositoryPartPattern.MatchString(identifier) {
				return parsed, errors.New("invalid prerelease identifier")
			}
			if _, err := strconv.Atoi(identifier); err == nil && len(identifier) > 1 && identifier[0] == '0' {
				return parsed, errors.New("numeric prerelease identifiers must not contain leading zeros")
			}
		}
	}
	return parsed, nil
}

func compareVersions(left, right string) (int, error) {
	a, err := parseVersion(left)
	if err != nil {
		return 0, fmt.Errorf("current version: %w", err)
	}
	b, err := parseVersion(right)
	if err != nil {
		return 0, fmt.Errorf("latest version: %w", err)
	}
	for index := range a.core {
		if a.core[index] < b.core[index] {
			return -1, nil
		}
		if a.core[index] > b.core[index] {
			return 1, nil
		}
	}
	if len(a.prerelease) == 0 && len(b.prerelease) == 0 {
		return 0, nil
	}
	if len(a.prerelease) == 0 {
		return 1, nil
	}
	if len(b.prerelease) == 0 {
		return -1, nil
	}
	for index := 0; index < len(a.prerelease) && index < len(b.prerelease); index++ {
		if comparison := comparePrereleaseIdentifier(a.prerelease[index], b.prerelease[index]); comparison != 0 {
			return comparison, nil
		}
	}
	switch {
	case len(a.prerelease) < len(b.prerelease):
		return -1, nil
	case len(a.prerelease) > len(b.prerelease):
		return 1, nil
	default:
		return 0, nil
	}
}

func comparePrereleaseIdentifier(left, right string) int {
	leftNumber, leftErr := strconv.Atoi(left)
	rightNumber, rightErr := strconv.Atoi(right)
	switch {
	case leftErr == nil && rightErr == nil:
		if leftNumber < rightNumber {
			return -1
		}
		if leftNumber > rightNumber {
			return 1
		}
		return 0
	case leftErr == nil:
		return -1
	case rightErr == nil:
		return 1
	case left < right:
		return -1
	case left > right:
		return 1
	default:
		return 0
	}
}

type progressWriter struct {
	io.Writer
	Total     int64
	Written   int64
	StartTime time.Time
}

func (pw *progressWriter) Write(p []byte) (int, error) {
	n, err := pw.Writer.Write(p)
	pw.Written += int64(n)

	if pw.Total > 0 {
		percent := float64(pw.Written) / float64(pw.Total) * 100
		fmt.Printf("\rProgress: %.2f%%", percent)
	}

	return n, err
}
