// Package loop manages Linux loop devices via /dev/loop-control ioctls.
// Stages that need block devices (e.g. filesystem formatters, partition
// assemblers) request loop devices through the API server, which delegates
// to this package.
package loop

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"unsafe"

	ilog "github.com/supakeen/image-assembler/internal/log"
	"golang.org/x/sys/unix"
)

const (
	LoopMajor = 7

	loopSetFD        = 0x4C00
	loopClrFD        = 0x4C01
	loopSetStatus64  = 0x4C04
	loopConfigure    = 0x4C0A
	loopCtlGetFree   = 0x4C82

	loFlagsReadOnly  = 1
	loFlagsAutoClear = 4
	loFlagsPartScan  = 8
)

type loopInfo64 struct {
	Device         uint64
	Inode          uint64
	Rdevice        uint64
	Offset         uint64
	SizeLimit      uint64
	Number         uint32
	EncryptType    uint32
	EncryptKeySize uint32
	Flags          uint32
	FileName       [64]byte
	CryptName      [64]byte
	EncryptKey     [32]byte
	Init           [2]uint64
}

type loopConfig struct {
	FD        uint32
	BlockSize uint32
	Info      loopInfo64
	Reserved  [8]uint64
}

// Options configures loop device behavior. Field names and semantics match
// the Python osbuild loop device service options.
type Options struct {
	Offset    uint64
	SizeLimit uint64
	BlockSize uint32
	Lock      bool
	PartScan  bool
	ReadOnly  bool
	AutoClear bool
}

// Control wraps /dev/loop-control for allocating free loop devices.
type Control struct {
	fd int
}

func NewControl() (*Control, error) {
	fd, err := unix.Open("/dev/loop-control", unix.O_RDWR|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, fmt.Errorf("opening /dev/loop-control: %w", err)
	}
	return &Control{fd: fd}, nil
}

func (c *Control) GetFree() (int, error) {
	minor, _, errno := unix.Syscall(unix.SYS_IOCTL, uintptr(c.fd), loopCtlGetFree, 0)
	if errno != 0 {
		return 0, fmt.Errorf("LOOP_CTL_GET_FREE: %w", errno)
	}
	return int(minor), nil
}

// LoopForFD attaches fd to a free loop device with the given options. It
// retries on EBUSY/EAGAIN (another process claimed the device between
// GetFree and configure) until it succeeds or ctx is canceled.
func (c *Control) LoopForFD(ctx context.Context, fd int, opts Options) (*Device, error) {
	logger := ilog.FromContext(ctx)

	if fd < 0 {
		return nil, fmt.Errorf("invalid file descriptor %d", fd)
	}

	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		minor, err := c.GetFree()
		if err != nil {
			return nil, err
		}

		dev, err := openDevice(minor)
		if err != nil {
			return nil, err
		}

		if opts.Lock {
			err = unix.Flock(dev.fd, unix.LOCK_EX|unix.LOCK_NB)
			if errors.Is(err, unix.EWOULDBLOCK) {
				logger.Debug("loop device locked, retrying", "minor", minor)
				dev.close()
				continue
			}
			if err != nil {
				dev.close()
				return nil, fmt.Errorf("flock: %w", err)
			}
		}

		err = dev.configure(fd, opts)
		if err == nil {
			logger.Debug("loop device allocated", "minor", minor, "device", dev.devname)
			return dev, nil
		}

		if errors.Is(err, unix.EBUSY) {
			logger.Debug("loop device busy, retrying", "minor", minor)
			dev.close()
			continue
		}

		if errors.Is(err, unix.EAGAIN) {
			logger.Debug("loop device EAGAIN, retrying", "minor", minor)
			dev.clearFD()
			dev.close()
			continue
		}

		dev.close()
		return nil, err
	}
}

func (c *Control) Close() error {
	if c.fd < 0 {
		return nil
	}
	err := unix.Close(c.fd)
	c.fd = -1
	return err
}

// Device represents an allocated loop device. Close detaches the backing
// file and releases the device number.
type Device struct {
	fd      int
	devname string
	minor   int
}

