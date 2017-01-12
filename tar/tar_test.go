package tar

import (
	"archive/tar"
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"strings"
	. "testing"

	"golang.org/x/sys/unix"

	. "gopkg.in/check.v1"
)

type tarSuite struct{}

var _ = Suite(&tarSuite{})

func TestTar(t *T) {
	TestingT(t)
}

func (ts *tarSuite) TestUnarchive(c *C) {
	tarball, sum, err := Archive(".", "/")
	c.Assert(err, IsNil)
	c.Assert(sum, Not(Equals), "")
	c.Assert(tarball, Not(Equals), "")
	defer os.Remove(tarball)

	tmp, err := ioutil.TempDir("", "unarchive-test")
	c.Assert(err, IsNil)
	defer os.RemoveAll(tmp)

	c.Assert(Unarchive(tmp, tarball), IsNil)

	_, err = os.Stat(filepath.Join(tmp, "tar.go"))
	c.Assert(err, IsNil)
	_, err = os.Stat(filepath.Join(tmp, "tar_test.go"))
	c.Assert(err, IsNil)
}

func (ts *tarSuite) TestArchive(c *C) {
	tarball, sum, err := Archive(context.Background(), ".", "/", []string{})
	c.Assert(err, IsNil)
	c.Assert(sum, Not(Equals), "")
	c.Assert(tarball, Not(Equals), "")
	defer os.Remove(tarball)

	f, err := os.Open(tarball)
	c.Assert(err, IsNil)
	defer f.Close()

	r := tar.NewReader(f)

	for {
		header, err := r.Next()
		if err != nil {
			break
		}

		c.Assert(strings.HasPrefix(header.Name, "/"), Equals, true)
	}
}

func (ts *tarSuite) TestArchiveSpecialFile(c *C) {
	dir, err := ioutil.TempDir("", "tar-test")
	c.Assert(err, IsNil)
	defer os.RemoveAll(dir)

	tmp, err := os.Create(filepath.Join(dir, "test"))
	c.Assert(err, IsNil)
	tmp.Close()
	c.Assert(os.Symlink(tmp.Name(), filepath.Join(dir, "testsym")), IsNil)
	c.Assert(unix.Mkfifo(filepath.Join(dir, "test.fifo"), 0666), IsNil)

	tarball, _, err := Archive(context.Background(), dir, "/", []string{})
	c.Assert(err, IsNil)
	c.Assert(tarball, Not(Equals), "")
	defer os.Remove(tarball)

	f, err := os.Open(tarball)
	c.Assert(err, IsNil)
	defer f.Close()

	r := tar.NewReader(f)

	count := 0
	names := []string{}

	for {
		header, err := r.Next()
		if err != nil {
			break
		}

		count++

		if strings.HasSuffix(header.Name, "testsym") {
			c.Assert(strings.HasSuffix(header.Linkname, "test"), Equals, true, Commentf("%v", header.Linkname))
		}

		names = append(names, header.Name)
	}

	c.Assert(count, Equals, 3, Commentf("%v", names))
}

func (ts *tarSuite) TestArchiveGlob(c *C) {
	prefixes := []string{"foo", "bar"}

	dir, err := ioutil.TempDir("", "tar-test")
	c.Assert(err, IsNil)
	defer os.RemoveAll(dir)

	for i := 0; i < 20; i++ {
		for _, prefix := range prefixes {
			c.Assert(ioutil.WriteFile(fmt.Sprintf("%s/%s%d", dir, prefix, i), nil, 0666), IsNil)
		}
	}

	for _, prefix := range prefixes {

		tarball, _, err := Archive(context.Background(), fmt.Sprintf("%s/%s*", dir, prefix), "/", []string{})
		c.Assert(err, IsNil)
		defer os.Remove(tarball)

		f, err := os.Open(tarball)
		c.Assert(err, IsNil)
		defer f.Close()

		r := tar.NewReader(f)

		for {
			header, err := r.Next()
			if err != nil {
				break
			}

			if header.Name == "/" {
				continue
			}

			c.Assert(strings.HasPrefix(path.Base(header.Name), prefix), Equals, true, Commentf("%s", header.Name))
		}
	}
}

func (ts *tarSuite) TestArchiveIgnore(c *C) {
	prefixes := []string{"foo", "bar"}

	dir, err := ioutil.TempDir("", "tar-test")
	c.Assert(err, IsNil)
	defer os.RemoveAll(dir)

	for i := 0; i < 20; i++ {
		for _, prefix := range prefixes {
			c.Assert(ioutil.WriteFile(fmt.Sprintf("%s/%s%d", dir, prefix, i), nil, 0666), IsNil)
		}
	}

	for _, prefix := range prefixes {
		tarball, _, err := Archive(context.Background(), dir, "/", []string{fmt.Sprintf("%s*", prefix)})
		c.Assert(err, IsNil)
		defer os.Remove(tarball)

		f, err := os.Open(tarball)
		c.Assert(err, IsNil)
		defer f.Close()

		r := tar.NewReader(f)

		for {
			header, err := r.Next()
			if err != nil {
				break
			}

			if header.Name == "/" {
				continue
			}

			c.Assert(strings.HasPrefix(path.Base(header.Name), prefix), Equals, false, Commentf("%s: %s", prefix, header.Name))
		}
	}
}
