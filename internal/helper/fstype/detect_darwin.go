package fstype

// syscall should not be included in the linux package given that runs in
// production.
// See: https://docs.google.com/document/d/1QXzI9I1pOfZPujQzxhyRy6EeHYTQitKKjHfpq0zpxZs
import "syscall"

func statFileSystemType(path string) (int64, error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return 0, err
	}

	return int64(stat.Type), nil
}
