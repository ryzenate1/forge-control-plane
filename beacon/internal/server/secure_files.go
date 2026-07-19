package server

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"

	"gamepanel/beacon/internal/rootfs"
	"gamepanel/beacon/internal/serverid"
)

const (
	maxFileWriteBytes        = int64(16 * 1024 * 1024)
	defaultMaxUploadBytes    = int64(2 * 1024 * 1024 * 1024)
	defaultMaxPullBytes      = int64(512 * 1024 * 1024)
	defaultMaxArchiveBytes   = int64(4 * 1024 * 1024 * 1024)
	defaultMaxArchiveEntries = 100000
	uploadExpiry             = 24 * time.Hour
)

var uploadLocks sync.Map

func lockUpload(serverID, uploadID string) func() {
	key := serverID + "/" + uploadID
	value, _ := uploadLocks.LoadOrStore(key, &sync.Mutex{})
	mu := value.(*sync.Mutex)
	mu.Lock()
	return func() {
		mu.Unlock()
		uploadLocks.Delete(key)
	}
}

func envBytes(name string, fallback int64) int64 {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}

func (s *Server) serverFilesystem(serverID string, create bool) (*rootfs.FS, error) {
	if err := serverid.Validate(serverID); err != nil {
		return nil, err
	}
	base, err := rootfs.New(s.dataDir)
	if err != nil {
		return nil, err
	}
	if create {
		err = base.MkdirAll(serverID, 0o750)
	} else {
		_, err = base.Stat(serverID)
	}
	_ = base.Close()
	if err != nil {
		return nil, err
	}
	return rootfs.New(path.Join(s.dataDir, serverID))
}

func cleanupExpiredUploads(fsys *rootfs.FS, now time.Time) {
	entries, err := fsys.ReadDir(".uploads")
	if err != nil {
		return
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".part") {
			continue
		}
		info, err := entry.Info()
		if err == nil && now.Sub(info.ModTime()) > uploadExpiry {
			_ = fsys.RemoveAll(path.Join(".uploads", entry.Name()))
		}
	}
}

type archiveLimits struct {
	bytes   int64
	entries int
}

type archivePathTracker struct {
	entries        map[string]bool
	requiredParent map[string]struct{}
}

func newArchivePathTracker() *archivePathTracker {
	return &archivePathTracker{entries: make(map[string]bool), requiredParent: make(map[string]struct{})}
}

func (t *archivePathTracker) add(name string, directory bool) error {
	if _, exists := t.entries[name]; exists {
		return errors.New("archive contains duplicate paths")
	}
	if !directory {
		if _, required := t.requiredParent[name]; required {
			return errors.New("archive contains a file used as a parent directory")
		}
	}
	for parent := path.Dir(name); parent != "." && parent != ""; parent = path.Dir(parent) {
		if isDirectory, exists := t.entries[parent]; exists && !isDirectory {
			return errors.New("archive contains a file used as a parent directory")
		}
		t.requiredParent[parent] = struct{}{}
	}
	t.entries[name] = directory
	return nil
}

func validateArchiveName(name string) (string, error) {
	if strings.HasPrefix(name, "/") || strings.Contains(name, "\\") {
		return "", errors.New("archive contains an absolute or platform-specific path")
	}
	clean, err := rootfs.Clean(name)
	if err != nil || clean == "" {
		return "", errors.New("archive contains an invalid path")
	}
	return clean, nil
}

func validateZip(reader *zip.Reader, limits archiveLimits) (int64, error) {
	if len(reader.File) > limits.entries {
		return 0, errors.New("archive contains too many entries")
	}
	var total int64
	tracker := newArchivePathTracker()
	for _, entry := range reader.File {
		name, err := validateArchiveName(entry.Name)
		if err != nil {
			return 0, err
		}
		mode := entry.Mode()
		if mode&os.ModeSymlink != 0 || (!mode.IsRegular() && !mode.IsDir()) {
			return 0, errors.New("archive contains a link or special file")
		}
		if err := tracker.add(name, mode.IsDir()); err != nil {
			return 0, err
		}
		if !mode.IsDir() {
			size := int64(entry.UncompressedSize64)
			if size < 0 || total > limits.bytes-size {
				return 0, errors.New("archive expanded size exceeds limit")
			}
			total += size
		}
	}
	return total, nil
}

