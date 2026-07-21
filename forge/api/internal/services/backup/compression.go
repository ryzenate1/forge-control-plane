package backup

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/klauspost/compress/zstd"
)

const (
	defaultCompressionLevel    = gzip.DefaultCompression
	compressionAlgorithmGzip  = "gzip"
	compressionAlgorithmZstd  = "zstd"
	compressionAlgorithmDeflate = "deflate"
	defaultCompressionAlgorithm = compressionAlgorithmGzip
)

func getCompressionAlgorithm() string {
	algo := os.Getenv("BACKUP_COMPRESSION_ALGORITHM")
	switch strings.ToLower(strings.TrimSpace(algo)) {
	case compressionAlgorithmZstd, compressionAlgorithmDeflate:
		return strings.ToLower(strings.TrimSpace(algo))
	case compressionAlgorithmGzip, "":
		return compressionAlgorithmGzip
	default:
		return defaultCompressionAlgorithm
	}
}

func IsCompressionEnabled() bool {
	return os.Getenv("BACKUP_COMPRESSION_ENABLED") == "true"
}

func GetCompressionLevel() int {
	raw := os.Getenv("BACKUP_COMPRESSION_LEVEL")
	if raw == "" {
		return defaultCompressionLevel
	}
	algo := getCompressionAlgorithm()
	if algo == compressionAlgorithmZstd || algo == compressionAlgorithmDeflate {
		level, err := strconv.Atoi(raw)
		if err != nil || level < int(zstd.SpeedFastest) || level > int(zstd.SpeedBestCompression) {
			return int(zstd.SpeedDefault)
		}
		return level
	}
	level, err := strconv.Atoi(raw)
	if err != nil || level < gzip.HuffmanOnly || level > gzip.BestCompression {
		return defaultCompressionLevel
	}
	return level
}

func GetCompressionAlgorithmName() string {
	return getCompressionAlgorithm()
}

func CompressReader(r io.Reader, path string) (io.Reader, error) {
	return CompressReaderWithLevel(r, path, GetCompressionLevel())
}

func CompressReaderWithLevel(r io.Reader, path string, level int) (io.Reader, error) {
	algo := getCompressionAlgorithm()
	switch algo {
	case compressionAlgorithmZstd, compressionAlgorithmDeflate:
		return compressReaderZstd(r, level)
	default:
		return compressReaderGzip(r, level)
	}
}

func compressReaderGzip(r io.Reader, level int) (io.Reader, error) {
	pr, pw := io.Pipe()
	gw, err := gzip.NewWriterLevel(pw, level)
	if err != nil {
		return nil, fmt.Errorf("new gzip writer level: %w", err)
	}
	go func() {
		_, err := io.Copy(gw, r)
		cerr := gw.Close()
		if err != nil {
			pw.CloseWithError(fmt.Errorf("compress reader: %w", err))
			return
		}
		if cerr != nil {
			pw.CloseWithError(fmt.Errorf("close gzip writer: %w", cerr))
			return
		}
		pw.Close()
	}()
	return pr, nil
}

func compressReaderZstd(r io.Reader, level int) (io.Reader, error) {
	pr, pw := io.Pipe()
	zw, err := zstd.NewWriter(pw, zstd.WithEncoderLevel(zstd.EncoderLevel(level)))
	if err != nil {
		return nil, fmt.Errorf("new zstd writer: %w", err)
	}
	go func() {
		_, err := io.Copy(zw, r)
		cerr := zw.Close()
		if err != nil {
			pw.CloseWithError(fmt.Errorf("compress reader zstd: %w", err))
			return
		}
		if cerr != nil {
			pw.CloseWithError(fmt.Errorf("close zstd writer: %w", cerr))
			return
		}
		pw.Close()
	}()
	return pr, nil
}

