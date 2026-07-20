package dbprovisioner

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"gamepanel/forge/internal/daemon"
	"gamepanel/forge/internal/store"
)

type DBContainerService struct {
	store          *store.Store
	daemon         *daemon.Client
	beaconBaseURL  string
	nodeToken      string
	dockerHost     string
	provisionLimit map[string]int
}

func NewDBContainerService(s *store.Store, dc *daemon.Client, beaconBaseURL, nodeToken, dockerHost string) *DBContainerService {
	return &DBContainerService{
		store:         s,
		daemon:        dc,
		beaconBaseURL: strings.TrimRight(beaconBaseURL, "/"),
		nodeToken:     nodeToken,
		dockerHost:    dockerHost,
		provisionLimit: map[string]int{
			"postgresql": 16,
			"mysql":      16,
			"mariadb":    16,
			"redis":      16,
			"mongodb":    16,
		},
	}
}

func generatePassword(length int) string {
	b := make([]byte, length)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)[:length]
}

func generateDBName() string {
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	return "db_" + hex.EncodeToString(b)[:8]
}

func generateUsername() string {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	return "u_" + hex.EncodeToString(b)[:8]
}

func imageForDB(engine, version string) string {
	image, ok := store.DBEngineImages[strings.ToLower(engine)]
	if !ok {
		image = engine
	}
	return fmt.Sprintf("%s:%s", image, version)
}

func envVarsForDB(engine, dbName, username, password string) []string {
	switch strings.ToLower(engine) {
	case "postgresql":
		return []string{
			"POSTGRES_DB=" + dbName,
			"POSTGRES_USER=" + username,
			"POSTGRES_PASSWORD=" + password,
		}
	case "mysql", "mariadb":
		return []string{
			"MYSQL_DATABASE=" + dbName,
			"MYSQL_USER=" + username,
			"MYSQL_PASSWORD=" + password,
			"MYSQL_ROOT_PASSWORD=" + password,
		}
	case "mongodb":
		return []string{
			"MONGO_INITDB_DATABASE=" + dbName,
			"MONGO_INITDB_ROOT_USERNAME=" + username,
			"MONGO_INITDB_ROOT_PASSWORD=" + password,
		}
	case "redis":
		return []string{
			"REDIS_PASSWORD=" + password,
		}
	default:
		return nil
	}
}

func defaultPortForEngine(engine string) int {
	if port, ok := store.DBEngineDefaultPorts[strings.ToLower(engine)]; ok {
		return port
	}
	return 0
}

func connectionStringForDB(engine, dbName, username, password, host string, port int, version string) string {
	switch strings.ToLower(engine) {
	case "postgresql":
		return fmt.Sprintf("postgresql://%s:%s@%s:%d/%s?sslmode=disable", username, password, host, port, dbName)
	case "mysql":
		return fmt.Sprintf("mysql://%s:%s@%s:%d/%s", username, password, host, port, dbName)
	case "mariadb":
		return fmt.Sprintf("mariadb://%s:%s@%s:%d/%s", username, password, host, port, dbName)
	case "mongodb":
		return fmt.Sprintf("mongodb://%s:%s@%s:%d/%s", username, password, host, port, dbName)
	case "redis":
		return fmt.Sprintf("redis://:%s@%s:%d", password, host, port)
	default:
		return ""
	}
}

func credentialsJSON(engine, dbName, username, password string) (json.RawMessage, error) {
	creds := map[string]string{
		"username": username,
		"password": password,
	}
	switch strings.ToLower(engine) {
	case "postgresql", "mysql", "mariadb", "mongodb":
		creds["database"] = dbName
	}
	raw, err := json.Marshal(creds)
	if err != nil {
		return nil, err
	}
	return raw, nil
}

func (s *DBContainerService) SupportedEngines() map[string][]string {
	return store.SupportedDBEngines
}

func (s *DBContainerService) Provision(ctx context.Context, serverID, engine, version string, memoryMB, cpuShares int) (store.DBContainer, error) {
	engine = strings.ToLower(strings.TrimSpace(engine))
	version = strings.TrimSpace(version)
	if err := store.ValidateDBEngine(engine, version); err != nil {
		return store.DBContainer{}, err
	}
	if s.daemon == nil {
		return store.DBContainer{}, errors.New("beacon client is not available for container provisioning")
	}
	req := store.CreateDBContainerRequest{
		ServerID:  serverID,
		Engine:    engine,
		Version:   version,
		MemoryMB:  memoryMB,
		CPUShares: cpuShares,
	}
	db, err := s.store.CreateDBContainer(ctx, req)
	if err != nil {
		return store.DBContainer{}, fmt.Errorf("create db container record: %w", err)
	}
	dbName := generateDBName()
	username := generateUsername()
	password := generatePassword(32)
	volumeName := "mgp-db-" + db.ID[:12]
	port := defaultPortForEngine(engine)

	daemonReq := daemon.DBContainerProvisionRequest{
		ServerID:   serverID,
		Engine:     engine,
		Version:    version,
		MemoryMB:   memoryMB,
		CPUShares:  cpuShares,
		DBName:     dbName,
		Username:   username,
		Password:   password,
		Port:       port,
		VolumeName: volumeName,
	}
	resp, err := s.daemon.ProvisionDatabase(ctx, s.beaconBaseURL, s.nodeToken, daemonReq)
	if err != nil {
		_ = s.store.SetDBContainerStatus(ctx, db.ID, "", "failed", 0, "", "", nil)
		return store.DBContainer{}, fmt.Errorf("provision container via beacon: %w", err)
	}
	connStr := connectionStringForDB(engine, dbName, username, password, s.dockerHost, resp.Port, version)
	creds, _ := credentialsJSON(engine, dbName, username, password)
	if err := s.store.SetDBContainerStatus(ctx, db.ID, resp.ContainerID, "running", resp.Port, resp.VolumeID, connStr, creds); err != nil {
		return store.DBContainer{}, fmt.Errorf("update container status: %w", err)
	}
	return s.store.GetDBContainer(ctx, db.ID)
}

func (s *DBContainerService) Deprovision(ctx context.Context, containerID string) error {
	db, err := s.store.GetDBContainer(ctx, containerID)
	if err != nil {
		return err
	}
	if s.daemon != nil && db.ContainerID != "" {
		_ = s.daemon.DeProvisionDatabase(ctx, s.beaconBaseURL, s.nodeToken, db.ContainerID, db.VolumeID)
	}
	return s.store.DeleteDBContainer(ctx, containerID)
}

func (s *DBContainerService) Restart(ctx context.Context, containerID string) error {
	db, err := s.store.GetDBContainer(ctx, containerID)
	if err != nil {
		return err
	}
	if db.ContainerID == "" {
		return errors.New("container not yet provisioned")
	}
	return s.store.SetDBContainerStatus(ctx, containerID, "", "running", 0, "", "", nil)
}

func (s *DBContainerService) Backup(ctx context.Context, containerID string) error {
	db, err := s.store.GetDBContainer(ctx, containerID)
	if err != nil {
		return err
	}
	if db.ContainerID == "" {
		return errors.New("container not yet provisioned")
	}
	if s.daemon != nil {
		return s.daemon.BackupDatabase(ctx, s.beaconBaseURL, s.nodeToken, db.ContainerID, db.Engine)
	}
	return errors.New("beacon client not available")
}

func (s *DBContainerService) Status(ctx context.Context, containerID string) (string, error) {
	db, err := s.store.GetDBContainer(ctx, containerID)
	if err != nil {
		return "", err
	}
	if db.ContainerID == "" {
		return "pending", nil
	}
	return db.Status, nil
}
