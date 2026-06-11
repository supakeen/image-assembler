package ipc

import (
	"fmt"

	"golang.org/x/sys/unix"
)

// FDSet holds file descriptors received via SCM_RIGHTS ancillary data. It
// provides ordinal access (Get) and ownership transfer (Steal) so that
// protocol handlers can claim specific fds by position while ensuring
// unclaimed fds are closed on cleanup.
type FDSet struct {
	fds []int
}

func NewFDSet(fds []int) *FDSet {
	copied := make([]int, len(fds))
	copy(copied, fds)
	return &FDSet{fds: copied}
}

func (f *FDSet) Len() int {
	n := 0
	for _, fd := range f.fds {
		if fd >= 0 {
			n++
		}
	}
	return n
}

func (f *FDSet) Get(i int) (int, error) {
	if i < 0 || i >= len(f.fds) {
		return -1, fmt.Errorf("fdset index %d out of range [0, %d)", i, len(f.fds))
	}
	fd := f.fds[i]
	if fd < 0 {
		return -1, fmt.Errorf("fdset index %d already stolen", i)
	}
	return fd, nil
}

// Steal returns the fd at position i and marks it as taken (-1) so Close
// won't close it. The caller assumes ownership of the returned fd.
func (f *FDSet) Steal(i int) (int, error) {
	fd, err := f.Get(i)
	if err != nil {
		return -1, err
	}
	f.fds[i] = -1
	return fd, nil
}

func (f *FDSet) Close() {
	for i, fd := range f.fds {
		if fd >= 0 {
			unix.Close(fd)
			f.fds[i] = -1
		}
	}
}
