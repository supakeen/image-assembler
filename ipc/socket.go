// Package ipc provides Unix SOCK_SEQPACKET sockets with SCM_RIGHTS file
// descriptor passing, reimplementing Python osbuild's jsoncomm.py. Messages
// are JSON-encoded and may be accompanied by file descriptors. When a message
// exceeds the kernel's wmem_max, it is transparently sent via a memfd to avoid
// fragmentation.
package ipc

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strconv"
	"strings"

	ilog "github.com/supakeen/image-assembler/internal/log"
	"golang.org/x/sys/unix"
)

var argsViaFDMarker = []byte("<args-via-fd>")

type unlinkInfo struct {
	dirfd int
	name  string
}

// Socket wraps a Unix SOCK_SEQPACKET file descriptor. It handles JSON
// serialization, SCM_RIGHTS fd passing, and automatic cleanup of bound
// socket paths on close.
type Socket struct {
	fd     int
	unlink *unlinkInfo
	logger *slog.Logger
}

func (s *Socket) SetLogger(logger *slog.Logger) {
	s.logger = logger
}

func (s *Socket) log() *slog.Logger {
	if s.logger != nil {
		return s.logger
	}
	return ilog.Discard()
}

func Dial(path string) (*Socket, error) {
	fd, err := unix.Socket(unix.AF_UNIX, unix.SOCK_SEQPACKET|unix.SOCK_CLOEXEC, 0)
	if err != nil {
		return nil, fmt.Errorf("creating socket: %w", err)
	}

	if err := unix.Bind(fd, &unix.SockaddrUnix{Name: ""}); err != nil {
		unix.Close(fd)
		return nil, fmt.Errorf("auto-bind: %w", err)
	}

	if err := unix.Connect(fd, &unix.SockaddrUnix{Name: path}); err != nil {
		unix.Close(fd)
		return nil, fmt.Errorf("connecting to %s: %w", path, err)
	}

	s := &Socket{fd: fd}
	s.log().Log(nil, ilog.LevelTrace, "socket dial", "path", path)
	return s, nil
}

func Listen(path string) (*Socket, error) {
	fd, err := unix.Socket(unix.AF_UNIX, unix.SOCK_SEQPACKET|unix.SOCK_CLOEXEC, 0)
	if err != nil {
		return nil, fmt.Errorf("creating socket: %w", err)
	}

	if err := unix.Bind(fd, &unix.SockaddrUnix{Name: path}); err != nil {
		unix.Close(fd)
		return nil, fmt.Errorf("binding to %s: %w", path, err)
	}

	if err := unix.SetNonblock(fd, true); err != nil {
		unix.Close(fd)
		return nil, fmt.Errorf("setting non-blocking: %w", err)
	}

	parts := strings.Split(path, "/")
	name := parts[len(parts)-1]
	dir := strings.Join(parts[:len(parts)-1], "/")
	if dir == "" {
		dir = "."
	}

	dirfd, err := unix.Open(dir, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if err != nil {
		unix.Close(fd)
		return nil, fmt.Errorf("opening parent dir: %w", err)
	}

	s := &Socket{
		fd:     fd,
		unlink: &unlinkInfo{dirfd: dirfd, name: name},
	}
	s.log().Log(nil, ilog.LevelTrace, "socket listen", "path", path)
	return s, nil
}

// Pair creates a connected socket pair for parent-child IPC. One end is
// passed to the child process via ExtraFiles, the other is kept by the manager.
func Pair() (*Socket, *Socket, error) {
	fds, err := unix.Socketpair(unix.AF_UNIX, unix.SOCK_SEQPACKET|unix.SOCK_CLOEXEC, 0)
	if err != nil {
		return nil, nil, fmt.Errorf("socketpair: %w", err)
	}
	return &Socket{fd: fds[0]}, &Socket{fd: fds[1]}, nil
}

// FromFD wraps an existing fd (typically inherited from a parent process) into
// a Socket. The fd is dup'd so the caller retains independent ownership.
func FromFD(fd int, closeFD bool) (*Socket, error) {
	newfd, err := unix.Dup(fd)
	if err != nil {
		return nil, fmt.Errorf("dup fd: %w", err)
	}
	unix.CloseOnExec(newfd)
	if closeFD {
		unix.Close(fd)
	}
	return &Socket{fd: newfd}, nil
}

func (s *Socket) FD() int {
	return s.fd
}

func (s *Socket) SetBlocking(blocking bool) error {
	return unix.SetNonblock(s.fd, !blocking)
}

func (s *Socket) Accept() (*Socket, error) {
	nfd, _, err := unix.Accept(s.fd)
	if err != nil {
		return nil, fmt.Errorf("accept: %w", err)
	}
	unix.CloseOnExec(nfd)
	return &Socket{fd: nfd}, nil
}

func (s *Socket) SetListenBacklog(backlog int) error {
	return unix.Listen(s.fd, backlog)
}

func (s *Socket) Send(payload interface{}, fds []int) error {
	serialized, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshaling payload: %w", err)
	}

	s.log().Log(nil, ilog.LevelTrace, "ipc send", "size", len(serialized), "fds", len(fds))

	if len(serialized) > wmemMax() {
		return s.sendViaFD(serialized, fds)
	}
	return s.sendViaSendmsg(serialized, fds)
}

