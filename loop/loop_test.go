package loop

import (
	"context"
	"os"
	"testing"

	"golang.org/x/sys/unix"
)

func skipUnlessRoot(t *testing.T) {
	t.Helper()
	if os.Getuid() != 0 {
		t.Skip("requires root")
	}
}

func TestNewControlAndGetFree(t *testing.T) {
	skipUnlessRoot(t)

	ctl, err := NewControl()
	if err != nil {
		t.Fatal(err)
	}
	defer ctl.Close()

	minor, err := ctl.GetFree()
	if err != nil {
		t.Fatal(err)
	}
	if minor < 0 {
		t.Errorf("expected non-negative minor, got %d", minor)
	}
}

func TestLoopForFD(t *testing.T) {
	skipUnlessRoot(t)

	img := createTestImage(t, 4*1024*1024)

	fd, err := unix.Open(img, unix.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer unix.Close(fd)

	ctl, err := NewControl()
	if err != nil {
		t.Fatal(err)
	}
	defer ctl.Close()

	dev, err := ctl.LoopForFD(context.Background(), fd, Options{AutoClear: true})
	if err != nil {
		t.Fatal(err)
	}
	defer dev.Close()

	if dev.Name() == "" || dev.Name() == "<closed>" {
		t.Error("expected valid devname")
	}
	if dev.Minor() < 0 {
		t.Errorf("expected non-negative minor, got %d", dev.Minor())
	}
}

func TestDeviceMknod(t *testing.T) {
	skipUnlessRoot(t)

	img := createTestImage(t, 4*1024*1024)

	fd, err := unix.Open(img, unix.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer unix.Close(fd)

	ctl, err := NewControl()
	if err != nil {
		t.Fatal(err)
	}
	defer ctl.Close()

	dev, err := ctl.LoopForFD(context.Background(), fd, Options{AutoClear: true})
	if err != nil {
		t.Fatal(err)
	}
	defer dev.Close()

	dir := t.TempDir()
	dirFD, err := unix.Open(dir, unix.O_RDONLY|unix.O_DIRECTORY, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer unix.Close(dirFD)

	if err := dev.Mknod(dirFD); err != nil {
		t.Fatal(err)
	}

	var st unix.Stat_t
	nodePath := dir + "/" + dev.Name()
	if err := unix.Stat(nodePath, &st); err != nil {
		t.Fatalf("stat created node: %v", err)
	}
	if st.Mode&unix.S_IFMT != unix.S_IFBLK {
		t.Error("created node is not a block device")
	}
}

func createTestImage(t *testing.T, size int) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "loop-test-*.img")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := f.Truncate(int64(size)); err != nil {
		t.Fatal(err)
	}
	return f.Name()
}
