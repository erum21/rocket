// Copyright 2014 CoreOS, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package tar

import (
	"archive/tar"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
)

type testTarEntry struct {
	header   *tar.Header
	contents string
}

func newTestTar(entries []*testTarEntry) (string, error) {
	t, err := ioutil.TempFile("", "test-tar")
	if err != nil {
		return "", err
	}
	defer t.Close()
	tw := tar.NewWriter(t)
	for _, entry := range entries {
		if err := tw.WriteHeader(entry.header); err != nil {
			return "", err
		}
		if _, err := io.WriteString(tw, entry.contents); err != nil {
			return "", err
		}
	}
	if err := tw.Close(); err != nil {
		return "", err
	}
	return t.Name(), nil
}

func TestExtractTarInsecureSymlink(t *testing.T) {
	entries := []*testTarEntry{
		{
			contents: "hello",
			header: &tar.Header{
				Name: "hello.txt",
				Size: 5,
			},
		},
		{
			header: &tar.Header{
				Name:     "link.txt",
				Linkname: "hello.txt",
				Typeflag: tar.TypeSymlink,
			},
		},
	}
	insecureSymlinkEntries := append(entries, &testTarEntry{
		header: &tar.Header{
			Name:     "../etc/secret.conf",
			Linkname: "secret.conf",
			Typeflag: tar.TypeSymlink,
		},
	})
	insecureHardlinkEntries := append(entries, &testTarEntry{
		header: &tar.Header{
			Name:     "../etc/secret.conf",
			Linkname: "secret.conf",
			Typeflag: tar.TypeLink,
		},
	})
	for _, entries := range [][]*testTarEntry{insecureSymlinkEntries, insecureHardlinkEntries} {
		testTarPath, err := newTestTar(entries)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		defer os.Remove(testTarPath)
		containerTar, err := os.Open(testTarPath)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		tr := tar.NewReader(containerTar)
		tmpdir, err := ioutil.TempDir("", "rocket-temp-dir")
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		os.RemoveAll(tmpdir)
		err = os.MkdirAll(tmpdir, 0755)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		defer os.RemoveAll(tmpdir)
		err = ExtractTar(tr, tmpdir, nil)
		if _, ok := err.(insecureLinkError); !ok {
			t.Errorf("expected insecureSymlinkError error")
		}
	}
}

func TestExtractTarFolders(t *testing.T) {
	entries := []*testTarEntry{
		{
			contents: "foo",
			header: &tar.Header{
				Name: "deep/folder/foo.txt",
				Size: 3,
			},
		},
		{
			header: &tar.Header{
				Name:     "deep/folder/",
				Typeflag: tar.TypeDir,
				Mode:     int64(0747),
			},
		},
		{
			contents: "bar",
			header: &tar.Header{
				Name: "deep/folder/bar.txt",
				Size: 3,
			},
		},
		{
			header: &tar.Header{
				Name:     "deep/folder2/symlink.txt",
				Typeflag: tar.TypeSymlink,
				Linkname: "deep/folder/foo.txt",
			},
		},
		{
			header: &tar.Header{
				Name:     "deep/folder2/",
				Typeflag: tar.TypeDir,
				Mode:     int64(0747),
			},
		},
		{
			contents: "bar",
			header: &tar.Header{
				Name: "deep/folder2/bar.txt",
				Size: 3,
			},
		},
		{
			header: &tar.Header{
				Name:     "deep/deep/folder",
				Typeflag: tar.TypeDir,
				Mode:     int64(0755),
			},
		},
		{
			header: &tar.Header{
				Name:     "deep/deep/",
				Typeflag: tar.TypeDir,
				Mode:     int64(0747),
			},
		},
	}

	testTarPath, err := newTestTar(entries)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	defer os.Remove(testTarPath)
	containerTar, err := os.Open(testTarPath)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	tr := tar.NewReader(containerTar)
	tmpdir, err := ioutil.TempDir("", "rocket-temp-dir")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	os.RemoveAll(tmpdir)
	err = os.MkdirAll(tmpdir, 0755)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	defer os.RemoveAll(tmpdir)
	err = ExtractTar(tr, tmpdir, nil)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	matches, err := filepath.Glob(filepath.Join(tmpdir, "deep/folder/*.txt"))
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if len(matches) != 2 {
		t.Errorf("unexpected number of files found: %d, wanted 2", len(matches))
	}
	matches, err = filepath.Glob(filepath.Join(tmpdir, "deep/folder2/*.txt"))
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if len(matches) != 2 {
		t.Errorf("unexpected number of files found: %d, wanted 2", len(matches))
	}

	dirInfo, err := os.Lstat(filepath.Join(tmpdir, "deep/folder"))
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	} else if dirInfo.Mode().Perm() != os.FileMode(0747) {
		t.Errorf("unexpected dir mode: %s", dirInfo.Mode())
	}
	dirInfo, err = os.Lstat(filepath.Join(tmpdir, "deep/deep"))
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	} else if dirInfo.Mode().Perm() != os.FileMode(0747) {
		t.Errorf("unexpected dir mode: %s", dirInfo.Mode())
	}
}

