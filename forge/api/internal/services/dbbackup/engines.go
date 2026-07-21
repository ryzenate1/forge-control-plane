package dbbackup

import (
	"fmt"
	"strings"
)

type EngineBackupCommand struct {
	Tool       string
	Args       []string
	RestoreCmd string
	RestoreArgs []string
	Extension  string
}

func backupCommandForEngine(engine, host string, port int, username, password, database, outputFile string) (string, []string) {
	switch strings.ToLower(engine) {
	case "postgresql", "postgres":
		return pgDumpCommand(host, port, username, password, database, outputFile)
	case "mysql", "mariadb":
		return mysqldumpCommand(host, port, username, password, database, outputFile)
	case "mongodb":
		return mongodumpCommand(host, port, username, password, database, outputFile)
	case "redis":
		return redisDumpCommand(host, port, password, outputFile)
	default:
		return "", nil
	}
}

func restoreCommandForEngine(engine, host string, port int, username, password, database, inputFile string) (string, []string) {
	switch strings.ToLower(engine) {
	case "postgresql", "postgres":
		return pgRestoreCommand(host, port, username, password, database, inputFile)
	case "mysql", "mariadb":
		return mysqlRestoreCommand(host, port, username, password, database, inputFile)
	case "mongodb":
		return mongorestoreCommand(host, port, username, password, database, inputFile)
	case "redis":
		return redisRestoreCommand(host, port, password, inputFile)
	default:
		return "", nil
	}
}

func pgDumpCommand(host string, port int, username, password, database, outputFile string) (string, []string) {
	return "pg_dump", []string{
		"-h", host,
		"-p", fmt.Sprintf("%d", port),
		"-U", username,
		"-d", database,
		"-F", "c",
		"-f", outputFile,
		"-v",
	}
}

func pgRestoreCommand(host string, port int, username, password, database, inputFile string) (string, []string) {
	return "pg_restore", []string{
		"-h", host,
		"-p", fmt.Sprintf("%d", port),
		"-U", username,
		"-d", database,
		"-c",
		"-v",
		inputFile,
	}
}

func mysqldumpCommand(host string, port int, username, password, database, outputFile string) (string, []string) {
	return "mysqldump", []string{
		"-h", host,
		"-P", fmt.Sprintf("%d", port),
		"-u", username,
		fmt.Sprintf("-p%s", password),
		database,
		"-r", outputFile,
		"--single-transaction",
		"--quick",
	}
}

func mysqlRestoreCommand(host string, port int, username, password, database, inputFile string) (string, []string) {
	return "mysql", []string{
		"-h", host,
		"-P", fmt.Sprintf("%d", port),
		"-u", username,
		fmt.Sprintf("-p%s", password),
		database,
		"-e", fmt.Sprintf("source %s", inputFile),
	}
}

func mongodumpCommand(host string, port int, username, password, database, outputDir string) (string, []string) {
	return "mongodump", []string{
		"--host", host,
		"--port", fmt.Sprintf("%d", port),
		"-u", username,
		"-p", password,
		"--db", database,
		"--out", outputDir,
	}
}

func mongorestoreCommand(host string, port int, username, password, database, inputDir string) (string, []string) {
	return "mongorestore", []string{
		"--host", host,
		"--port", fmt.Sprintf("%d", port),
		"-u", username,
		"-p", password,
		"--db", database,
		"--drop",
		inputDir,
	}
}

func redisDumpCommand(host string, port int, password, outputFile string) (string, []string) {
	return "redis-cli", []string{
		"-h", host,
		"-p", fmt.Sprintf("%d", port),
		"-a", password,
		"--rdb", outputFile,
	}
}

func redisRestoreCommand(host string, port int, password, inputFile string) (string, []string) {
	return "redis-cli", []string{
		"-h", host,
		"-p", fmt.Sprintf("%d", port),
		"-a", password,
		"--pipe",
		"<", inputFile,
	}
}

func dockerExecCommand(containerID, tool string, args []string) (string, []string) {
	execArgs := []string{"exec", "-i", containerID, tool}
	execArgs = append(execArgs, args...)
	return "docker", execArgs
}