func (s *Socket) sendViaSendmsg(serialized []byte, fds []int) error {
	var oob []byte
	if len(fds) > 0 {
		oob = unix.UnixRights(fds...)
	}

	unix.SetsockoptInt(s.fd, unix.SOL_SOCKET, unix.SO_SNDBUF, len(serialized))

	n, err := unix.SendmsgN(s.fd, serialized, oob, nil, 0)
	if err != nil {
		return fmt.Errorf("sendmsg: %w", err)
	}
	if n != len(serialized) {
		return fmt.Errorf("sendmsg: short write %d/%d", n, len(serialized))
	}
	return nil
}

func (s *Socket) sendViaFD(serialized []byte, fds []int) error {
	memfd, err := unix.MemfdCreate("jsoncomm/payload", 0)
	if err != nil {
		return fmt.Errorf("memfd_create: %w", err)
	}
	defer unix.Close(memfd)

	if _, err := unix.Write(memfd, serialized); err != nil {
		return fmt.Errorf("writing to memfd: %w", err)
	}
	if _, err := unix.Seek(memfd, 0, io.SeekStart); err != nil {
		return fmt.Errorf("seeking memfd: %w", err)
	}

	allFDs := make([]int, 0, 1+len(fds))
	allFDs = append(allFDs, memfd)
	allFDs = append(allFDs, fds...)

	oob := unix.UnixRights(allFDs...)
	n, err := unix.SendmsgN(s.fd, argsViaFDMarker, oob, nil, 0)
	if err != nil {
		return fmt.Errorf("sendmsg (via fd): %w", err)
	}
	if n != len(argsViaFDMarker) {
		return fmt.Errorf("sendmsg (via fd): short write %d/%d", n, len(argsViaFDMarker))
	}
	return nil
}

func (s *Socket) Recv() (map[string]interface{}, *FDSet, error) {
	size := 4096
	for {
		buf := make([]byte, size)
		n, _, flags, _, err := unix.Recvmsg(s.fd, buf, nil, unix.MSG_PEEK)
		if err != nil {
			return nil, nil, fmt.Errorf("recvmsg peek: %w", err)
		}
		if n == 0 {
			return nil, nil, nil
		}
		if flags&unix.MSG_TRUNC == 0 {
			break
		}
		size *= 2
	}

	buf := make([]byte, size)
	cmsgBuf := make([]byte, unix.CmsgSpace(253*4))

	n, oobn, flags, _, err := unix.Recvmsg(s.fd, buf, cmsgBuf, 0)
	if err != nil {
		return nil, nil, fmt.Errorf("recvmsg: %w", err)
	}

	var fds []int
	if oobn > 0 {
		cmsgs, err := unix.ParseSocketControlMessage(cmsgBuf[:oobn])
		if err != nil {
			return nil, nil, fmt.Errorf("parsing control messages: %w", err)
		}
		for _, cmsg := range cmsgs {
			if cmsg.Header.Level == unix.SOL_SOCKET && cmsg.Header.Type == unix.SCM_RIGHTS {
				rights, err := unix.ParseUnixRights(&cmsg)
				if err != nil {
					return nil, nil, fmt.Errorf("parsing unix rights: %w", err)
				}
				fds = append(fds, rights...)
			}
		}
	}

	data := buf[:n]
	var fdset *FDSet
	var serialized []byte

	if bytes.Equal(data, argsViaFDMarker) {
		if len(fds) == 0 {
			return nil, nil, fmt.Errorf("args-via-fd marker but no fds received")
		}
		payloadFD := fds[0]
		defer unix.Close(payloadFD)

		payload, err := readAll(payloadFD)
		if err != nil {
			closeFDs(fds[1:])
			return nil, nil, fmt.Errorf("reading payload from fd: %w", err)
		}
		serialized = payload
		fdset = NewFDSet(fds[1:])
	} else {
		serialized = data
		fdset = NewFDSet(fds)
	}

	if flags&(unix.MSG_TRUNC|unix.MSG_CTRUNC) != 0 {
		if fdset != nil {
			fdset.Close()
		}
		return nil, nil, fmt.Errorf("message truncated (flags=0x%x)", flags)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(serialized, &result); err != nil {
		if fdset != nil {
			fdset.Close()
		}
		return nil, nil, fmt.Errorf("unmarshaling payload: %w", err)
	}

	s.log().Log(nil, ilog.LevelTrace, "ipc recv", "size", len(serialized), "fds", len(fds))

	return result, fdset, nil
}

func (s *Socket) Close() error {
	if s.fd < 0 {
		return nil
	}

	s.log().Log(nil, ilog.LevelTrace, "socket closed", "fd", s.fd)
	err := unix.Close(s.fd)
	s.fd = -1

	if s.unlink != nil {
		unix.Unlinkat(s.unlink.dirfd, s.unlink.name, 0)
		unix.Close(s.unlink.dirfd)
		s.unlink = nil
	}

	return err
}

func wmemMax() int {
	data, err := os.ReadFile("/proc/sys/net/core/wmem_max")
	if err != nil {
		return 64000
	}
	val, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return 64000
	}
	return val
}

func readAll(fd int) ([]byte, error) {
	var buf bytes.Buffer
	tmp := make([]byte, 32768)
	for {
		n, err := unix.Read(fd, tmp)
		if n > 0 {
			buf.Write(tmp[:n])
		}
		if err != nil {
			return nil, err
		}
		if n == 0 {
			break
		}
	}
	return buf.Bytes(), nil
}

func closeFDs(fds []int) {
	for _, fd := range fds {
		if fd >= 0 {
			unix.Close(fd)
		}
	}
}
