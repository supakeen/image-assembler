package ipc

import (
	"os"
	"testing"

	"golang.org/x/sys/unix"
)

func TestFDSetLen(t *testing.T) {
	r, w, _ := os.Pipe()
	defer r.Close()
	defer w.Close()

	fs := NewFDSet([]int{int(r.Fd()), int(w.Fd())})
	if fs.Len() != 2 {
		t.Errorf("expected Len 2, got %d", fs.Len())
	}
}

func TestFDSetGet(t *testing.T) {
	r, w, _ := os.Pipe()
	rfd := int(r.Fd())
	wfd := int(w.Fd())
	defer r.Close()
	defer w.Close()

	fs := NewFDSet([]int{rfd, wfd})

	got, err := fs.Get(0)
	if err != nil || got != rfd {
		t.Errorf("Get(0): got %d, err %v", got, err)
	}

	_, err = fs.Get(5)
	if err == nil {
		t.Error("expected error for out-of-range Get")
	}
}

func TestFDSetSteal(t *testing.T) {
	fds, err := unix.Socketpair(unix.AF_UNIX, unix.SOCK_STREAM, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer unix.Close(fds[0])
	defer unix.Close(fds[1])

	fs := NewFDSet([]int{fds[0], fds[1]})

	stolen, err := fs.Steal(0)
	if err != nil {
		t.Fatal(err)
	}
	if stolen != fds[0] {
		t.Errorf("expected fd %d, got %d", fds[0], stolen)
	}

	if fs.Len() != 1 {
		t.Errorf("expected Len 1 after steal, got %d", fs.Len())
	}

	_, err = fs.Steal(0)
	if err == nil {
		t.Error("expected error for already-stolen fd")
	}
}

func TestFDSetClose(t *testing.T) {
	r, w, _ := os.Pipe()
	rfd, _ := unix.Dup(int(r.Fd()))
	wfd, _ := unix.Dup(int(w.Fd()))
	r.Close()
	w.Close()

	fs := NewFDSet([]int{rfd, wfd})
	fs.Close()

	if fs.Len() != 0 {
		t.Errorf("expected Len 0 after Close, got %d", fs.Len())
	}

	err := unix.Fstat(rfd, &unix.Stat_t{})
	if err == nil {
		t.Error("expected closed fd to fail Fstat")
	}
}
