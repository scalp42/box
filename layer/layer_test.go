package layer

import (
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	. "testing"

	. "gopkg.in/check.v1"
)

type layerSuite struct{}

var _ = Suite(&layerSuite{})

func TestLayer(t *T) {
	TestingT(t)
}

func inTmpDir(c *C, fun func(c *C, dir string)) {
	wd, err := os.Getwd()
	c.Assert(err, IsNil)

	name, err := ioutil.TempDir("", "box-layer-test")
	c.Assert(err, IsNil)

	c.Assert(os.Chdir(name), IsNil)

	fun(c, name)

	defer os.Chdir(wd)
	defer os.RemoveAll(name)
}

func (s *layerSuite) TestNew(c *C) {
	table := []struct {
		pathargs   [2]string
		errCheck   Checker
		layerCheck Checker
		errStr     string
	}{
		{
			[2]string{"..", ""},
			NotNil,
			IsNil,
			"Cannot use ..",
		},
		{
			[2]string{"..", ".."},
			NotNil,
			IsNil,
			"Cannot use ..",
		},
		{
			[2]string{".", ".."},
			IsNil,
			NotNil,
			"",
		},
		{
			[2]string{".", ""},
			IsNil,
			NotNil,
			"",
		},
		{
			[2]string{"", ""},
			NotNil,
			IsNil,
			"",
		},
	}

	for i, check := range table {
		comment := Commentf("Index: %d", i)
		l, err := New(check.pathargs[0], check.pathargs[1])
		c.Assert(err, check.errCheck, comment)
		c.Assert(l, check.layerCheck, comment)
		if l != nil {
			c.Assert(l.dirname, Equals, check.pathargs[0])
			path, err := filepath.Abs(check.pathargs[1])
			c.Assert(err, IsNil)
			c.Assert(l.workingDir, Equals, path)
		}

		if check.errStr != "" {
			c.Assert(strings.Contains(err.Error(), check.errStr), Equals, true, comment)
		}
	}

	dir, err := ioutil.TempDir("", "box-layer-test")
	c.Assert(err, IsNil)
	defer os.RemoveAll(dir)

	l, err := New(path.Base(dir), os.TempDir())
	c.Assert(err, IsNil)
	c.Assert(l, NotNil)
}

func (s *layerSuite) TestCreateRemove(c *C) {
	inTmpDir(c, func(c *C, dir string) {
		l, err := New("quux", dir)
		c.Assert(err, IsNil)
		c.Assert(l.Create(), IsNil)
		fi, err := os.Stat(filepath.Join(dir, "quux"))
		c.Assert(err, IsNil)
		c.Assert(fi.IsDir(), Equals, true)
		c.Assert(l.Remove(), IsNil)
		_, err = os.Stat(filepath.Join(dir, "quux"))
		c.Assert(err, NotNil)
	})
}

func (s *layerSuite) TestFilesystemMount(c *C) {
	inTmpDir(c, func(c *C, dir string) {
		target := filepath.Join(dir, "quux")
		c.Assert(os.Mkdir(target, 0700), IsNil)

		fs := &Filesystem{Mountpoint: target}
		l, err := New("foo", dir)
		c.Assert(err, IsNil)

		l2, err := New("bar", dir)
		c.Assert(err, IsNil)

		defer func() {
			c.Assert(fs.Unmount(), IsNil)
			out, err := exec.Command("mount", "-t", "overlay").CombinedOutput()
			c.Assert(err, IsNil)
			c.Assert(strings.TrimSpace(string(out)), Equals, "")
		}()

		fs.Layers = []*Layer{l, l2}
		c.Assert(fs.Mount(filepath.Join(dir, "work")), IsNil)
		c.Assert(fs.Mounted(), Equals, true)
		out, err := exec.Command("mount", "-t", "overlay").CombinedOutput()
		c.Assert(err, IsNil)
		c.Assert(strings.TrimSpace(string(out)), Not(Equals), "")
	})
}