func openDevice(minor int) (*Device, error) {
	path := fmt.Sprintf("/dev/loop%d", minor)
	fd, err := unix.Open(path, unix.O_RDWR|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, fmt.Errorf("opening %s: %w", path, err)
	}

	var st unix.Stat_t
	if err := unix.Fstat(fd, &st); err != nil {
		unix.Close(fd)
		return nil, fmt.Errorf("stat %s: %w", path, err)
	}

	if st.Mode&unix.S_IFMT != unix.S_IFBLK {
		unix.Close(fd)
		return nil, fmt.Errorf("%s is not a block device", path)
	}

	if unix.Major(st.Rdev) != LoopMajor || unix.Minor(st.Rdev) != uint32(minor) {
		unix.Close(fd)
		return nil, fmt.Errorf("%s: unexpected device %d:%d", path, unix.Major(st.Rdev), unix.Minor(st.Rdev))
	}

	return &Device{
		fd:      fd,
		devname: fmt.Sprintf("loop%d", minor),
		minor:   minor,
	}, nil
}

func (d *Device) Name() string {
	return d.devname
}

func (d *Device) Minor() int {
	return d.minor
}

// Mknod creates the device node inside the sandbox's /dev. It prefers
// bind-mounting the host's /dev/loopN for compatibility with containers
// that lack CAP_MKNOD, falling back to mknodat if the host node is absent.
func (d *Device) Mknod(dirFD int) error {
	hostPath := fmt.Sprintf("/dev/%s", d.devname)
	if _, err := os.Stat(hostPath); err == nil {
		target := fmt.Sprintf("/proc/self/fd/%d/%s", dirFD, d.devname)
		if err := os.WriteFile(target, nil, 0o600); err != nil {
			return d.mknodFallback(dirFD)
		}
		if err := exec.Command("mount", "--bind", hostPath, target).Run(); err != nil {
			os.Remove(target)
			return d.mknodFallback(dirFD)
		}
		return nil
	}
	return d.mknodFallback(dirFD)
}

func (d *Device) mknodFallback(dirFD int) error {
	devNum := unix.Mkdev(LoopMajor, uint32(d.minor))
	return unix.Mknodat(dirFD, d.devname, unix.S_IFBLK|0o600, int(devNum))
}

func (d *Device) Close() error {
	if d.fd < 0 {
		return nil
	}
	d.clearFD()
	return d.close()
}

func (d *Device) configure(fd int, opts Options) error {
	var info loopInfo64
	if opts.Offset != 0 {
		info.Offset = opts.Offset
	}
	if opts.SizeLimit != 0 {
		info.SizeLimit = opts.SizeLimit
	}
	if opts.AutoClear {
		info.Flags |= loFlagsAutoClear
	}
	if opts.PartScan {
		info.Flags |= loFlagsPartScan
	}
	if opts.ReadOnly {
		info.Flags |= loFlagsReadOnly
	}

	cfg := loopConfig{
		FD:        uint32(fd),
		BlockSize: opts.BlockSize,
		Info:      info,
	}

	_, _, errno := unix.Syscall(unix.SYS_IOCTL, uintptr(d.fd), loopConfigure, uintptr(unsafe.Pointer(&cfg)))
	if errno == 0 {
		return nil
	}

	if errno != unix.EINVAL {
		return errno
	}

	// Fallback for older kernels: LOOP_SET_FD + LOOP_SET_STATUS64
	_, _, errno = unix.Syscall(unix.SYS_IOCTL, uintptr(d.fd), loopSetFD, uintptr(fd))
	if errno != 0 {
		return fmt.Errorf("LOOP_SET_FD: %w", errno)
	}

	_, _, errno = unix.Syscall(unix.SYS_IOCTL, uintptr(d.fd), loopSetStatus64, uintptr(unsafe.Pointer(&info)))
	if errno != 0 {
		return fmt.Errorf("LOOP_SET_STATUS64: %w", errno)
	}

	return nil
}

func (d *Device) clearFD() {
	unix.Syscall(unix.SYS_IOCTL, uintptr(d.fd), loopClrFD, 0)
}

func (d *Device) close() error {
	fd := d.fd
	d.fd = -1
	d.devname = "<closed>"
	return unix.Close(fd)
}