func TestExtractTarFileToBuf(t *testing.T) {
	entries := []*testTarEntry{
		{
			header: &tar.Header{
				Name:     "folder/",
				Typeflag: tar.TypeDir,
				Mode:     int64(0747),
			},
		},
		{
			contents: "foo",
			header: &tar.Header{
				Name: "folder/foo.txt",
				Size: 3,
			},
		},
		{
			contents: "bar",
			header: &tar.Header{
				Name: "folder/bar.txt",
				Size: 3,
			},
		},
		{
			header: &tar.Header{
				Name:     "folder/symlink.txt",
				Typeflag: tar.TypeSymlink,
				Linkname: "folder/foo.txt",
			},
		},
	}
	testTarPath, err := newTestTar(entries)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer os.Remove(testTarPath)
	containerTar, err := os.Open(testTarPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	tr := tar.NewReader(containerTar)
	buf, err := ExtractFileFromTar(tr, "folder/foo.txt")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if string(buf) != "foo" {
		t.Errorf("unexpected contents, wanted: %s, got: %s", "foo", buf)
	}
	containerTar.Close()

	containerTar, err = os.Open(testTarPath)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	tr = tar.NewReader(containerTar)
	buf, err = ExtractFileFromTar(tr, "folder/symlink.txt")
	if err == nil {
		t.Errorf("expected error")
	}
	containerTar.Close()
}

func TestExtractTarPWL(t *testing.T) {
	entries := []*testTarEntry{
		{
			header: &tar.Header{
				Name:     "folder/",
				Typeflag: tar.TypeDir,
				Mode:     int64(0747),
			},
		},
		{
			contents: "foo",
			header: &tar.Header{
				Name: "folder/foo.txt",
				Size: 3,
			},
		},
		{
			contents: "bar",
			header: &tar.Header{
				Name: "folder/bar.txt",
				Size: 3,
			},
		},
		{
			header: &tar.Header{
				Name:     "folder/symlink.txt",
				Typeflag: tar.TypeSymlink,
				Linkname: "folder/foo.txt",
			},
		},
	}
	testTarPath, err := newTestTar(entries)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	defer os.Remove(testTarPath)
	containerTar, err := os.Open(testTarPath)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	tr := tar.NewReader(containerTar)
	tmpdir, err := ioutil.TempDir("", "rocket-temp-dir")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	defer os.RemoveAll(tmpdir)

	pwl := make(PathWhitelistMap)
	pwl["folder/foo.txt"] = struct{}{}
	err = ExtractTar(tr, tmpdir, pwl)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	matches, err := filepath.Glob(filepath.Join(tmpdir, "folder/*.txt"))
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if len(matches) != 1 {
		t.Errorf("unexpected number of files found: %d, wanted 1", len(matches))
	}
}
