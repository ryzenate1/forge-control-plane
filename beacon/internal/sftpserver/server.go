package sftpserver

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"gamepanel/beacon/internal/rootfs"
	"gamepanel/beacon/internal/server"
	"gamepanel/beacon/internal/serverid"
	"gamepanel/beacon/internal/system"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
	"golang.org/x/time/rate"
)

// Username validation regex (format: username.server_id)
// This prevents unnecessary database/API connection scans by failing fast
var validUsernameRegexp = regexp.MustCompile(`^(?i)([a-z0-9_.@-]{1,128})\.([a-z0-9-]{8,36})$`)

type AuthResult struct {
	UserID      string   `json:"user"`
	ServerID    string   `json:"server"`
	Permissions []string `json:"permissions"`
	DiskLimitMB int64    `json:"diskLimitMb"`
	Suspended   bool     `json:"suspended"`
	ReadOnly    bool     `json:"readOnly"`
}

type SessionRegistry interface {
	TrackSession(userID, serverID string, closer io.Closer) func()
}

type Server struct {
	Addr               string
	DataDir            string
	PanelAPIURL        string
	NodeToken          string
	HTTPClient         *http.Client
	ReadOnly           bool
	IdleTimeout        time.Duration
	MaxConnections     int
	MaxSessionsPerUser int
	MaxSessionLifetime time.Duration
	HostKeyPassphrase  string
	Activity           *system.ActivityDedup
	Sessions           SessionRegistry

	mu           sync.Mutex
	connections  int
	userSessions map[string]int
	writeLocks   sync.Map
	listener     net.Listener
	active       map[net.Conn]struct{}
	wg           sync.WaitGroup
	authMu       sync.Mutex
	authLimiters map[string]*authVisitor
}

type authVisitor struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

func (s *Server) Run(ctx context.Context) error {
	if s.Addr == "" {
		s.Addr = ":2022"
	}
	if s.HTTPClient == nil {
		s.HTTPClient = &http.Client{Timeout: 10 * time.Second}
	}
	if s.IdleTimeout <= 0 {
		s.IdleTimeout = 15 * time.Minute
	}
	if s.MaxConnections <= 0 {
		s.MaxConnections = 128
	}
	if s.MaxSessionsPerUser <= 0 {
		s.MaxSessionsPerUser = 8
	}
	if s.MaxSessionLifetime <= 0 {
		s.MaxSessionLifetime = 24 * time.Hour
	}
	signer, err := loadOrCreateHostKey(filepath.Join(s.DataDir, ".sftp", "id_ed25519"), []byte(s.HostKeyPassphrase))
	if err != nil {
		return err
	}
	config := &ssh.ServerConfig{
		PasswordCallback:  s.passwordCallback,
		PublicKeyCallback: s.publicKeyCallback,
		Config: ssh.Config{
			KeyExchanges: []string{"curve25519-sha256", "ecdh-sha2-nistp256", "diffie-hellman-group16-sha512"},
			Ciphers:      []string{"chacha20-poly1305@openssh.com", "aes256-gcm@openssh.com", "aes128-gcm@openssh.com"},
			MACs:         []string{"hmac-sha2-512-etm@openssh.com", "hmac-sha2-256-etm@openssh.com"},
		},
	}
	config.AddHostKey(signer)
	listener, err := net.Listen("tcp", s.Addr)
	if err != nil {
		return err
	}
	s.mu.Lock()
	s.Addr = listener.Addr().String()
	s.listener = listener
	s.active = make(map[net.Conn]struct{})
	if s.userSessions == nil {
		s.userSessions = make(map[string]int)
	}
	s.mu.Unlock()
	go func() { <-ctx.Done(); s.shutdown() }()
	log.Printf("native sftp listening on %s", listener.Addr())
	for {
		raw, err := listener.Accept()
		if err != nil {
			if ctx.Err() != nil {
				s.wg.Wait()
				return nil
			}
			return err
		}
		if !s.acquireConnection(raw) {
			_ = raw.Close()
			continue
		}
		conn := &idleConn{Conn: raw, timeout: s.IdleTimeout, absoluteDeadline: time.Now().Add(s.MaxSessionLifetime)}
		s.wg.Add(1)
		go func() { defer s.wg.Done(); defer s.releaseConnection(raw); s.handleConn(conn, config) }()
	}
}

