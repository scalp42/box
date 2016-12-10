package tar

import (
	"archive/tar"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/erikh/box/copy"
)

var errIgnore = errors.New("ignore this file")

func archiveSingle(rel, target string, tw *tar.Writer) error {
	fi, err := os.Lstat(rel)
	if err != nil {
		return err
	}

	if strings.HasSuffix(target, "/") {
		target = filepath.Join(target, filepath.Base(rel))
	}

	header, err := tar.FileInfoHeader(fi, target)
	if err != nil {
		return err
	}

	header.Name = target

	if err := tw.WriteHeader(header); err != nil {
		return err
	}

	p, err := os.Open(rel)
	if err != nil {
		return err
	}

	defer p.Close()

	return copy.WithProgress(tw, p, fmt.Sprintf("Writing %s", rel))
}

func checkIgnore(rel, path string, ignoreList []string) error {
	for _, ignore := range ignoreList {
		entries, err := filepath.Glob(filepath.Join(rel, ignore))
		if err != nil {
			return err
		}

		for _, entry := range entries {
			if ok, err := filepath.Rel(entry, path); err == nil && !strings.HasPrefix(ok, "..") {
				return errIgnore
			}
		}
	}

	return nil
}

func archiveWalk(rel, target string, ignoreList []string, tw *tar.Writer) filepath.WalkFunc {
	return func(path string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		path, err = filepath.Abs(path)
		if err != nil {
			return err
		}

		relpath, err := filepath.Rel(rel, path)
		if err != nil {
			return err
		}

		if err := checkIgnore(rel, path, ignoreList); err == errIgnore {
			return nil
		} else if err != nil {
			return err
		}

		header, err := tar.FileInfoHeader(fi, filepath.Join(target, relpath))
		if err != nil {
			return err
		}

		if !(header.Typeflag == tar.TypeReg || header.Typeflag == tar.TypeSymlink) {
			return nil
		}

		realpath, err := filepath.EvalSymlinks(path)
		if err != nil {
			return fmt.Errorf("evaluating (probably dangling) symlink %q: %v", path, err)
		}

		realrel, err := filepath.Rel(rel, realpath)
		if err != nil {
			return err
		}

		if strings.HasPrefix(realrel, "..") {
			return fmt.Errorf("path %q (symlink: %q) falls below the box working directory", realpath, path)
		}

		header.Name = filepath.Join(target, realrel)

		return writeTarFile(tw, header, realpath)
	}
}

func writeTarFile(tw *tar.Writer, header *tar.Header, realpath string) error {
	if err := tw.WriteHeader(header); err != nil {
		return err
	}

	if header.Typeflag == tar.TypeReg {
		p, err := os.Open(realpath)
		if err != nil {
			return err
		}

		err = copy.WithProgress(tw, p, fmt.Sprintf("Writing %s", realpath))
		if err != nil && err != io.EOF {
			p.Close()
			return err
		}

		p.Close()
	}

	return nil
}

// FIXME move to utility lib
func checkContext(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	return nil
}

type lstatInfo struct {
	filename string
	fi       os.FileInfo
}

func processEntries(entries []string) ([]lstatInfo, error) {
	lstatEntries := []lstatInfo{}

	for _, entry := range entries {
		evaledentry, err := filepath.EvalSymlinks(entry)
		if err != nil {
			return lstatEntries, err
		}

		evaledentry, err = filepath.Abs(evaledentry)
		if err != nil {
			return lstatEntries, err
		}

		fi, err := os.Lstat(evaledentry)
		if err != nil {
			return lstatEntries, err
		}

		lstatEntries = append(lstatEntries, lstatInfo{evaledentry, fi})
	}

	return lstatEntries, nil
}

