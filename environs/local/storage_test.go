package local_test

import (
	"bytes"
	"io/ioutil"
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/local"
	"launchpad.net/juju-core/environs/storage"
	"net/http"
)

type storageSuite struct{}

var _ = Suite(&storageSuite{})

// TestPersistence tests the adding, reading, listing and removing
// of files from the local storage.
func (s *storageSuite) TestPersistence(c *C) {
	portNo, listener, _ := nextTestSet(c)
	defer listener.Close()

	store := local.NewStorage("127.0.0.1", portNo)
	names := []string{
		"aa",
		"zzz/aa",
		"zzz/bb",
	}
	for _, name := range names {
		checkFileDoesNotExist(c, store, name)
		checkPutFile(c, store, name, []byte(name))
	}
	checkList(c, store, "", names)
	checkList(c, store, "a", []string{"aa"})
	checkList(c, store, "zzz/", []string{"zzz/aa", "zzz/bb"})

	storage2 := local.NewStorage("127.0.0.1", portNo)
	for _, name := range names {
		checkFileHasContents(c, storage2, name, []byte(name))
	}

	// remove the first file and check that the others remain.
	err := storage2.Remove(names[0])
	c.Check(err, IsNil)

	// check that it's ok to remove a file twice.
	err = storage2.Remove(names[0])
	c.Check(err, IsNil)

	// ... and check it's been removed in the other environment
	checkFileDoesNotExist(c, store, names[0])

	// ... and that the rest of the files are still around
	checkList(c, storage2, "", names[1:])

	for _, name := range names[1:] {
		err := storage2.Remove(name)
		c.Assert(err, IsNil)
	}

	// check they've all gone
	checkList(c, storage2, "", nil)
}

func checkList(c *C, store storage.Reader, prefix string, names []string) {
	lnames, err := store.List(prefix)
	c.Assert(err, IsNil)
	c.Assert(lnames, DeepEquals, names)
}

func checkPutFile(c *C, store storage.Writer, name string, contents []byte) {
	c.Logf("check putting file %s ...", name)
	err := store.Put(name, bytes.NewBuffer(contents), int64(len(contents)))
	c.Assert(err, IsNil)
}

func checkFileDoesNotExist(c *C, store storage.Reader, name string) {
	var notFoundError *environs.NotFoundError
	r, err := store.Get(name)
	c.Assert(r, IsNil)
	c.Assert(err, FitsTypeOf, notFoundError)
}

func checkFileHasContents(c *C, store storage.Reader, name string, contents []byte) {
	r, err := store.Get(name)
	c.Assert(err, IsNil)
	c.Check(r, NotNil)
	defer r.Close()

	data, err := ioutil.ReadAll(r)
	c.Check(err, IsNil)
	c.Check(data, DeepEquals, contents)

	url, err := store.URL(name)
	c.Assert(err, IsNil)

	resp, err := http.Get(url)
	c.Assert(err, IsNil)
	data, err = ioutil.ReadAll(resp.Body)
	c.Assert(err, IsNil)
	defer resp.Body.Close()
	c.Assert(resp.StatusCode, Equals, 200, Commentf("error response: %s", data))
	c.Check(data, DeepEquals, contents)
}