func (s *Server) Address() string { s.mu.Lock(); defer s.mu.Unlock(); return s.Addr }

func (s *Server) shutdown() {
	s.mu.Lock()
	if s.listener != nil {
		_ = s.listener.Close()
	}
	connections := make([]net.Conn, 0, len(s.active))
	for conn := range s.active {
		connections = append(connections, conn)
	}
	s.mu.Unlock()
	for _, conn := range connections {
		_ = conn.Close()
	}
}

func (s *Server) acquireConnection(conn net.Conn) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.connections >= s.MaxConnections {
		return false
	}
	s.connections++
	if s.active == nil {
		s.active = make(map[net.Conn]struct{})
	}
	s.active[conn] = struct{}{}
	return true
}
func (s *Server) releaseConnection(conn net.Conn) {
	s.mu.Lock()
	if s.connections > 0 {
		s.connections--
	}
	delete(s.active, conn)
	s.mu.Unlock()
}
func (s *Server) acquireUser(user string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.userSessions[user] >= s.MaxSessionsPerUser {
		return false
	}
	s.userSessions[user]++
	return true
}
func (s *Server) releaseUser(user string) {
	s.mu.Lock()
	if s.userSessions[user] > 0 {
		s.userSessions[user]--
	}
	s.mu.Unlock()
}

func (s *Server) passwordCallback(meta ssh.ConnMetadata, password []byte) (*ssh.Permissions, error) {
	if !s.allowAuthentication(remoteIP(meta.RemoteAddr())) {
		return nil, errors.New("too many authentication attempts")
	}
	if !validUsernameRegexp.MatchString(meta.User()) {
		return nil, errors.New("invalid username format")
	}
	result, err := s.authenticateCredential(meta.User(), "password", string(password), remoteIP(meta.RemoteAddr()))
	if err != nil {
		return nil, err
	}
	return authPermissions(result), nil
}

func (s *Server) publicKeyCallback(meta ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error) {
	if !s.allowAuthentication(remoteIP(meta.RemoteAddr())) {
		return nil, errors.New("too many authentication attempts")
	}
	if !validUsernameRegexp.MatchString(meta.User()) {
		return nil, errors.New("invalid username format")
	}
	encoded := strings.TrimSpace(string(ssh.MarshalAuthorizedKey(key)))
	result, err := s.authenticateCredential(meta.User(), "public_key", encoded, remoteIP(meta.RemoteAddr()))
	if err != nil {
		return nil, err
	}
	return authPermissions(result), nil
}

func authPermissions(result AuthResult) *ssh.Permissions {
	return &ssh.Permissions{Extensions: map[string]string{"user": result.UserID, "server": result.ServerID, "permissions": strings.Join(result.Permissions, ","), "disk": fmt.Sprint(result.DiskLimitMB), "readonly": fmt.Sprint(result.ReadOnly), "suspended": fmt.Sprint(result.Suspended)}}
}