func validateTar(file *os.File, limits archiveLimits) (int64, error) {
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return 0, err
	}
	gz, err := gzip.NewReader(file)
	if err != nil {
		return 0, err
	}
	defer gz.Close()
	reader := tar.NewReader(gz)
	var total int64
	entries := 0
	tracker := newArchivePathTracker()
	for {
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			return total, nil
		}
		if err != nil {
			return 0, err
		}
		entries++
		if entries > limits.entries {
			return 0, errors.New("archive contains too many entries")
		}
		name, err := validateArchiveName(header.Name)
		if err != nil {
			return 0, err
		}
		directory := header.Typeflag == tar.TypeDir
		switch header.Typeflag {
		case tar.TypeDir:
		case tar.TypeReg, tar.TypeRegA:
			if header.Size < 0 || total > limits.bytes-header.Size {
				return 0, errors.New("archive expanded size exceeds limit")
			}
			total += header.Size
		default:
			return 0, errors.New("archive contains a link or special file")
		}
		if err := tracker.add(name, directory); err != nil {
			return 0, err
		}
	}
}

func extractZipStaged(fsys *rootfs.FS, reader *zip.Reader, stage string, limits archiveLimits) error {
	var written int64
	for _, entry := range reader.File {
		name, err := validateArchiveName(entry.Name)
		if err != nil {
			return err
		}
		target := path.Join(stage, name)
		if entry.FileInfo().IsDir() {
			if err := fsys.MkdirAll(target, 0o750); err != nil {
				return err
			}
			continue
		}
		if entry.Mode()&os.ModeSymlink != 0 || !entry.Mode().IsRegular() {
			return errors.New("archive contains a link or special file")
		}
		source, err := entry.Open()
		if err != nil {
			return err
		}
		if err := fsys.MkdirAll(path.Dir(target), 0o750); err != nil {
			source.Close()
			return err
		}
		destination, err := fsys.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, entry.Mode().Perm()&0o750)
		if err != nil {
			source.Close()
			return err
		}
		remaining := limits.bytes - written
		count, copyErr := io.Copy(destination, io.LimitReader(source, remaining+1))
		closeErr := destination.Close()
		_ = source.Close()
		written += count
		if copyErr != nil {
			return copyErr
		}
		if count > remaining {
			return errors.New("archive expanded size exceeds limit")
		}
		if count != int64(entry.UncompressedSize64) {
			return errors.New("archive entry size is incomplete")
		}
		if closeErr != nil {
			return closeErr
		}
	}
	return nil
}

func extractTarStaged(fsys *rootfs.FS, file *os.File, stage string, limits archiveLimits) error {
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return err
	}
	gz, err := gzip.NewReader(file)
	if err != nil {
		return err
	}
	defer gz.Close()
	reader := tar.NewReader(gz)
	var written int64
	entries := 0
	for {
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		entries++
		if entries > limits.entries {
			return errors.New("archive contains too many entries")
		}
		name, err := validateArchiveName(header.Name)
		if err != nil {
			return err
		}
		target := path.Join(stage, name)
		switch header.Typeflag {
		case tar.TypeDir:
			if err := fsys.MkdirAll(target, 0o750); err != nil {
				return err
			}
		case tar.TypeReg, tar.TypeRegA:
			if header.Size < 0 || header.Size > limits.bytes-written {
				return errors.New("archive expanded size exceeds limit")
			}
			if err := fsys.MkdirAll(path.Dir(target), 0o750); err != nil {
				return err
			}
			destination, err := fsys.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, os.FileMode(header.Mode)&0o750)
			if err != nil {
				return err
			}
			count, copyErr := io.Copy(destination, io.LimitReader(reader, header.Size+1))
			closeErr := destination.Close()
			written += count
			if copyErr != nil {
				return copyErr
			}
			if count != header.Size {
				return errors.New("archive entry size is incomplete")
			}
			if closeErr != nil {
				return closeErr
			}
		default:
			return errors.New("archive contains a link or special file")
		}
	}
}

type archiveCommit struct {
	staged string
	live   string
	backup string
}

