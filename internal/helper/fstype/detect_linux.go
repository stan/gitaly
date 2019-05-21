package fstype

import "golang.org/x/sys/unix"

func statFileSystemType(path string) (int64, error) {
	var stat unix.Statfs_t
	if err := unix.Statfs(path, &stat); err != nil {
		return 0, err
	}

	return stat.Type, nil
}
