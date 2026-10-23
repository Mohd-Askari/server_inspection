//go:build linux

package collector

import "syscall"

// getFilesystemStats returns total, used, free bytes for a mount path on Linux.
func getFilesystemStats(path string) (total, free, avail uint64, err error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return 0, 0, 0, err
	}
	bsize := uint64(stat.Bsize)
	total = stat.Blocks * bsize
	free = stat.Bfree * bsize
	avail = stat.Bavail * bsize
	return total, free, avail, nil
}