// Unarchive takes a destination directory and tar filename; it returns an
// error if the unarchive failed.  It unpacks a tar archive into the
// destination directory.
func Unarchive(path, tarFile string) error {
	tf, err := os.Open(tarFile)
	if err != nil {
		return err
	}
	defer tf.Close()

	tr := tar.NewReader(tf)
	path = filepath.Clean(path)

	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		} else if err != nil {
			return err
		}

		if strings.HasPrefix(header.Name, "/") {
			header.Name = header.Name[1:]
		}

		rel, err := filepath.Rel(path, filepath.Clean(filepath.Join(path, header.Name)))
		if err != nil {
			return err
		}

		rel = filepath.Join(path, rel)

		if path == rel {
			continue
		}

		if strings.HasPrefix(rel, "..") {
			return fmt.Errorf("Path %q lies underneath temporary directory %q", header.Name, path)
		}

		if header.FileInfo().IsDir() {
			if err := os.MkdirAll(rel, header.FileInfo().Mode()); err != nil {
				return err
			}
		} else {
			f, err := os.OpenFile(rel, os.O_CREATE|os.O_WRONLY, header.FileInfo().Mode())
			if err != nil {
				f.Close()
				return err
			}

			if _, err := io.Copy(f, tr); err != nil {
				f.Close()
				return err
			}

			f.Close()
		}

		if err := os.Chown(rel, header.Uid, header.Gid); err != nil {
			return err
		}
	}

	return nil
}

// Archive takes a source and target directory and returns a filename and/or
// error. The source will be archived relative to the target. The file will
// live in the user's os.TempDir().
func Archive(ctx context.Context, rel, target string, ignoreList []string) (string, string, error) {
	if err := checkContext(ctx); err != nil {
		return "", "", err
	}

	entries, err := filepath.Glob(rel)
	if err != nil {
		return "", "", err
	}

	lstatEntries, err := processEntries(entries)
	if err != nil {
		return "", "", err
	}

	f, err := ioutil.TempFile("", "box-copy.")
	if err != nil {
		return "", "", err
	}

	hash := sha256.New()
	r, w := io.Pipe()
	tw := tar.NewWriter(w)

	tee := io.TeeReader(r, hash)
	go io.Copy(f, tee)

	for _, li := range lstatEntries {
		if err := checkContext(ctx); err != nil {
			os.Remove(f.Name())
			return "", "", err
		}

		if li.fi.IsDir() {
			header, err := tar.FileInfoHeader(li.fi, li.filename)
			if err != nil {
				return "", "", err
			}

			if target == "." {
				target = li.filename
			}

			header.Linkname = target
			header.Name = header.Linkname

			if err := tw.WriteHeader(header); err != nil {
				return "", "", err
			}
			if err := filepath.Walk(li.filename, archiveWalk(li.filename, target, ignoreList, tw)); err != nil {
				return "", "", err
			}
		} else {
			err := checkIgnore(li.filename, li.filename, ignoreList)
			if err == nil {
				if err := archiveSingle(li.filename, target, tw); err != nil {
					return "", "", err
				}
			} else if err != errIgnore {
				return "", "", err
			}
		}
	}

	tw.Close()
	f.Close()

	return f.Name(), hex.EncodeToString(hash.Sum(nil)), nil
}

// SumWithCopy simultaneously sums and copies a stream.
func SumWithCopy(writer io.WriteCloser, reader io.Reader, fileType string) (string, error) {
	hashReader, hashWriter := io.Pipe()
	tarReader := io.TeeReader(reader, hashWriter)

	sumChan := make(chan string, 1)
	errChan := make(chan error, 1)

	go func() {
		sum, err := SumReader(hashReader)
		if err != nil {
			errChan <- err
		} else {
			sumChan <- sum
		}
	}()

	if err := copy.WithProgress(writer, tarReader, fileType); err != nil {
		writer.Close()
		return "", err
	}

	writer.Close()
	hashWriter.Close()

	var sum string

	select {
	case err := <-errChan:
		return "", err
	case sum = <-sumChan:
	}

	return sum, nil
}

// SumReader sums an io.Reader
func SumReader(reader io.Reader) (string, error) {
	hash := sha256.New()
	_, err := io.Copy(hash, reader)
	return hex.EncodeToString(hash.Sum(nil)), err
}
