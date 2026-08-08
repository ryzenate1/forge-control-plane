package dbprovisioner

import (
	"context"
	"database/sql"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"gamepanel/forge/internal/store"
)

type fakeStore struct {
	host          store.DatabaseHost
	hostPassword  string
	database      store.ServerDatabase
	candidate     string
	commitErr     error
	stateErr      error
	events        []string
	eventSink     *[]string
	states        []string
	failureDetail string
}

func (f *fakeStore) GetDatabaseHostForServerDatabase(context.Context, string) (store.DatabaseHost, string, error) {
	return f.host, f.hostPassword, nil
}
func (f *fakeStore) GetServerDatabaseForProvisioning(context.Context, string, string) (store.ServerDatabase, error) {
	return f.database, nil
}
func (f *fakeStore) SetServerDatabaseProvisioningState(_ context.Context, _, _, state, detail string) error {
	f.events = append(f.events, "state:"+state)
	f.states = append(f.states, state)
	f.failureDetail = detail
	return f.stateErr
}
func (f *fakeStore) NewServerDatabasePasswordCandidate() string { return f.candidate }
func (f *fakeStore) CommitServerDatabasePassword(_ context.Context, _, _, oldPassword, newPassword string, _ *string) error {
	event := "commit:" + oldPassword + "->" + newPassword
	f.events = append(f.events, event)
	if f.eventSink != nil {
		*f.eventSink = append(*f.eventSink, event)
	}
	return f.commitErr
}

type fakeAdmin struct {
	events      *[]string
	failExec    string
	failExecAll map[string]error
	pingErr     error
}

func (f *fakeAdmin) PingContext(ctx context.Context) error {
	*f.events = append(*f.events, "ping")
	if _, ok := ctx.Deadline(); !ok {
		return errors.New("ping context has no deadline")
	}
	return f.pingErr
}
func (f *fakeAdmin) ExecContext(_ context.Context, query string, _ ...any) (sql.Result, error) {
	*f.events = append(*f.events, query)
	if f.failExec != "" && strings.Contains(query, f.failExec) {
		f.failExec = ""
		return nil, errors.New("injected exec failure")
	}
	for fragment, err := range f.failExecAll {
		if strings.Contains(query, fragment) {
			return nil, err
		}
	}
	return fakeResult(1), nil
}
func (f *fakeAdmin) QueryRowContext(context.Context, string, ...any) rowScanner {
	return fakeRow{value: false}
}
func (f *fakeAdmin) BeginTx(context.Context, *sql.TxOptions) (adminTx, error) {
	return &fakeTx{admin: f}, nil
}
func (f *fakeAdmin) Close() error { return nil }

type fakeTx struct{ admin *fakeAdmin }

func (f *fakeTx) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	return f.admin.ExecContext(ctx, query, args...)
}
func (f *fakeTx) QueryRowContext(context.Context, string, ...any) rowScanner {
	return fakeRow{value: false}
}
func (f *fakeTx) Commit() error {
	*f.admin.events = append(*f.admin.events, "commit-tx")
	return nil
}
func (f *fakeTx) Rollback() error { return nil }

type fakeRow struct{ value bool }

func (r fakeRow) Scan(dest ...any) error {
	*(dest[0].(*bool)) = r.value
	return nil
}

type fakeResult int64

func (r fakeResult) LastInsertId() (int64, error) { return 0, nil }
func (r fakeResult) RowsAffected() (int64, error) { return int64(r), nil }

func mysqlFixture(state string) (*fakeStore, *fakeAdmin, *Service) {
	password := "old-secret"
	events := []string{}
	fs := &fakeStore{
		host:      store.DatabaseHost{ID: "host-1", Engine: "mysql", Host: "db.example.com", Port: 3306, Username: "admin", TLSMode: "required"},
		database:  store.ServerDatabase{ID: "db-1", DatabaseName: "s12345678_game", Username: "u12345678_game", Remote: "%", ProvisioningState: state, Password: &password},
		candidate: "new-secret",
		eventSink: &events,
	}
	admin := &fakeAdmin{events: &events, failExecAll: map[string]error{}}
	service := newServiceForTest(fs, func(store.DatabaseHost, string) (adminDB, error) { return admin, nil })
	return fs, admin, service
}

func TestProvisionFailureCompensatesAndMarksFailed(t *testing.T) {
	fs, admin, service := mysqlFixture(store.DatabaseStatePending)
	admin.failExec = "CREATE USER"

	err := service.Provision(context.Background(), "server-1", "db-1")
	if err == nil {
		t.Fatal("expected provisioning failure")
	}
	events := *admin.events
	assertContainsAfter(t, events, "DROP DATABASE IF EXISTS", "CREATE USER")
	if indexContaining(events, "DROP USER IF EXISTS") >= 0 {
		t.Fatalf("compensation attempted to drop a user that was not created: %#v", events)
	}
	if !reflect.DeepEqual(fs.states, []string{store.DatabaseStateFailed}) {
		t.Fatalf("states = %#v", fs.states)
	}
	if !strings.Contains(fs.failureDetail, "remote provisioning failed") {
		t.Fatalf("failure detail = %q", fs.failureDetail)
	}
}

