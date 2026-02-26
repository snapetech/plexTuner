//go:build linux
// +build linux

package vodfs

import (
	"syscall"

	"github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"
)

type movieDirStream struct {
	root *Root
	i    int
}

var _ fs.DirStream = (*movieDirStream)(nil)

func (s *movieDirStream) HasNext() bool {
	return s != nil && s.root != nil && s.i < len(s.root.Movies)
}

func (s *movieDirStream) Next() (fuse.DirEntry, syscall.Errno) {
	if !s.HasNext() {
		return fuse.DirEntry{}, 0
	}
	m := &s.root.Movies[s.i]
	s.i++
	return fuse.DirEntry{
		Name: s.root.movieDirName(m),
		Ino:  s.root.ino("movie:" + m.ID),
		Mode: fuse.S_IFDIR | 0755,
	}, 0
}

func (s *movieDirStream) Close() {}

type seriesDirStream struct {
	root *Root
	i    int
}

var _ fs.DirStream = (*seriesDirStream)(nil)

func (s *seriesDirStream) HasNext() bool {
	return s != nil && s.root != nil && s.i < len(s.root.Series)
}

func (s *seriesDirStream) Next() (fuse.DirEntry, syscall.Errno) {
	if !s.HasNext() {
		return fuse.DirEntry{}, 0
	}
	ser := &s.root.Series[s.i]
	s.i++
	return fuse.DirEntry{
		Name: s.root.showDirName(ser),
		Ino:  s.root.ino("series:" + ser.ID),
		Mode: fuse.S_IFDIR | 0755,
	}, 0
}

func (s *seriesDirStream) Close() {}
