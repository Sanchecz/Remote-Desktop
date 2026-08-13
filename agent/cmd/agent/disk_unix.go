//go:build !windows

package main

import "syscall"

func diskSpace() (total, free int64) {
	var stats syscall.Statfs_t
	if syscall.Statfs("/", &stats) != nil {
		return 0, 0
	}
	return int64(stats.Blocks) * int64(stats.Bsize), int64(stats.Bavail) * int64(stats.Bsize)
}