func TestRotationChangesRemoteBeforeCommitAndRollsBack(t *testing.T) {
	fs, admin, service := mysqlFixture(store.DatabaseStateReady)
	fs.commitErr = errors.New("panel unavailable")

	_, err := service.RotatePassword(context.Background(), "server-1", "db-1", nil)
	if err == nil || !strings.Contains(err.Error(), "rolled back") {
		t.Fatalf("unexpected error: %v", err)
	}
	firstAlter := indexContaining(*admin.events, "'new-secret'")
	commit := indexContaining(*admin.events, "commit:old-secret->new-secret")
	rollback := indexContaining(*admin.events, "'old-secret'")
	if firstAlter < 0 || commit <= firstAlter || rollback <= commit {
		t.Fatalf("rotation ordering events = %#v", *admin.events)
	}
	if len(fs.events) == 0 || fs.events[0] != "commit:old-secret->new-secret" {
		t.Fatalf("store events = %#v", fs.events)
	}
}

func TestRotationRollbackFailureMarksFailed(t *testing.T) {
	fs, admin, service := mysqlFixture(store.DatabaseStateReady)
	fs.commitErr = errors.New("panel unavailable")
	admin.failExecAll["'old-secret'"] = errors.New("rollback rejected")

	_, err := service.RotatePassword(context.Background(), "server-1", "db-1", nil)
	if err == nil || !strings.Contains(err.Error(), "rollback failed") {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(fs.states) != 1 || fs.states[0] != store.DatabaseStateFailed {
		t.Fatalf("states = %#v", fs.states)
	}
}

func TestDeprovisionFailureDoesNotMutatePanelState(t *testing.T) {
	fs, admin, service := mysqlFixture(store.DatabaseStateReady)
	admin.failExecAll["DROP DATABASE"] = errors.New("host rejected delete")

	if err := service.Deprovision(context.Background(), "server-1", "db-1"); err == nil {
		t.Fatal("expected delete failure")
	}
	if len(fs.states) != 0 || len(fs.events) != 0 {
		t.Fatalf("panel was mutated on remote delete failure: states=%v events=%v", fs.states, fs.events)
	}
}

func TestTestConnectionUsesProvisioningConnector(t *testing.T) {
	fs, admin, service := mysqlFixture(store.DatabaseStatePending)
	if err := service.TestConnection(context.Background(), fs.host, fs.hostPassword); err != nil {
		t.Fatal(err)
	}
	if len(*admin.events) != 1 || (*admin.events)[0] != "ping" {
		t.Fatalf("events = %#v", *admin.events)
	}
}

func TestConnectionDSNUsesTLSAndEscapesCredentials(t *testing.T) {
	host := store.DatabaseHost{ID: "host", Engine: "postgresql", Host: "db.example.com", Port: 5432, Username: "admin@tenant", TLSMode: "verify-full"}
	dsn := connectionDSN(host, "p@ss:/word")
	if strings.Contains(dsn, "p@ss:/word") || !strings.Contains(dsn, "sslmode=verify-full") {
		t.Fatalf("unexpected postgres DSN %q", dsn)
	}
	cfg, err := hostTLSConfig(host)
	if err != nil || cfg == nil || cfg.InsecureSkipVerify || cfg.ServerName != host.Host {
		t.Fatalf("unexpected TLS config: %#v, %v", cfg, err)
	}
	if _, err := connectorForHost(host, "p@ss:/word"); err != nil {
		t.Fatalf("postgres connector: %v", err)
	}

	host.Engine, host.TLSMode = "mysql", "required"
	mysqlDSN := connectionDSN(host, "p@ss:/word")
	if !strings.Contains(mysqlDSN, "tls=gamepanel-") || !strings.Contains(mysqlDSN, "tcp(db.example.com:5432)") {
		t.Fatalf("unexpected mysql DSN %q", mysqlDSN)
	}
	if _, err := connectorForHost(host, "p@ss:/word"); err != nil {
		t.Fatalf("mysql connector: %v", err)
	}
}

func TestIdentifierQuoting(t *testing.T) {
	if got := quotePostgresIdentifier(`a"b`); got != `"a""b"` {
		t.Fatalf("postgres quote = %q", got)
	}
	if got := quoteMySQLIdentifier("a`b"); got != "`a``b`" {
		t.Fatalf("mysql quote = %q", got)
	}
	if got := quoteSQLString("a'b"); got != "'a''b'" {
		t.Fatalf("string quote = %q", got)
	}
}

func TestPingUsesBoundedContext(t *testing.T) {
	_, admin, service := mysqlFixture(store.DatabaseStatePending)
	start := time.Now()
	if err := service.Provision(context.Background(), "server-1", "db-1"); err != nil {
		t.Fatal(err)
	}
	if time.Since(start) > pingTimeout {
		t.Fatal("provision exceeded ping timeout")
	}
	if len(*admin.events) == 0 || (*admin.events)[0] != "ping" {
		t.Fatalf("events = %#v", *admin.events)
	}
}

func assertContainsAfter(t *testing.T, events []string, wanted, after string) {
	t.Helper()
	if indexContaining(events, wanted) <= indexContaining(events, after) {
		t.Fatalf("%q was not after %q in %#v", wanted, after, events)
	}
}
func indexContaining(events []string, fragment string) int {
	for i, event := range events {
		if strings.Contains(event, fragment) {
			return i
		}
	}
	return -1
}