func (s *Server) authenticate(username, password, ip string) (AuthResult, error) {
	return s.authenticateCredential(username, "password", password, ip)
}
func (s *Server) authenticateCredential(username, authType, credential, ip string) (AuthResult, error) {
	body := map[string]string{"type": authType, "username": username, "ip": ip}
	if authType == "password" {
		body["password"] = credential
	} else {
		body["publicKey"] = credential
	}
	return s.authRequest(body)
}
func (s *Server) recheck(userID, serverID, ip string) (AuthResult, error) {
	return s.authRequest(map[string]string{"type": "check", "user": userID, "server": serverID, "ip": ip})
}
func (s *Server) authRequest(payload map[string]string) (AuthResult, error) {
	if s.PanelAPIURL == "" || s.NodeToken == "" {
		return AuthResult{}, errors.New("sftp panel auth is not configured")
	}
	panelURL, err := url.Parse(s.PanelAPIURL)
	if err != nil || panelURL.Hostname() == "" {
		return AuthResult{}, errors.New("sftp panel URL is invalid")
	}
	if panelURL.Scheme != "https" {
		hostIP := net.ParseIP(panelURL.Hostname())
		if panelURL.Hostname() != "localhost" && (hostIP == nil || !hostIP.IsLoopback()) {
			return AuthResult{}, errors.New("sftp panel authentication requires HTTPS")
		}
	}
	body, _ := json.Marshal(payload)
	base := strings.TrimSuffix(strings.TrimSuffix(strings.TrimRight(s.PanelAPIURL, "/"), "/api/remote"), "/api/v1")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/api/remote/sftp/auth", bytes.NewReader(body))
	if err != nil {
		return AuthResult{}, err
	}
	req.Header.Set("Authorization", "Bearer "+s.NodeToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	// The panel's /api/remote middleware requires a signed request (method,
	// URI, timestamp, nonce, and body) with a fresh nonce to prevent replay.
	// The scheme matches beacon/internal/remote/client.go.
	nonceBytes := make([]byte, 16)
	if _, err := rand.Read(nonceBytes); err != nil {
		return AuthResult{}, fmt.Errorf("generate panel request nonce: %w", err)
	}
	timestamp := time.Now().UTC().Format(time.RFC3339)
	nonce := hex.EncodeToString(nonceBytes)
	mac := hmac.New(sha256.New, []byte(s.NodeToken))
	_, _ = io.WriteString(mac, req.Method+"\n"+req.URL.RequestURI()+"\n"+timestamp+"\n"+nonce+"\n")
	_, _ = mac.Write(body)
	req.Header.Set("X-Panel-Timestamp", timestamp)
	req.Header.Set("X-Panel-Nonce", nonce)
	req.Header.Set("X-Panel-Signature", hex.EncodeToString(mac.Sum(nil)))
	res, err := s.HTTPClient.Do(req)
	if err != nil {
		return AuthResult{}, err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return AuthResult{}, errors.New("sftp access denied")
	}
	var result AuthResult
	if err := json.NewDecoder(res.Body).Decode(&result); err != nil {
		return AuthResult{}, err
	}
	if err := serverid.Validate(result.ServerID); err != nil {
		return AuthResult{}, fmt.Errorf("sftp auth returned invalid server id: %w", err)
	}
	if result.Suspended {
		return AuthResult{}, errors.New("server is suspended")
	}
	return result, nil
}

func (s *Server) allowAuthentication(ip string) bool {
	now := time.Now()
	s.authMu.Lock()
	defer s.authMu.Unlock()
	if s.authLimiters == nil {
		s.authLimiters = make(map[string]*authVisitor)
	}
	if visitor := s.authLimiters[ip]; visitor != nil {
		visitor.lastSeen = now
		return visitor.limiter.Allow()
	}
	if len(s.authLimiters) >= 10_000 {
		for candidate, visitor := range s.authLimiters {
			if now.Sub(visitor.lastSeen) > 15*time.Minute {
				delete(s.authLimiters, candidate)
			}
		}
		if len(s.authLimiters) >= 10_000 {
			return false
		}
	}
	visitor := &authVisitor{limiter: rate.NewLimiter(rate.Every(5*time.Second), 5), lastSeen: now}
	s.authLimiters[ip] = visitor
	return visitor.limiter.Allow()
}

func (s *Server) handleConn(raw net.Conn, config *ssh.ServerConfig) {
	conn, channels, requests, err := ssh.NewServerConn(raw, config)
	if err != nil {
		_ = raw.Close()
		return
	}
	defer conn.Close()
	go ssh.DiscardRequests(requests)

	// Validate username format before making remote requests (security best practice)
	username := conn.User()
	if !validUsernameRegexp.MatchString(username) {
		log.Printf("SFTP: rejected invalid username format: %s from %s", username, raw.RemoteAddr())
		return
	}

	userID, serverID := conn.Permissions.Extensions["user"], conn.Permissions.Extensions["server"]
	if !s.acquireUser(userID) {
		return
	}
	defer s.releaseUser(userID)
	untrack := func() {}
	if s.Sessions != nil {
		untrack = s.Sessions.TrackSession(userID, serverID, conn)
	}
	defer untrack()
	for channel := range channels {
		if channel.ChannelType() != "session" {
			_ = channel.Reject(ssh.UnknownChannelType, "unsupported channel type")
			continue
		}
		accepted, channelRequests, err := channel.Accept()
		if err != nil {
			continue
		}
		go s.handleChannel(conn, accepted, channelRequests)
	}
}

func (s *Server) handleChannel(conn *ssh.ServerConn, accepted ssh.Channel, requests <-chan *ssh.Request) {
	defer accepted.Close()
	for req := range requests {
		ok := req.Type == "subsystem" && len(req.Payload) >= 4 && string(req.Payload[4:]) == "sftp"
		_ = req.Reply(ok, nil)
		if !ok {
			continue
		}
		userID, serverID := conn.Permissions.Extensions["user"], conn.Permissions.Extensions["server"]
		fresh, err := s.recheck(userID, serverID, remoteIP(conn.RemoteAddr()))
		if err != nil {
			return
		}
		base, err := rootfs.New(s.DataDir)
		if err != nil {
			return
		}
		if err := base.MkdirAll(serverID, 0o750); err != nil {
			base.Close()
			return
		}
		_ = base.Close()
		fsys, err := rootfs.New(filepath.Join(s.DataDir, serverID))
		if err != nil {
			return
		}
		defer fsys.Close()
		diskMB, _ := parseInt64(conn.Permissions.Extensions["disk"])
		if fresh.DiskLimitMB >= 0 {
			diskMB = fresh.DiskLimitMB
		}
		lockValue, _ := s.writeLocks.LoadOrStore(serverID, &sync.Mutex{})
		writeLock, _ := lockValue.(*sync.Mutex)
		h := &handler{root: filepath.Join(s.DataDir, serverID), fsys: fsys, permissions: fresh.Permissions, readOnly: s.ReadOnly || fresh.ReadOnly, quotaBytes: server.MbToBytes(diskMB), writeLock: writeLock, activity: s.Activity, serverID: serverID, userID: userID, ip: remoteIP(conn.RemoteAddr()), client: sanitizeClient(conn.ClientVersion()), sessionID: sessionID(conn)}
		requestServer := sftp.NewRequestServer(accepted, h.handlers())
		_ = requestServer.Serve()
		_ = requestServer.Close()
		return
	}
}

func sessionID(conn *ssh.ServerConn) string {
	sum := sha256.Sum256([]byte(conn.Permissions.Extensions["user"] + "\x00" + conn.RemoteAddr().String() + "\x00" + time.Now().UTC().Format(time.RFC3339Nano)))
	return base64.RawURLEncoding.EncodeToString(sum[:12])
}
func sanitizeClient(value []byte) string {
	text := strings.TrimSpace(string(value))
	if len(text) > 128 {
		text = text[:128]
	}
	return text
}
func parseInt64(value string) (int64, error) {
	var n int64
	_, err := fmt.Sscan(value, &n)
	return n, err
}
func loadOrCreateHostKey(path string, passphrase []byte) (ssh.Signer, error) {
	if len(passphrase) < 16 {
		return nil, errors.New("SFTP host-key passphrase must contain at least 16 bytes")
	}
	body, readErr := os.ReadFile(path)
	if readErr == nil {
		info, statErr := os.Stat(path)
		if statErr != nil {
			return nil, statErr
		}
		if info.Mode().Perm()&0o077 != 0 {
			return nil, errors.New("SFTP host key permissions must be 0600 or stricter")
		}
		signer, parseErr := ssh.ParsePrivateKeyWithPassphrase(body, passphrase)
		if parseErr != nil {
			if _, unencryptedErr := ssh.ParsePrivateKey(body); unencryptedErr == nil {
				return nil, errors.New("existing SFTP host key is unencrypted; move it aside so Beacon can create an encrypted key")
			}
			return nil, parseErr
		}
		return signer, nil
	}
	if !errors.Is(readErr, os.ErrNotExist) {
		return nil, readErr
	}
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	block, err := ssh.MarshalPrivateKeyWithPassphrase(privateKey, "", passphrase)
	if err != nil {
		return nil, err
	}
	body = pem.EncodeToMemory(block)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	temp, err := os.CreateTemp(filepath.Dir(path), ".id_ed25519.tmp-*")
	if err != nil {
		return nil, err
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)
	if err := temp.Chmod(0o600); err != nil {
		_ = temp.Close()
		return nil, err
	}
	if _, err := temp.Write(body); err != nil {
		_ = temp.Close()
		return nil, err
	}
	if err := temp.Sync(); err != nil {
		_ = temp.Close()
		return nil, err
	}
	if err := temp.Close(); err != nil {
		return nil, err
	}
	if err := os.Rename(tempPath, path); err != nil {
		return nil, err
	}
	parent, err := os.Open(filepath.Dir(path))
	if err != nil {
		return nil, err
	}
	syncErr := parent.Sync()
	_ = parent.Close()
	if syncErr != nil {
		return nil, syncErr
	}
	return ssh.NewSignerFromKey(privateKey)
}
func remoteIP(addr net.Addr) string {
	host, _, err := net.SplitHostPort(addr.String())
	if err != nil {
		return addr.String()
	}
	return host
}

type idleConn struct {
	net.Conn
	timeout          time.Duration
	absoluteDeadline time.Time
}

func (c *idleConn) Read(p []byte) (int, error) {
	_ = c.SetDeadline(c.nextDeadline())
	return c.Conn.Read(p)
}
func (c *idleConn) Write(p []byte) (int, error) {
	_ = c.SetDeadline(c.nextDeadline())
	return c.Conn.Write(p)
}

func (c *idleConn) nextDeadline() time.Time {
	idleDeadline := time.Now().Add(c.timeout)
	if !c.absoluteDeadline.IsZero() && c.absoluteDeadline.Before(idleDeadline) {
		return c.absoluteDeadline
	}
	return idleDeadline
}

type handler struct {
	root                                    string
	fsys                                    *rootfs.FS
	permissions                             []string
	readOnly                                bool
	quotaBytes                              int64
	writeLock                               *sync.Mutex
	activity                                *system.ActivityDedup
	serverID, userID, ip, client, sessionID string
}

func (h *handler) filesystem() (*rootfs.FS, error) {
	if h.fsys != nil {
		return h.fsys, nil
	}
	fsys, err := rootfs.New(h.root)
	if err == nil {
		h.fsys = fsys
	}
	return fsys, err
}
func (h *handler) handlers() sftp.Handlers {
	return sftp.Handlers{FileGet: h, FilePut: h, FileCmd: h, FileList: h}
}
func (h *handler) record(action, path string) {
	if h.activity != nil {
		h.activity.RecordDetailed(h.serverID, action, path, h.ip, h.userID, h.client, h.sessionID)
	}
}
func (h *handler) Fileread(request *sftp.Request) (io.ReaderAt, error) {
	request.Filepath = strings.TrimLeft(request.Filepath, "/")
	if !h.can("file.read-content") {
		return nil, sftp.ErrSSHFxPermissionDenied
	}
	fsys, err := h.filesystem()
	if err != nil {
		return nil, sftp.ErrSSHFxFailure
	}
	file, err := fsys.Open(request.Filepath)
	if err != nil {
		return nil, sftp.ErrSSHFxNoSuchFile
	}
	h.record("sftp.file.read", request.Filepath)
	return file, nil
}
func (h *handler) Filewrite(request *sftp.Request) (io.WriterAt, error) {
	if h.readOnly {
		return nil, sftp.ErrSSHFxPermissionDenied
	}
	fsys, err := h.filesystem()
	if err != nil {
		return nil, sftp.ErrSSHFxFailure
	}
	name, err := cleanSFTPPath(request.Filepath)
	if err != nil || name == "" {
		return nil, sftp.ErrSSHFxPermissionDenied
	}
	permission := "file.update"
	oldSize := int64(0)
	if info, statErr := fsys.Stat(name); errors.Is(statErr, os.ErrNotExist) {
		permission = "file.create"
	} else if statErr != nil {
		return nil, sftp.ErrSSHFxFailure
	} else {
		oldSize = info.Size()
	}
	if !h.can(permission) {
		return nil, sftp.ErrSSHFxPermissionDenied
	}
	if h.writeLock == nil {
		h.writeLock = &sync.Mutex{}
	}
	h.writeLock.Lock()
	usage, err := fsys.Usage()
	if err != nil {
		h.writeLock.Unlock()
		return nil, sftp.ErrSSHFxFailure
	}
	maxFinal := int64(-1)
	if h.quotaBytes > 0 {
		maxFinal = h.quotaBytes - usage + oldSize
		if maxFinal < 0 {
			h.writeLock.Unlock()
			return nil, sftp.ErrSSHFxFailure
		}
	}
	parent := filepath.ToSlash(filepath.Dir(name))
	if parent == "." {
		parent = ""
	}
	if err := fsys.MkdirAll(parent, 0o750); err != nil {
		h.writeLock.Unlock()
		return nil, sftp.ErrSSHFxFailure
	}
	file, err := fsys.CreateAtomic(name, 0o640)
	if err != nil {
		h.writeLock.Unlock()
		return nil, sftp.ErrSSHFxFailure
	}
	return &quotaWriter{file: file, max: maxFinal, unlock: h.writeLock.Unlock, onCommit: func() { h.record("sftp.file.write", name) }}, nil
}

type quotaWriter struct {
	file     *rootfs.AtomicFile
	max      int64
	mu       sync.Mutex
	closed   bool
	unlock   func()
	onCommit func()
}

func (w *quotaWriter) WriteAt(p []byte, off int64) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed || off < 0 {
		return 0, sftp.ErrSSHFxFailure
	}
	if w.max >= 0 && (off > w.max || int64(len(p)) > w.max-off) {
		return 0, sftp.ErrSSHFxFailure
	}
	return w.file.WriteAt(p, off)
}
func (w *quotaWriter) Close() error {
	w.mu.Lock()
	if w.closed {
		w.mu.Unlock()
		return nil
	}
	w.closed = true
	err := w.file.Close()
	w.mu.Unlock()
	if w.unlock != nil {
		w.unlock()
	}
	if err == nil && w.onCommit != nil {
		w.onCommit()
	}
	return err
}

