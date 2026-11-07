//go:build linux

package main

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"

	"golang.org/x/sys/unix"
)

// Moby's archive writer preserves security.capability, not arbitrary user/ACL
// xattrs. Inspect the actual selected mount so omitted metadata cannot turn into
// a falsely faithful export. OpenRoot confines parent resolution; llistxattr
// does not follow a final symlink. No source file is opened writable or executed.
func checkVolume(rootPath string) (int, error) {
	root, err := os.OpenRoot(rootPath)
	if err != nil {
		return 0, err
	}
	defer root.Close()
	count := 0
	err = fs.WalkDir(root.FS(), ".", func(name string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		count++
		if count > 100000 || len(name) > 4096 {
			return errors.New("volume exceeds portable entry/path budget")
		}
		directory, err := root.Open(path.Dir(name))
		if err != nil {
			return err
		}
		defer directory.Close()
		anchored := fmt.Sprintf("/proc/self/fd/%d/%s", directory.Fd(), path.Base(name))
		attributes, err := unix.Llistxattr(anchored, nil)
		if err != nil {
			return fmt.Errorf("inspect attributes on %q: %w", name, err)
		}
		if attributes != 0 {
			return fmt.Errorf("volume entry %q has extended attributes/ACLs not supported by portable format 1", name)
		}
		mode := entry.Type()
		if mode != 0 && !mode.IsDir() && mode&os.ModeSymlink == 0 {
			return fmt.Errorf("special object %q is not portable", name)
		}
		return nil
	})
	return count, err
}