func mergeStaging(fsys *rootfs.FS, stage, destination string) error {
	backupRoot := ".extract-backup-" + randomHex(12)
	if err := fsys.MkdirAll(backupRoot, 0o700); err != nil {
		return err
	}
	commits := make([]archiveCommit, 0)
	createdDestination := false
	if destination != "" {
		if _, err := fsys.Stat(destination); errors.Is(err, fs.ErrNotExist) {
			createdDestination = true
		} else if err != nil {
			_ = fsys.RemoveAll(backupRoot)
			return err
		}
		if err := fsys.MkdirAll(destination, 0o750); err != nil {
			_ = fsys.RemoveAll(backupRoot)
			return err
		}
	}
	if err := commitStaging(fsys, stage, destination, backupRoot, &commits); err != nil {
		rollbackArchiveCommit(fsys, commits)
		if createdDestination {
			_ = fsys.RemoveAll(destination)
		}
		_ = fsys.RemoveAll(backupRoot)
		return err
	}
	if err := fsys.RemoveAll(stage); err != nil {
		rollbackArchiveCommit(fsys, commits)
		if createdDestination {
			_ = fsys.RemoveAll(destination)
		}
		_ = fsys.RemoveAll(backupRoot)
		return err
	}
	return fsys.RemoveAll(backupRoot)
}

func commitStaging(fsys *rootfs.FS, stagedDir, liveDir, backupRoot string, commits *[]archiveCommit) error {
	entries, err := fsys.ReadDir(stagedDir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		staged := path.Join(stagedDir, entry.Name())
		live := path.Join(liveDir, entry.Name())
		liveInfo, statErr := fsys.Stat(live)
		if entry.IsDir() && statErr == nil && liveInfo.IsDir() {
			if err := commitStaging(fsys, staged, live, backupRoot, commits); err != nil {
				return err
			}
			continue
		}
		if statErr != nil && !errors.Is(statErr, fs.ErrNotExist) {
			return statErr
		}
		commit := archiveCommit{staged: staged, live: live}
		if statErr == nil {
			commit.backup = path.Join(backupRoot, live)
			if err := fsys.MkdirAll(path.Dir(commit.backup), 0o700); err != nil {
				return err
			}
			if err := fsys.Rename(live, commit.backup); err != nil {
				return err
			}
		}
		if err := fsys.Rename(staged, live); err != nil {
			if commit.backup != "" {
				_ = fsys.Rename(commit.backup, live)
			}
			return err
		}
		*commits = append(*commits, commit)
	}
	return nil
}

func rollbackArchiveCommit(fsys *rootfs.FS, commits []archiveCommit) {
	for index := len(commits) - 1; index >= 0; index-- {
		commit := commits[index]
		_ = fsys.MkdirAll(path.Dir(commit.staged), 0o700)
		_ = fsys.Rename(commit.live, commit.staged)
		if commit.backup != "" {
			_ = fsys.MkdirAll(path.Dir(commit.live), 0o750)
			_ = fsys.Rename(commit.backup, commit.live)
		}
	}
}

func extractArchive(fsys *rootfs.FS, archiveName, destination string, manager *ServerManager, serverID string) (int64, error) {
	archive, err := fsys.Open(archiveName)
	if err != nil {
		return 0, err
	}
	defer archive.Close()
	info, err := archive.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return 0, errors.New("archive is not a regular file")
	}
	limits := archiveLimits{bytes: envBytes("DAEMON_ARCHIVE_MAX_EXPANDED_BYTES", defaultMaxArchiveBytes), entries: defaultMaxArchiveEntries}
	stage := ".extract-" + randomHex(12)
	if err := fsys.MkdirAll(stage, 0o700); err != nil {
		return 0, err
	}
	ok := false
	defer func() {
		if !ok {
			_ = fsys.RemoveAll(stage)
		}
	}()
	lower := strings.ToLower(archiveName)
	var expanded int64
	if strings.HasSuffix(lower, ".zip") {
		reader, err := zip.NewReader(archive, info.Size())
		if err != nil {
			return 0, err
		}
		expanded, err = validateZip(reader, limits)
		if err != nil {
			return 0, err
		}
		if err := manager.HasSpaceForWriteFS(serverID, expanded, fsys); err != nil {
			return 0, err
		}
		if err := extractZipStaged(fsys, reader, stage, limits); err != nil {
			return 0, err
		}
	} else if strings.HasSuffix(lower, ".tar.gz") || strings.HasSuffix(lower, ".tgz") {
		expanded, err = validateTar(archive, limits)
		if err != nil {
			return 0, err
		}
		if err := manager.HasSpaceForWriteFS(serverID, expanded, fsys); err != nil {
			return 0, err
		}
		if err := extractTarStaged(fsys, archive, stage, limits); err != nil {
			return 0, err
		}
	} else {
		return 0, errors.New("unsupported archive type")
	}
	if err := mergeStaging(fsys, stage, destination); err != nil {
		return 0, err
	}
	ok = true
	return expanded, nil
}

