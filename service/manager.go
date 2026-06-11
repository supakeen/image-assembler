// Package service manages Python osbuild service subprocesses (stages,
// sources, devices, inputs, mounts). Each service is a Python process
// communicating over a Unix SOCK_SEQPACKET socket using the JSON-RPC-like
// protocol defined in protocol.go, matching Python's osbuild/host.py.
package service

import (
	"bufio"
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"sync"

	ilog "github.com/supakeen/image-assembler/internal/log"
	"github.com/supakeen/image-assembler/ipc"
)

// Client represents a single running service process and its IPC connection.
// CallWithFDs supports the signal/reply/exception message loop that Python
// services use for progress reporting and error propagation.
type Client struct {
	uid  string
	proc *exec.Cmd
	sock *ipc.Socket
}

func (c *Client) Call(ctx context.Context, method string, args interface{}) (interface{}, error) {
	ret, _, err := c.CallWithFDs(ctx, method, args, nil, nil)
	return ret, err
}

func (c *Client) CallWithFDs(ctx context.Context, method string, args interface{}, fds []int,
	onSignal func(interface{}, *ipc.FDSet)) (interface{}, *ipc.FDSet, error) {

	msg := EncodeMethod(method, args)
	if err := c.sock.Send(msg, fds); err != nil {
		return nil, nil, fmt.Errorf("sending method call: %w", err)
	}

	for {
		if err := ctx.Err(); err != nil {
			return nil, nil, err
		}

		ret, retFDs, err := c.sock.Recv()
		if err != nil {
			return nil, nil, fmt.Errorf("receiving reply: %w", err)
		}
		if ret == nil {
			return nil, nil, fmt.Errorf("connection closed")
		}

		kind, data, err := DecodeMessage(ret)
		if err != nil {
			if retFDs != nil {
				retFDs.Close()
			}
			return nil, nil, err
		}

		switch kind {
		case "signal":
			reply, _ := DecodeReply(data)
			if onSignal != nil {
				onSignal(reply, retFDs)
			} else if retFDs != nil {
				retFDs.Close()
			}
			continue

		case "reply":
			reply, err := DecodeReply(data)
			if err != nil {
				if retFDs != nil {
					retFDs.Close()
				}
				return nil, nil, err
			}
			return reply, retFDs, nil

		case "exception":
			if retFDs != nil {
				retFDs.Close()
			}
			re, err := DecodeException(data)
			if err != nil {
				return nil, nil, err
			}
			return nil, nil, re

		default:
			if retFDs != nil {
				retFDs.Close()
			}
			return nil, nil, &ProtocolError{Msg: fmt.Sprintf("unknown message type: %s", kind)}
		}
	}
}

func (c *Client) Stop() {
	if c.sock != nil {
		c.sock.Close()
		c.sock = nil
	}
	if c.proc != nil {
		c.proc.Wait()
	}
}

// Manager tracks running service processes and shuts them down in reverse
// start order on Close, ensuring child processes that depend on earlier
// services are stopped first.
type Manager struct {
	services map[string]*Client
	order    []string
	libdir   string
	logger   *slog.Logger
	mu       sync.Mutex
}

func NewManager(libdir string) *Manager {
	return &Manager{
		services: make(map[string]*Client),
		libdir:   libdir,
		logger:   ilog.Discard(),
	}
}

func (m *Manager) SetLogger(logger *slog.Logger) {
	m.logger = logger
}

func (m *Manager) Start(ctx context.Context, uid, cmd string, extraArgs []string) (*Client, error) {
	m.mu.Lock()
	if _, exists := m.services[uid]; exists {
		m.mu.Unlock()
		return nil, fmt.Errorf("service %s already started", uid)
	}
	m.mu.Unlock()

	ours, theirs, err := ipc.Pair()
	if err != nil {
		return nil, fmt.Errorf("creating socket pair: %w", err)
	}

	env, err := m.makeEnv()
	if err != nil {
		ours.Close()
		theirs.Close()
		return nil, fmt.Errorf("building environment: %w", err)
	}

	argv := []string{cmd, "--service-id", uid, "--service-fd", "3"}
	if len(extraArgs) > 0 {
		argv = append(argv, extraArgs...)
	}

	theirFD := theirs.FD()
	theirFile := os.NewFile(uintptr(theirFD), "service-socket")
	// theirFile now owns this fd — don't close via theirs to avoid double-close
	theirs = nil

	proc := exec.CommandContext(ctx, argv[0], argv[1:]...)
	proc.Env = env
	proc.Stdin = nil
	proc.Stdout = nil
	proc.Stderr = nil
	proc.ExtraFiles = []*os.File{theirFile}

	stdinR, stdinW, _ := os.Pipe()
	proc.Stdin = stdinR

	stdoutR, stdoutW, err := os.Pipe()
	if err != nil {
		ours.Close()
		theirFile.Close()
		return nil, fmt.Errorf("creating stdout pipe: %w", err)
	}
	proc.Stdout = stdoutW
	proc.Stderr = stdoutW

	if err := proc.Start(); err != nil {
		ours.Close()
		theirFile.Close()
		stdoutR.Close()
		stdoutW.Close()
		return nil, fmt.Errorf("starting service %s: %w", uid, err)
	}

	theirFile.Close()
	stdoutW.Close()
	stdinW.Close()
	stdinR.Close()

	go m.readStdout(uid, filepath.Base(cmd), stdoutR)

	client := &Client{
		uid:  uid,
		proc: proc,
		sock: ours,
	}

	m.mu.Lock()
	m.services[uid] = client
	m.order = append(m.order, uid)
	m.mu.Unlock()

	m.logger.Info("service started", "uid", uid, "command", cmd)
	return client, nil
}

func (m *Manager) Stop(uid string) error {
	m.mu.Lock()
	client, ok := m.services[uid]
	if !ok {
		m.mu.Unlock()
		return fmt.Errorf("unknown service: %s", uid)
	}
	delete(m.services, uid)
	for i, u := range m.order {
		if u == uid {
			m.order = append(m.order[:i], m.order[i+1:]...)
			break
		}
	}
	m.mu.Unlock()

	client.Stop()
	m.logger.Info("service stopped", "uid", uid)
	return nil
}

func (m *Manager) Close() error {
	m.mu.Lock()
	m.logger.Debug("manager closing", "service_count", len(m.order), "order", m.order)
	order := make([]string, len(m.order))
	copy(order, m.order)
	services := m.services
	m.services = make(map[string]*Client)
	m.order = nil
	m.mu.Unlock()

	for i := len(order) - 1; i >= 0; i-- {
		if client, ok := services[order[i]]; ok {
			client.Stop()
		}
	}
	return nil
}

func (m *Manager) makeEnv() ([]string, error) {
	pythonPath, err := findOsbuildPythonPath()
	if err != nil {
		return nil, err
	}

	env := os.Environ()
	env = append(env, "PYTHONPATH="+pythonPath, "PYTHONUNBUFFERED=1")
	return env, nil
}

func findOsbuildPythonPath() (string, error) {
	out, err := exec.Command("python3", "-c",
		"import osbuild; import os; print(os.path.dirname(osbuild.__path__[0]))").Output()
	if err != nil {
		return "", fmt.Errorf("finding osbuild Python path: %w", err)
	}
	path := filepath.Clean(string(out[:len(out)-1]))
	return path, nil
}

func (m *Manager) readStdout(uid, name string, r *os.File) {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		fmt.Fprintf(os.Stderr, "%s (%s): %s\n", uid, name, scanner.Text())
	}
	r.Close()
}