func DecompressReader(r io.Reader, path string) (io.Reader, error) {
	algo := getCompressionAlgorithm()
	switch algo {
	case compressionAlgorithmZstd, compressionAlgorithmDeflate:
		return decompressReaderZstd(r)
	default:
		return decompressReaderGzip(r)
	}
}

func decompressReaderGzip(r io.Reader) (io.Reader, error) {
	gr, err := gzip.NewReader(r)
	if err != nil {
		return nil, fmt.Errorf("decompress reader: %w", err)
	}
	pr, pw := io.Pipe()
	go func() {
		_, err := io.Copy(pw, gr)
		gr.Close()
		if err != nil {
			pw.CloseWithError(fmt.Errorf("decompress reader copy: %w", err))
		} else {
			pw.Close()
		}
	}()
	return pr, nil
}

func decompressReaderZstd(r io.Reader) (io.Reader, error) {
	zr, err := zstd.NewReader(r)
	if err != nil {
		return nil, fmt.Errorf("new zstd reader: %w", err)
	}
	pr, pw := io.Pipe()
	go func() {
		_, err := io.Copy(pw, zr)
		zr.Close()
		if err != nil {
			pw.CloseWithError(fmt.Errorf("decompress reader zstd copy: %w", err))
		} else {
			pw.Close()
		}
	}()
	return pr, nil
}

func Compress(data []byte) ([]byte, error) {
	return CompressWithLevel(data, GetCompressionLevel())
}

func CompressWithLevel(data []byte, level int) ([]byte, error) {
	algo := getCompressionAlgorithm()
	switch algo {
	case compressionAlgorithmZstd, compressionAlgorithmDeflate:
		return compressZstd(data, level)
	default:
		return compressGzip(data, level)
	}
}

func compressGzip(data []byte, level int) ([]byte, error) {
	pr, pw := io.Pipe()
	gw, err := gzip.NewWriterLevel(pw, level)
	if err != nil {
		return nil, fmt.Errorf("new gzip writer level: %w", err)
	}
	go func() {
		_, err := gw.Write(data)
		cerr := gw.Close()
		if err != nil {
			pw.CloseWithError(fmt.Errorf("compress: %w", err))
			return
		}
		if cerr != nil {
			pw.CloseWithError(fmt.Errorf("close gzip: %w", cerr))
			return
		}
		pw.Close()
	}()
	out, err := io.ReadAll(pr)
	if err != nil {
		return nil, fmt.Errorf("read compressed: %w", err)
	}
	return out, nil
}

func compressZstd(data []byte, level int) ([]byte, error) {
	var buf bytes.Buffer
	zw, err := zstd.NewWriter(&buf, zstd.WithEncoderLevel(zstd.EncoderLevel(level)))
	if err != nil {
		return nil, fmt.Errorf("new zstd writer: %w", err)
	}
	if _, err := zw.Write(data); err != nil {
		zw.Close()
		return nil, fmt.Errorf("compress zstd write: %w", err)
	}
	if err := zw.Close(); err != nil {
		return nil, fmt.Errorf("close zstd writer: %w", err)
	}
	return buf.Bytes(), nil
}

func Decompress(data []byte) ([]byte, error) {
	algo := getCompressionAlgorithm()
	switch algo {
	case compressionAlgorithmZstd, compressionAlgorithmDeflate:
		return decompressZstd(data)
	default:
		return decompressGzip(data)
	}
}

func decompressGzip(data []byte) ([]byte, error) {
	gr, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("new gzip reader: %w", err)
	}
	defer gr.Close()
	out, err := io.ReadAll(gr)
	if err != nil {
		return nil, fmt.Errorf("decompress: %w", err)
	}
	return out, nil
}

func decompressZstd(data []byte) ([]byte, error) {
	zr, err := zstd.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("new zstd reader: %w", err)
	}
	defer zr.Close()
	out, err := io.ReadAll(zr)
	if err != nil {
		return nil, fmt.Errorf("decompress zstd: %w", err)
	}
	return out, nil
}
