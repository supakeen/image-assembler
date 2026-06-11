// Package sandbox wraps bubblewrap (bwrap) to run osbuild stages in isolated
// namespaces (mount, PID, network, IPC). Each stage runs with a minimal
// filesystem assembled from bind mounts of the build tree, host /usr, and
// API sockets. The sandbox ensures stages cannot interfere with each other
// or the host.
package sandbox

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/supakeen/image-assembler/api"
	ilog "github.com/supakeen/image-assembler/internal/log"
)

// Sandbox manages the lifecycle of an isolated execution environment.
// Usage: New → Open (creates /dev, tmpfs, API servers) → Run (one or more
// stage invocations) → Close (tears down mounts and temp dirs).
type Sandbox struct {
	rootDir   string
	runner    string
	libdir    string
	varDir    string

	dev       string
	varPath   string
	tmpPath   string
	procDir   string

	apis      []*api.Server
	logger    *slog.Logger
	MountBoot bool
	Caps      map[string]struct{}
}

// RunOptions configures a single stage execution within the sandbox.
type RunOptions struct {
	Binds         []string
	ReadonlyBinds []string
	ExtraEnv      map[string]string
	Timeout       *int
	LiveOutput    io.Writer
}

// Result captures the exit code and combined stdout/stderr of a stage run.
type Result struct {
	ReturnCode int
	Output     string
}

func New(root, runner, libdir, varDir string) *Sandbox {
	return &Sandbox{
		rootDir:   root,
		runner:    runner,
		libdir:    libdir,
		varDir:    varDir,
		MountBoot: true,
	}
}

func (s *Sandbox) RegisterAPI(srv *api.Server) {
	s.apis = append(s.apis, srv)
}

func (s *Sandbox) DevPath() string {
	return s.dev
}

func (s *Sandbox) Open(ctx context.Context) error {
	s.logger = ilog.FromContext(ctx)
	os.MkdirAll("/run/osbuild", 0o755)

	dev, err := os.MkdirTemp("/run/osbuild", "osbuild-dev-")
	if err != nil {
		return fmt.Errorf("creating dev tmpdir: %w", err)
	}
	s.dev = dev

	if err := exec.CommandContext(ctx, "mount", "-t", "tmpfs", "-o", "nosuid", "none", s.dev).Run(); err != nil {
		os.RemoveAll(dev)
		return fmt.Errorf("mounting dev tmpfs: %w", err)
	}

	for _, name := range []string{"full", "null", "random", "urandom", "tty", "zero"} {
		if err := s.bindDev(name); err != nil {
			s.Close()
			return err
		}
	}

	os.MkdirAll(s.varDir, 0o755)
	tmpPath, err := os.MkdirTemp(s.varDir, "osbuild-tmp-")
	if err != nil {
		s.Close()
		return fmt.Errorf("creating tmp dir: %w", err)
	}
	s.tmpPath = tmpPath

	s.varPath = filepath.Join(s.tmpPath, "var")
	os.MkdirAll(s.varPath, 0o755)
	os.MkdirAll(filepath.Join(s.varPath, "tmp"), 0o1777)

	s.procDir = filepath.Join(s.tmpPath, "proc")
	os.MkdirAll(s.procDir, 0o755)
	os.WriteFile(filepath.Join(s.procDir, "cmdline"), []byte("root=/dev/osbuild\n"), 0o644)

	for _, srv := range s.apis {
		if err := srv.Start(ctx); err != nil {
			s.Close()
			return fmt.Errorf("starting api %s: %w", srv.Endpoint, err)
		}
	}

	s.logger.Info("sandbox opened", "dev", s.dev, "root", s.rootDir)
	return nil
}

func (s *Sandbox) bindDev(name string) error {
	dest := filepath.Join(s.dev, name)
	src := filepath.Join("/dev", name)

	f, err := os.Create(dest)
	if err != nil {
		return fmt.Errorf("creating dev node %s: %w", name, err)
	}
	f.Close()

	if err := exec.Command("mount", "--bind", src, dest).Run(); err != nil {
		return fmt.Errorf("bind mount /dev/%s: %w", name, err)
	}
	return nil
}

func (s *Sandbox) Close() error {
	for _, srv := range s.apis {
		srv.Stop()
	}

	if s.dev != "" {
		if out, err := exec.Command("umount", "-R", s.dev).CombinedOutput(); err != nil && s.logger != nil {
			s.logger.Warn("sandbox umount failed", "dev", s.dev, "error", err, "output", string(out))
		}
		os.RemoveAll(s.dev)
		s.dev = ""
	}

	if s.tmpPath != "" {
		os.RemoveAll(s.tmpPath)
		s.tmpPath = ""
	}

	if s.logger != nil {
		s.logger.Info("sandbox closed")
	}
	return nil
}

