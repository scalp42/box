package layer

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/erikh/box/tar"

	"golang.org/x/sys/unix"
)

// ErrNotMounted is returned when the filesystem was not already mounted during
// an operation which required it to be.
var ErrNotMounted = errors.New("Filesystem not mounted")

// Filesystem encapsulates a fully mounted filesystem. It is manipulated by
// adding layers and unmounting (and remounting) the product.
type Filesystem struct {
	Layers     []*Layer
	Mountpoint string

	workDir string
	mounted bool
}

// Mount creates any missing layers and mounts the filesystem.
func (f *Filesystem) Mount(work string) error {
	for _, layer := range f.Layers {
		if !layer.Exists() {
			if err := layer.Create(); err != nil {
				return err
			}
		}
	}

	var lower []*Layer
	var upper *Layer

	if len(f.Layers) == 1 {
		return fmt.Errorf("Minimum 2 layers for mountpoint %q: got 1", f.Mountpoint)
	}

	lower = f.Layers[:len(f.Layers)-1]
	upper = f.Layers[len(f.Layers)-1]

	if work == "" {
		return fmt.Errorf("In mount of mountpoint %q: workdir cannot be empty", f.Mountpoint)
	}

	f.workDir = work
	if err := os.Mkdir(work, 0700); err != nil {
		return err
	}

	lowerStrs := []string{}
	for _, layer := range lower {
		lowerStrs = append(lowerStrs, layer.Path())
	}

	data := fmt.Sprintf("lowerdir=%s,upperdir=%s,workdir=%s", strings.Join(lowerStrs, ":"), upper.Path(), work)

	if err := unix.Mount("overlay", f.Mountpoint, "overlay", 0, data); err != nil {
		return err
	}

	f.mounted = true

	return nil
}

// Unmount unmounts the filesystem. Does not touch anything else.
func (f *Filesystem) Unmount() error {
	if !f.mounted {
		return ErrNotMounted
	}

	if err := unix.Unmount(f.Mountpoint, 0); err != nil {
		return err
	}

	if err := os.RemoveAll(f.workDir); err != nil {
		return fmt.Errorf("Could not remove work dir: %v", err)
	}

	f.mounted = false
	return nil
}

// Mounted returns whether or not the filesystem is mounted. This is based on
// internal, not kernel data.
func (f *Filesystem) Mounted() bool {
	return f.mounted
}

// Flatten flattens an overlayfs filesystem, by tarring the top-most mount and
// returning the tarfile name. Note that you must STILL call Unmount to remove
// the mount.
func (f *Filesystem) Flatten() (string, string, error) {
	if !f.mounted {
		return "", "", ErrNotMounted
	}

	return tar.Archive(context.Background(), f.Mountpoint, "/", []string{})
}
