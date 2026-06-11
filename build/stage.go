package build

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/supakeen/image-assembler/api"
	ilog "github.com/supakeen/image-assembler/internal/log"
	"github.com/supakeen/image-assembler/manifest"
	"github.com/supakeen/image-assembler/monitor"
	"github.com/supakeen/image-assembler/sandbox"
	"github.com/supakeen/image-assembler/service"
	"github.com/supakeen/image-assembler/store"
)

type monitorWriter struct {
	mon monitor.Monitor
}

func (w *monitorWriter) Write(p []byte) (int, error) {
	w.mon.Log(string(p), "org.osbuild")
	return len(p), nil
}

// BuildTree provides the root filesystem path for a build environment. Both
// store.Object (for pipelines that build inside another pipeline's tree) and
// store.HostTree (for host-based builds) satisfy this interface.
type BuildTree interface {
	Tree() string
}

// RunStage executes a single stage in a bubblewrap sandbox. It sets up API
// servers (store, loop, osbuild), opens devices/inputs/mounts via their
// respective Python service processes, writes the stage arguments JSON, and
// runs the stage binary inside the sandbox. Resources are torn down in
// reverse order when the stage completes.
func RunStage(ctx context.Context, stage *manifest.Stage, treePath, metaName string,
	runner manifest.Runner, buildTree BuildTree, st *store.Store,
	libdir string, timeout *int, mon monitor.Monitor) (*BuildResult, error) {

	logger := ilog.FromContext(ctx)
	logger.Info("running stage", "name", stage.Name(), "id", stage.ID())

	sb := sandbox.New(buildTree.Tree(), runner.Path, libdir, filepath.Join(st.Root(), "tmp"))
	sb.MountBoot = stage.Build != nil

	caps := make(map[string]struct{})
	for k := range sandbox.DefaultCaps {
		caps[k] = struct{}{}
	}
	for k := range stage.Caps {
		caps[k] = struct{}{}
	}
	sb.Caps = caps

	tmpDir, cleanup, err := st.TempDir(ctx, "buildroot-tmp-")
	if err != nil {
		return nil, fmt.Errorf("creating tmpdir: %w", err)
	}
	defer cleanup()

	inputsDir := filepath.Join(tmpDir, "inputs")
	os.MkdirAll(inputsDir, 0o755)

	mountsDir := filepath.Join(tmpDir, "mounts")
	os.MkdirAll(mountsDir, 0o755)

	apiDir := filepath.Join(tmpDir, "api")
	os.MkdirAll(apiDir, 0o755)
	argsPath := filepath.Join(apiDir, "arguments")

	devices := make(map[string]interface{})
	inputs := make(map[string]interface{})
	mounts := make(map[string]interface{})

	storeAPI := api.NewStoreServer(st)
	defer storeAPI.CleanupMounts()
	sb.RegisterAPI(storeAPI.Server)

	osbuildAPI := api.NewOsbuildServer()
	sb.RegisterAPI(osbuildAPI.Server)

	loopAPI := api.NewLoopServer()
	defer loopAPI.CleanupDevices()
	sb.RegisterAPI(loopAPI.Server)

	if err := sb.Open(ctx); err != nil {
		return nil, fmt.Errorf("opening sandbox: %w", err)
	}
	defer sb.Close()

	mgr := service.NewManager(libdir)
	mgr.SetLogger(logger)
	defer mgr.Close()

	devMgr := NewDeviceManager(mgr, sb.DevPath(), treePath)
	ipMgr := NewInputManager(mgr, storeAPI, inputsDir)
	mntMgr := NewMountManager(devMgr, mountsDir)

	var firstErr error

	stage.Inputs.Range(func(key string, ip *manifest.Input) bool {
		reply, err := ipMgr.Map(ctx, ip)
		if err != nil {
			firstErr = err
			return false
		}
		inputs[key] = reply
		return true
	})
	if firstErr != nil {
		return nil, firstErr
	}

	stage.Devices.Range(func(name string, dev *manifest.Device) bool {
		reply, err := devMgr.Open(ctx, dev)
		if err != nil {
			firstErr = err
			return false
		}
		devices[name] = reply
		return true
	})
	if firstErr != nil {
		return nil, firstErr
	}

	stage.Mounts.Range(func(key string, mnt *manifest.Mount) bool {
		reply, err := mntMgr.Mount(ctx, mnt)
		if err != nil {
			firstErr = err
			return false
		}
		mounts[key] = reply
		return true
	})
	if firstErr != nil {
		return nil, firstErr
	}

	args := map[string]interface{}{
		"tree": "/run/osbuild/tree",
		"paths": map[string]interface{}{
			"devices": "/dev",
			"inputs":  "/run/osbuild/inputs",
			"mounts":  "/run/osbuild/mounts",
		},
		"devices": devices,
		"inputs":  inputs,
		"mounts":  mounts,
		"options": stage.Options,
		"meta": map[string]interface{}{
			"id": stage.ID(),
		},
	}

	if stage.SourceEpoch != nil {
		meta := args["meta"].(map[string]interface{})
		meta["source-epoch"] = *stage.SourceEpoch
	}

	reroot(args)

	argsJSON, err := json.Marshal(args)
	if err != nil {
		return nil, fmt.Errorf("marshaling stage arguments: %w", err)
	}
	if err := os.WriteFile(argsPath, argsJSON, 0o644); err != nil {
		return nil, fmt.Errorf("writing arguments: %w", err)
	}

	binds := []string{
		treePath + ":/run/osbuild/tree",
		metaName + ":/run/osbuild/meta",
		mountsDir + ":/run/osbuild/mounts",
	}

	roBinds := []string{
		stage.ExecPath + ":/run/osbuild/bin/" + stage.Name(),
		inputsDir + ":/run/osbuild/inputs",
		argsPath + ":/run/osbuild/api/arguments",
	}

	extraEnv := make(map[string]string)
	if stage.SourceEpoch != nil {
		extraEnv["SOURCE_DATE_EPOCH"] = strconv.Itoa(*stage.SourceEpoch)
	}

	runOpts := sandbox.RunOptions{
		Binds:         binds,
		ReadonlyBinds: roBinds,
		ExtraEnv:      extraEnv,
		Timeout:       timeout,
	}
	if mon != nil {
		runOpts.LiveOutput = &monitorWriter{mon: mon}
	}

	r, err := sb.Run(ctx, []string{"/run/osbuild/bin/" + stage.Name()}, runOpts)
	if err != nil {
		return nil, fmt.Errorf("running stage: %w", err)
	}

	return &BuildResult{
		Stage:      stage,
		ReturnCode: r.ReturnCode,
		Output:     r.Output,
		Error:      osbuildAPI.Error,
	}, nil
}

func reroot(args map[string]interface{}) {
	paths, ok := args["paths"].(map[string]interface{})
	if !ok {
		return
	}

	for name, root := range paths {
		rootStr, ok := root.(string)
		if !ok {
			continue
		}
		group, ok := args[name].(map[string]interface{})
		if !ok {
			continue
		}
		for _, item := range group {
			itemMap, ok := item.(map[string]interface{})
			if !ok {
				continue
			}
			path, ok := itemMap["path"].(string)
			if !ok || path == "" {
				continue
			}
			itemMap["path"] = filepath.Join(rootStr, path)
		}
	}
}
