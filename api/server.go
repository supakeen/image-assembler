// Package api provides Unix socket RPC servers that stages call back into
// from inside the sandbox. The socket is bind-mounted into the sandbox at
// /run/osbuild/api/<endpoint>, allowing stages to request host resources
// (like loop devices) without direct host access.
package api

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"

	"golang.org/x/sys/unix"

	ilog "github.com/supakeen/image-assembler/internal/log"
	"github.com/supakeen/image-assembler/ipc"
)

// MessageHandler processes a single RPC request from a stage inside the
// sandbox. It receives the decoded JSON message, any file descriptors
// passed via SCM_RIGHTS, and the connection socket for sending the reply.
type MessageHandler func(msg map[string]interface{}, fds *ipc.FDSet, sock *ipc.Socket)

// Server accepts connections on a Unix socket and dispatches messages to a
// handler. Each endpoint (e.g. "osbuild") gets its own Server instance,
// started before the sandbox and stopped after it exits.
type Server struct {
	Endpoint      string
	SocketAddress string
	listener      *ipc.Socket
	cancel        context.CancelFunc
	wg            sync.WaitGroup
	handler       MessageHandler
	socketDir     string
	logger        *slog.Logger
}

func NewServer(endpoint string, handler MessageHandler) *Server {
	return &Server{
		Endpoint: endpoint,
		handler:  handler,
	}
}

func (s *Server) Start(ctx context.Context) error {
	s.logger = ilog.FromContext(ctx)
	if s.SocketAddress == "" {
		dir, err := os.MkdirTemp("/run/osbuild", "api-")
		if err != nil {
			return fmt.Errorf("creating socket dir: %w", err)
		}
		s.socketDir = dir
		s.SocketAddress = filepath.Join(dir, s.Endpoint)
	}

	listener, err := ipc.Listen(s.SocketAddress)
	if err != nil {
		return fmt.Errorf("listen %s: %w", s.SocketAddress, err)
	}

	if err := listener.SetListenBacklog(5); err != nil {
		listener.Close()
		return err
	}

	s.listener = listener

	ctx, cancel := context.WithCancel(ctx)
	s.cancel = cancel

	s.wg.Add(1)
	go s.acceptLoop(ctx)

	s.logger.Debug("api server started", "endpoint", s.Endpoint, "socket", s.SocketAddress)
	return nil
}

func (s *Server) acceptLoop(ctx context.Context) {
	defer s.wg.Done()

	pollFDs := []unix.PollFd{{Fd: int32(s.listener.FD()), Events: unix.POLLIN}}

	for {
		if ctx.Err() != nil {
			return
		}

		n, err := unix.Poll(pollFDs, 100)
		if err != nil {
			if err == unix.EINTR {
				continue
			}
			return
		}
		if n == 0 {
			continue
		}

		conn, err := s.listener.Accept()
		if err != nil {
			return
		}

		s.wg.Add(1)
		go s.handleClient(ctx, conn)
	}
}

func (s *Server) handleClient(ctx context.Context, conn *ipc.Socket) {
	defer s.wg.Done()
	defer conn.Close()

	pollFDs := []unix.PollFd{{Fd: int32(conn.FD()), Events: unix.POLLIN}}

	for {
		if ctx.Err() != nil {
			return
		}

		n, err := unix.Poll(pollFDs, 100)
		if err != nil {
			if err == unix.EINTR {
				continue
			}
			return
		}
		if n == 0 {
			continue
		}

		msg, fds, err := conn.Recv()
		if err != nil || msg == nil {
			if fds != nil {
				fds.Close()
			}
			return
		}

		s.handler(msg, fds, conn)
	}
}

func (s *Server) Stop() {
	if s.cancel != nil {
		s.cancel()
	}
	s.wg.Wait()
	if s.listener != nil {
		s.listener.Close()
		s.listener = nil
	}
	if s.logger != nil {
		s.logger.Debug("api server stopped", "endpoint", s.Endpoint)
	}
}

func (s *Server) Cleanup() {
	s.Stop()
	if s.socketDir != "" {
		os.RemoveAll(s.socketDir)
	}
}
