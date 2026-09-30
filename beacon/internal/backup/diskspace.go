package backup

import "fmt"

// downloadDiskReserveBytes keeps the daemon from consuming the last bytes
// of the filesystem when staging an S3 backup download; the reserve leaves
// room for logs, metadata, and the spool cleanup itself.
const downloadDiskReserveBytes = int64(64 << 20)

// ensureStagingDiskSpace aborts an S3 backup download before the first
// payload byte is written when the staging filesystem cannot hold the
// payload plus the reserve. s.diskFreeFn is nil in production; tests
// inject a stub.
func (s *S3Backup) ensureStagingDiskSpace(dir string, needed int64) error {
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
	required := needed + downloadDiskReserveBytes
	if free < required {
		return fmt.Errorf("insufficient free disk space: need %d bytes (%d payload + %d reserve), have %d", required, needed, downloadDiskReserveBytes, free)
	}
	return nil
}