func (s *Sandbox) Run(ctx context.Context, argv []string, opts RunOptions) (*Result, error) {
	cmd := []string{
		"bwrap",
		"--chdir", "/",
		"--die-with-parent",
		"--new-session",
		"--unshare-ipc",
		"--unshare-pid",
		"--unshare-net",
	}

	cmd = append(cmd, BuildCapArgs(s.Caps)...)

	var mounts []string

	if s.MountBoot {
		bootDir := filepath.Join(s.rootDir, "boot")
		if info, err := os.Stat(bootDir); err == nil && info.IsDir() {
			mounts = append(mounts, "--ro-bind", bootDir, "/boot")
		}
	}

	usrDir := filepath.Join(s.rootDir, "usr")
	if info, err := os.Lstat(usrDir); err == nil && info.IsDir() && info.Mode()&os.ModeSymlink == 0 {
		mounts = append(mounts, "--ro-bind", usrDir, "/usr")
	}

	mounts = append(mounts,
		"--symlink", "usr/lib", "/lib",
		"--symlink", "usr/lib64", "/lib64",
		"--symlink", "usr/bin", "/bin",
		"--symlink", "usr/sbin", "/sbin",
	)

	mounts = append(mounts, "--dev-bind", s.dev, "/dev")
	mounts = append(mounts, "--tmpfs", "/dev/shm")

	mounts = append(mounts,
		"--dir", "/etc",
		"--tmpfs", "/run",
		"--tmpfs", "/tmp",
		"--bind", s.varPath, "/var",
	)

	mounts = append(mounts,
		"--proc", "/proc",
		"--ro-bind", "/sys", "/sys",
		"--ro-bind-try", "/sys/fs/selinux", "/sys/fs/selinux",
	)

	mounts = append(mounts,
		"--ro-bind-try", filepath.Join(s.rootDir, "etc/mke2fs.conf"), "/etc/mke2fs.conf",
		"--ro-bind-try", filepath.Join(s.rootDir, "etc/containers"), "/etc/containers",
		"--ro-bind-try", filepath.Join(s.rootDir, "ostree"), "/ostree",
		"--ro-bind-try", filepath.Join(s.rootDir, "etc/selinux"), "/etc/selinux",
	)

	mounts = append(mounts, "--ro-bind", s.libdir, "/run/osbuild/lib")

	osbuildModDir := filepath.Join(s.libdir, "osbuild")
	if entries, err := os.ReadDir(osbuildModDir); err != nil || len(entries) == 0 {
		if modPath := findOsbuildModule(); modPath != "" {
			mounts = append(mounts, "--ro-bind", modPath, "/run/osbuild/lib/osbuild")
		}
	}

	mounts = append(mounts,
		"--ro-bind", filepath.Join(s.procDir, "cmdline"), "/proc/cmdline",
	)

	for _, b := range opts.Binds {
		parts := strings.SplitN(b, ":", 2)
		if len(parts) == 2 {
			mounts = append(mounts, "--bind", parts[0], parts[1])
		}
	}
	for _, b := range opts.ReadonlyBinds {
		parts := strings.SplitN(b, ":", 2)
		if len(parts) == 2 {
			mounts = append(mounts, "--ro-bind", parts[0], parts[1])
		}
	}

	mounts = append(mounts, "--dir", "/run/osbuild/api")
	for _, a := range s.apis {
		apiPath := "/run/osbuild/api/" + a.Endpoint
		mounts = append(mounts, "--bind", a.SocketAddress, apiPath)
	}

	runnerName := filepath.Base(s.runner)
	runnerTarget := "/run/osbuild/runner/" + runnerName
	mounts = append(mounts, "--ro-bind", s.runner, runnerTarget)

	cmd = append(cmd, mounts...)
	cmd = append(cmd, "--", runnerTarget)
	cmd = append(cmd, argv...)

	env := []string{
		"container=bwrap-osbuild",
		"LC_CTYPE=C.UTF-8",
		"PATH=/usr/sbin:/usr/bin",
		"PYTHONPATH=/run/osbuild/lib",
		"PYTHONUNBUFFERED=1",
		"TERM=" + envOrDefault("TERM", "dumb"),
	}
	for k, v := range opts.ExtraEnv {
		env = append(env, k+"="+v)
	}

	runCtx := ctx
	if opts.Timeout != nil && *opts.Timeout > 0 {
		timeoutCtx, cancel := context.WithTimeout(ctx, time.Duration(*opts.Timeout)*time.Second)
		defer cancel()
		runCtx = timeoutCtx
	}

	s.logger.Info("sandbox run", "command", argv[0])
	s.logger.Debug("sandbox run details", "bwrap_args", cmd, "env", env)

	proc := exec.CommandContext(runCtx, cmd[0], cmd[1:]...)
	proc.Env = env
	proc.Stdin = nil

	var output bytes.Buffer
	var w io.Writer = &output
	if opts.LiveOutput != nil {
		w = io.MultiWriter(&output, opts.LiveOutput)
	}
	proc.Stdout = w
	proc.Stderr = w

	err := proc.Run()

	result := &Result{
		Output: output.String(),
	}

	if proc.ProcessState != nil {
		result.ReturnCode = proc.ProcessState.ExitCode()
	}

	if err != nil && result.ReturnCode == 0 {
		return result, err
	}

	return result, nil
}

func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func findOsbuildModule() string {
	out, err := exec.Command("python3", "-c", "import osbuild; import os; print(os.path.dirname(osbuild.__file__))").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
