package server

import (
	"fmt"

	"golang.org/x/sys/unix"
)

// spoolDiskHeadroomBytes is reserved beyond any payload being spooled to
// disk so an upload, pull, or transfer can never consume the filesystem to
// its final byte and prevent logs, rollback data, or the spool cleanup
// itself from being written.
const spoolDiskHeadroomBytes = int64(10 << 30)

// availableDiskBytes reports the free bytes on the filesystem holding path.
func availableDiskBytes(path string) (int64, error) {
	var stat unix.Statfs_t
	if err := unix.Statfs(path, &stat); err != nil {
		return 0, err
	}
	free := uint64(stat.Bavail) * uint64(stat.Bsize)
	const maxInt64 = uint64(^uint64(0) >> 1)
	if free > maxInt64 {
		return int64(maxInt64), nil
	}
	return int64(free), nil
}

// ensureDiskSpaceForSpool verifies the filesystem holding dir has room for
// needed bytes plus spoolDiskHeadroomBytes before any payload byte is
// written. s.diskFreeFn is nil in production; tests inject a stub.
func (s *Server) ensureDiskSpaceForSpool(dir string, needed int64) error {
	if needed <= 0 {
		return nil
	}
	var free int64
	var err error
	if s.diskFreeFn != nil {
		free, err = s.diskFreeFn(dir)
	} else {
		free, err = availableDiskBytes(dir)
	}
	if err != nil {
		return fmt.Errorf("check free disk space: %w", err)
	}
	required := needed + spoolDiskHeadroomBytes
	if free < required {
		return fmt.Errorf("insufficient free disk space: need %d bytes (%d payload + %d headroom), have %d", required, needed, spoolDiskHeadroomBytes, free)
	}
	return nil
}