func (h *handler) Filecmd(request *sftp.Request) error {
	request.Filepath = strings.TrimLeft(request.Filepath, "/")
	request.Target = strings.TrimLeft(request.Target, "/")
	if h.readOnly {
		return sftp.ErrSSHFxPermissionDenied
	}
	fsys, err := h.filesystem()
	if err != nil {
		return sftp.ErrSSHFxFailure
	}
	switch request.Method {
	case "Mkdir":
		if !h.can("file.create") {
			return sftp.ErrSSHFxPermissionDenied
		}
		err = fsys.MkdirAll(request.Filepath, 0o750)
	case "Remove", "Rmdir":
		if !h.can("file.delete") {
			return sftp.ErrSSHFxPermissionDenied
		}
		if h.writeLock != nil {
			h.writeLock.Lock()
			defer h.writeLock.Unlock()
		}
		err = fsys.RemoveAll(request.Filepath)
	case "Rename":
		if !h.can("file.update") {
			return sftp.ErrSSHFxPermissionDenied
		}
		to, cleanErr := rootfs.Clean(request.Target)
		if cleanErr != nil || to == "" {
			return sftp.ErrSSHFxPermissionDenied
		}
		parent := filepath.ToSlash(filepath.Dir(to))
		if parent == "." {
			parent = ""
		}
		if err = fsys.MkdirAll(parent, 0o750); err == nil {
			err = fsys.Rename(request.Filepath, to)
		}
	case "Setstat":
		if !h.can("file.update") {
			return sftp.ErrSSHFxPermissionDenied
		}
		flags := request.AttrFlags()
		if !flags.Size && !flags.UidGid && !flags.Permissions && !flags.Acmodtime {
			return sftp.ErrSSHFxOpUnsupported
		}
		if flags.Size || flags.UidGid {
			return sftp.ErrSSHFxOpUnsupported
		}
		attrs := request.Attributes()
		if flags.Permissions {
			err = fsys.Chmod(request.Filepath, attrs.FileMode())
		}
		if err == nil && flags.Acmodtime {
			err = fsys.Chtimes(request.Filepath, attrs.AccessTime(), attrs.ModTime())
		}
	default:
		return sftp.ErrSSHFxOpUnsupported
	}
	if err != nil {
		return sftp.ErrSSHFxFailure
	}
	h.record("sftp.file."+strings.ToLower(request.Method), request.Filepath)
	return sftp.ErrSSHFxOk
}
func (h *handler) Filelist(request *sftp.Request) (sftp.ListerAt, error) {
	request.Filepath = strings.TrimLeft(request.Filepath, "/")
	if !h.can("file.read") {
		return nil, sftp.ErrSSHFxPermissionDenied
	}
	fsys, err := h.filesystem()
	if err != nil {
		return nil, sftp.ErrSSHFxFailure
	}
	switch request.Method {
	case "List":
		entries, err := fsys.ReadDir(request.Filepath)
		if err != nil {
			return nil, sftp.ErrSSHFxNoSuchFile
		}
		infos := make([]os.FileInfo, 0, len(entries))
		for _, entry := range entries {
			if info, err := entry.Info(); err == nil {
				infos = append(infos, info)
			}
		}
		return listerAt(infos), nil
	case "Stat":
		info, err := fsys.Stat(request.Filepath)
		if err != nil {
			return nil, sftp.ErrSSHFxNoSuchFile
		}
		return listerAt([]os.FileInfo{info}), nil
	}
	return nil, sftp.ErrSSHFxOpUnsupported
}
func cleanSFTPPath(requested string) (string, error) {
	return rootfs.Clean(strings.TrimLeft(requested, "/"))
}
func (h *handler) safePath(requested string) (string, error) { return rootfs.Clean(requested) }
func (h *handler) can(permission string) bool {
	for _, grant := range h.permissions {
		if grant == "*" || grant == permission {
			return true
		}
	}
	return false
}

type listerAt []os.FileInfo

func (l listerAt) ListAt(out []os.FileInfo, offset int64) (int, error) {
	if offset >= int64(len(l)) {
		return 0, io.EOF
	}
	count := copy(out, l[offset:])
	if count < len(out) {
		return count, io.EOF
	}
	return count, nil
}
func (s *Server) String() string { return fmt.Sprintf("sftp[%s]", s.Addr) }
