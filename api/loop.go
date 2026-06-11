package api

import (
	"context"

	"github.com/supakeen/image-assembler/ipc"
	"github.com/supakeen/image-assembler/loop"
)

// LoopServer handles loop device requests from stages inside the sandbox.
// A stage sends a file descriptor and device options; the server attaches
// it to a loop device and creates the device node in the sandbox's /dev.
// All allocated devices are released when CleanupDevices is called.
type LoopServer struct {
	*Server
	devs []*loop.Device
	ctl  *loop.Control
}

func NewLoopServer() *LoopServer {
	s := &LoopServer{}
	s.Server = NewServer("remoteloop", s.handle)
	return s
}

func (s *LoopServer) handle(msg map[string]interface{}, fds *ipc.FDSet, sock *ipc.Socket) {
	if fds == nil {
		sock.Send(map[string]interface{}{"error": "no file descriptors"}, nil)
		return
	}
	defer fds.Close()

	fdIdx, _ := toInt(msg["fd"])
	dirFDIdx, _ := toInt(msg["dir_fd"])

	fd, err := fds.Steal(fdIdx)
	if err != nil {
		sock.Send(map[string]interface{}{"error": "invalid fd index"}, nil)
		return
	}

	dirFD, err := fds.Steal(dirFDIdx)
	if err != nil {
		sock.Send(map[string]interface{}{"error": "invalid dir_fd index"}, nil)
		return
	}

	if s.ctl == nil {
		s.ctl, err = loop.NewControl()
		if err != nil {
			sock.Send(map[string]interface{}{"error": err.Error()}, nil)
			return
		}
	}

	opts := loop.Options{
		AutoClear: true,
	}

	if v, ok := toUint64(msg["offset"]); ok {
		opts.Offset = v
	}
	if v, ok := toUint64(msg["sizelimit"]); ok {
		opts.SizeLimit = v
	}
	if v, ok := msg["lock"].(bool); ok {
		opts.Lock = v
	}
	if v, ok := msg["partscan"].(bool); ok {
		opts.PartScan = v
	}
	if v, ok := msg["read_only"].(bool); ok {
		opts.ReadOnly = v
	}
	if v, ok := toUint32(msg["sector_size"]); ok {
		opts.BlockSize = v
	}

	dev, err := s.ctl.LoopForFD(context.Background(), fd, opts)
	if err != nil {
		sock.Send(map[string]interface{}{"error": err.Error()}, nil)
		return
	}

	if err := dev.Mknod(dirFD); err != nil {
		dev.Close()
		sock.Send(map[string]interface{}{"error": err.Error()}, nil)
		return
	}

	s.devs = append(s.devs, dev)
	if s.logger != nil {
		s.logger.Debug("loop device attached", "device", dev.Name())
	}
	sock.Send(map[string]interface{}{"devname": dev.Name()}, nil)
}

func (s *LoopServer) CleanupDevices() {
	for _, d := range s.devs {
		d.Close()
	}
	s.devs = nil
	if s.ctl != nil {
		s.ctl.Close()
		s.ctl = nil
	}
}

func toInt(v interface{}) (int, bool) {
	switch n := v.(type) {
	case float64:
		return int(n), true
	case int:
		return n, true
	}
	return 0, false
}

func toUint64(v interface{}) (uint64, bool) {
	switch n := v.(type) {
	case float64:
		return uint64(n), true
	case int:
		return uint64(n), true
	}
	return 0, false
}

func toUint32(v interface{}) (uint32, bool) {
	switch n := v.(type) {
	case float64:
		return uint32(n), true
	case int:
		return uint32(n), true
	}
	return 0, false
}
