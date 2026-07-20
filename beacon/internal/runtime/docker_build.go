package runtime

import (
	"archive/tar"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/docker/docker/api/types"
)

const maxBuildLogBytes = 256 * 1024
const maxBuildContextBytes = 64 * 1024 * 1024

// Build creates a node-local image from a validated source directory. The
// caller owns source acquisition; this method never follows symlinks or sends
// the repository metadata directory into Docker's build context.
func (r *DockerRuntime) Build(ctx context.Context, request BuildRequest) (BuildResult, error) {
	if r == nil || r.client == nil {
		return BuildResult{}, errors.New("docker runtime is not initialized")
	}
	contextDir, dockerfile, err := validateBuildRequest(request)
	if err != nil {
		return BuildResult{}, err
	}
	archive, err := buildContextArchive(contextDir)
	if err != nil {
		return BuildResult{}, err
	}
	defer archive.Close()
	args := make(map[string]*string, len(request.BuildArgs))
	for key, value := range request.BuildArgs {
		if strings.TrimSpace(key) == "" {
			return BuildResult{}, errors.New("docker build argument name is required")
		}
		valueCopy := value
		args[key] = &valueCopy
	}
	response, err := r.client.ImageBuild(ctx, archive, types.ImageBuildOptions{
		Dockerfile:  dockerfile,
		Tags:        []string{request.ImageTag},
		BuildArgs:   args,
		Remove:      true,
		ForceRemove: true,
	})
	if err != nil {
		return BuildResult{}, fmt.Errorf("start docker build: %w", err)
	}
	defer response.Body.Close()
	logBytes, err := io.ReadAll(io.LimitReader(response.Body, maxBuildLogBytes))
	if err != nil {
		return BuildResult{}, fmt.Errorf("read docker build output: %w", err)
	}
	log := string(logBytes)
	if message := dockerBuildError(log); message != "" {
		return BuildResult{}, errors.New(message)
	}
	return BuildResult{Image: request.ImageTag, Log: log}, nil
}

func validateBuildRequest(request BuildRequest) (string, string, error) {
	if strings.TrimSpace(request.ImageTag) == "" {
		return "", "", errors.New("docker build image tag is required")
	}
	root, err := filepath.Abs(strings.TrimSpace(request.ContextDir))
	if err != nil {
		return "", "", err
	}
	info, err := os.Stat(root)
	if err != nil || !info.IsDir() {
		return "", "", errors.New("docker build context directory is unavailable")
	}
	dockerfile := strings.TrimSpace(request.DockerfilePath)
	if dockerfile == "" {
		dockerfile = "Dockerfile"
	}
	if filepath.IsAbs(dockerfile) || dockerfile == ".." || strings.HasPrefix(filepath.Clean(dockerfile), ".."+string(filepath.Separator)) {
		return "", "", errors.New("dockerfile path must stay inside the build context")
	}
	if info, err := os.Stat(filepath.Join(root, dockerfile)); err != nil || info.IsDir() {
		return "", "", errors.New("dockerfile was not found in the build context")
	}
	return root, filepath.ToSlash(dockerfile), nil
}

func buildContextArchive(root string) (io.ReadCloser, error) {
	buffer := &bytes.Buffer{}
	writer := tar.NewWriter(buffer)
	var totalBytes int64
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if relative == "." {
			return nil
		}
		if relative == ".git" || strings.HasPrefix(relative, ".git"+string(filepath.Separator)) {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("symlinks are not permitted in Docker build context: %s", relative)
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		header.Name = filepath.ToSlash(relative)
		if info.IsDir() {
			header.Name += "/"
		}
		if err := writer.WriteHeader(header); err != nil {
			return err
		}
		if info.Mode().IsRegular() {
			totalBytes += info.Size()
			if totalBytes > maxBuildContextBytes {
				return fmt.Errorf("Docker build context exceeds %d MiB", maxBuildContextBytes/(1024*1024))
			}
			file, err := os.Open(path)
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
		return nil
	})
	if closeErr := writer.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return nil, err
	}
	return io.NopCloser(bytes.NewReader(buffer.Bytes())), nil
}

func dockerBuildError(log string) string {
	for _, line := range strings.Split(log, "\n") {
		if !strings.Contains(line, `"error"`) {
			continue
		}
		if marker := `"error":"`; strings.Contains(line, marker) {
			message := strings.SplitN(line, marker, 2)[1]
			message = strings.SplitN(message, `"`, 2)[0]
			if message != "" {
				return "docker build failed: " + message
			}
		}
	}
	return ""
}

var _ Builder = (*DockerRuntime)(nil)
