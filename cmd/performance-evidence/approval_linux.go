package main

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/stanimirivanov/argus/internal/performance/adapters/cli/evidencecli"
)

// Open each component without following links. Ownership/modes supplement trusted
// issuance; they do not authenticate policy that a subject supplied to the host.
func protectedApproval(path string) ([]byte, error) {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path || path == "/" {
		return nil, evidencecli.ErrDenied
	}
	fd, err := syscall.Open("/", syscall.O_RDONLY|syscall.O_DIRECTORY|syscall.O_CLOEXEC, 0)
	if err != nil {
		return nil, evidencecli.ErrDenied
	}
	current := os.NewFile(uintptr(fd), "approval root")
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
	for i, part := range parts {
		flags := syscall.O_RDONLY | syscall.O_NOFOLLOW | syscall.O_NONBLOCK | syscall.O_CLOEXEC
		leaf := i == len(parts)-1
		if !leaf {
			flags |= syscall.O_DIRECTORY
		}
		next, err := syscall.Openat(int(current.Fd()), part, flags, 0)
		_ = current.Close() //nolint:errcheck // The read/open failure has precedence.
		if err != nil {
			return nil, evidencecli.ErrDenied
		}
		current = os.NewFile(uintptr(next), "approval resource")
		if !approvalOwner(current, leaf) {
			_ = current.Close() //nolint:errcheck // No authority survives refusal.
			return nil, evidencecli.ErrDenied
		}
	}
	defer func() { _ = current.Close() }() //nolint:errcheck // The bounded read has precedence.
	raw, err := io.ReadAll(io.LimitReader(current, (64<<10)+1))
	if err != nil || len(raw) > 64<<10 {
		return nil, evidencecli.ErrDenied
	}

	return raw, nil
}

func approvalOwner(file *os.File, leaf bool) bool {
	info, err := file.Stat()
	if err != nil {
		return false
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || (stat.Uid != 0 && uint64(stat.Uid) != uint64(os.Getuid())) {
		return false
	}
	if leaf {
		return info.Mode().IsRegular() && stat.Nlink == 1 && info.Mode().Perm()&0o077 == 0 && info.Size() <= 64<<10
	}

	return info.IsDir() && (info.Mode().Perm()&0o022 == 0 || info.Mode()&os.ModeSticky != 0)
}