func restrictedIP(ip net.IP) bool {
	return ip == nil || ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsMulticast() || ip.IsUnspecified() || !ip.IsGlobalUnicast()
}

type pinnedResolver struct {
	lookup    func(context.Context, string) ([]net.IPAddr, error)
	mu        sync.Mutex
	addresses map[string][]net.IPAddr
}

func (p *pinnedResolver) validate(ctx context.Context, value *url.URL) error {
	if value == nil || (value.Scheme != "http" && value.Scheme != "https") || value.Hostname() == "" || value.User != nil {
		return errors.New("invalid or disallowed URL")
	}
	addresses, err := p.lookup(ctx, value.Hostname())
	if err != nil || len(addresses) == 0 {
		return errors.New("could not resolve host")
	}
	for _, address := range addresses {
		if restrictedIP(address.IP) {
			return errors.New("requests to private or internal addresses are not allowed")
		}
	}
	p.mu.Lock()
	p.addresses[strings.ToLower(value.Hostname())] = addresses
	p.mu.Unlock()
	return nil
}

func (p *pinnedResolver) dial(ctx context.Context, network, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, err
	}
	p.mu.Lock()
	addresses := append([]net.IPAddr(nil), p.addresses[strings.ToLower(host)]...)
	p.mu.Unlock()
	if len(addresses) == 0 {
		return nil, errors.New("host was not validated")
	}
	dialer := net.Dialer{Timeout: 30 * time.Second}
	var last error
	for _, candidate := range addresses {
		conn, err := dialer.DialContext(ctx, network, net.JoinHostPort(candidate.IP.String(), port))
		if err == nil {
			return conn, nil
		}
		last = err
	}
	return nil, last
}

func securePullClient(ctx context.Context, initial *url.URL) (*http.Client, error) {
	return securePullClientWithLookup(ctx, initial, net.DefaultResolver.LookupIPAddr)
}

func securePullClientWithLookup(ctx context.Context, initial *url.URL, lookup func(context.Context, string) ([]net.IPAddr, error)) (*http.Client, error) {
	pinned := &pinnedResolver{lookup: lookup, addresses: make(map[string][]net.IPAddr)}
	if err := pinned.validate(ctx, initial); err != nil {
		return nil, err
	}
	transport := &http.Transport{Proxy: nil, DialContext: pinned.dial, TLSHandshakeTimeout: 15 * time.Second, ResponseHeaderTimeout: 30 * time.Second}
	return &http.Client{
		Transport: transport,
		Timeout:   2 * time.Minute,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return errors.New("too many redirects")
			}
			return pinned.validate(req.Context(), req.URL)
		},
	}, nil
}

func safePullFilename(value string) (string, error) {
	clean, err := rootfs.Clean(value)
	if err != nil || clean == "" || strings.Contains(clean, "/") {
		return "", errors.New("invalid file name")
	}
	return clean, nil
}

func archiveTree(fsys *rootfs.FS, writer *tar.Writer, source, archiveBase string) error {
	info, err := fsys.Stat(source)
	if err != nil {
		return err
	}
	header, err := tar.FileInfoHeader(info, "")
	if err != nil {
		return err
	}
	header.Name = archiveBase
	if err := writer.WriteHeader(header); err != nil {
		return err
	}
	if info.Mode().IsRegular() {
		file, err := fsys.Open(source)
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(writer, file)
		closeErr := file.Close()
		if copyErr != nil {
			return copyErr
		}
		return closeErr
	}
	if !info.IsDir() {
		return errors.New("archive source contains a link or special file")
	}
	entries, err := fsys.ReadDir(source)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		childSource := path.Join(source, entry.Name())
		childArchive := path.Join(archiveBase, entry.Name())
		if err := archiveTree(fsys, writer, childSource, childArchive); err != nil {
			return err
		}
	}
	return nil
}

func mapFileError(err error) error {
	if errors.Is(err, fs.ErrNotExist) {
		return fs.ErrNotExist
	}
	return fmt.Errorf("filesystem operation failed: %w", err)
}
