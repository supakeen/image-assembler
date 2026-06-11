// CLI entry point reimplementing Python osbuild's main_cli.py. Reads a v2
// manifest, validates it against module schemas, resolves pipeline
// dependencies, downloads sources, executes stages in sandboxed containers,
// and exports the resulting artifacts.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/supakeen/image-assembler/build"
	ilog "github.com/supakeen/image-assembler/internal/log"
	"github.com/supakeen/image-assembler/manifest"
	"github.com/supakeen/image-assembler/monitor"
	"github.com/supakeen/image-assembler/registry"
	"github.com/supakeen/image-assembler/store"
)

type stringSlice []string

func (s *stringSlice) String() string { return strings.Join(*s, ",") }
func (s *stringSlice) Set(v string) error {
	*s = append(*s, v)
	return nil
}

func main() {
	os.Exit(run())
}

func run() int {
	var checkpoints stringSlice
	var exports stringSlice

	inspect := flag.Bool("inspect", false, "Print manifest with IDs and exit")
	libdir := flag.String("libdir", "/usr/lib/osbuild", "Directory containing stages/sources/runners")
	cacheDir := flag.String("cache", ".osbuild", "Object store location")
	flag.Var(&checkpoints, "checkpoint", "Stage to checkpoint (can be repeated, accepts globs)")
	flag.Var(&exports, "export", "Pipeline to export (can be repeated)")
	outputDir := flag.String("output-directory", "", "Where to write exported artifacts")
	jsonOutput := flag.Bool("json", false, "Output results in JSON format")
	monitorName := flag.String("monitor", "", "Monitor name: LogMonitor, JSONSeqMonitor, NullMonitor")
	monitorFD := flag.Int("monitor-fd", 1, "File descriptor for monitor output")
	stageTimeout := flag.Int("stage-timeout", 0, "Maximum seconds per stage")
	quiet := flag.Bool("quiet", false, "Suppress normal output")
	flag.BoolFunc("q", "Suppress normal output", func(string) error { *quiet = true; return nil })
	cacheMaxSize := flag.String("cache-max-size", "", "Maximum cache size (e.g. 1GiB, 500MB, unlimited)")
	logLevel := flag.String("log-level", "", "Log level: trace, debug, info, warn (default: off)")
	version := flag.Bool("version", false, "Print version and exit")

	flag.Parse()

	if *version {
		fmt.Println("osbuild (image-assembler) 0.1.0")
		return 0
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if *logLevel != "" {
		level, err := ilog.ParseLevel(*logLevel)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			return 2
		}
		logger := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{
			Level:       level,
			ReplaceAttr: ilog.ReplaceLevelAttr,
		}))
		ctx = ilog.WithLogger(ctx, logger)
	}

	manifestPath := flag.Arg(0)
	if manifestPath == "" {
		fmt.Fprintln(os.Stderr, "Usage: osbuild [flags] MANIFEST")
		return 2
	}

	var r io.Reader
	if manifestPath == "-" {
		r = os.Stdin
	} else {
		f, err := os.Open(manifestPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			return 1
		}
		defer f.Close()
		r = f
	}

	var desc map[string]interface{}
	dec := json.NewDecoder(r)
	dec.UseNumber()
	if err := dec.Decode(&desc); err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing manifest JSON: %v\n", err)
		return 1
	}

	reg := registry.New(*libdir)
	reg.SetLogger(ilog.FromContext(ctx))

	vr := manifest.Validate(ctx, desc, reg)
	if !vr.Valid() {
		fmt.Fprint(os.Stderr, manifest.FormatValidationErrors(vr))
		return 2
	}

	m, err := manifest.Parse(ctx, desc, reg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	for _, e := range exports {
		p := m.Pipeline(e)
		if p == nil {
			p = m.PipelineByID(e)
		}
		if p == nil {
			fmt.Fprintf(os.Stderr, "Error: unknown export %q\n", e)
			return 1
		}
	}

	var marked map[string]struct{}
	if len(checkpoints) > 0 {
		marked = m.MarkCheckpoints(checkpoints)
		if len(marked) == 0 {
			fmt.Fprintln(os.Stderr, "Error: no checkpoints matched")
			return 1
		}
	}

	if *inspect {
		result := manifest.Describe(m, true)
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(result); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			return 1
		}
		return 0
	}

	if len(exports) > 0 && *outputDir == "" {
		fmt.Fprintln(os.Stderr, "Error: --output-directory required when using --export")
		return 1
	}

	st, err := store.Open(ctx, *cacheDir, false)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error opening store: %v\n", err)
		return 1
	}
	defer st.Close()

	if *cacheMaxSize != "" {
		size, err := store.ParseSize(*cacheMaxSize)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			return 2
		}
		if err := st.SetMaximumSize(size); err != nil {
			fmt.Fprintf(os.Stderr, "Error setting cache max size: %v\n", err)
			return 1
		}
	}

	pipelines, err := m.Depsolve(ctx, st, exports)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error resolving dependencies: %v\n", err)
		return 1
	}

	if len(pipelines) == 0 {
		for i := range m.Pipelines {
			pipelines = append(pipelines, m.Pipelines[i].Name)
		}
	}

	totalSteps := len(m.Sources) + len(pipelines)

	name := *monitorName
	if name == "" {
		if *jsonOutput || *quiet {
			name = "NullMonitor"
		} else {
			name = "LogMonitor"
		}
	}

	mon, err := monitor.Make(name, *monitorFD, totalSteps)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating monitor: %v\n", err)
		return 1
	}

	logger := ilog.FromContext(ctx)
	logger.Info("starting build", "manifest", manifestPath, "pipelines", pipelines)

	mon.Log(fmt.Sprintf("starting %s\n", manifestPath), "osbuild.main_cli")

	if err := build.DownloadAll(ctx, m, st, *libdir, mon); err != nil {
		fmt.Fprintf(os.Stderr, "Error downloading sources: %v\n", err)
		return 1
	}

	var timeout *int
	if *stageTimeout > 0 {
		timeout = stageTimeout
	}

	res, err := build.Build(ctx, m, st, pipelines, mon, *libdir, timeout)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	success, _ := res["success"].(bool)

	if success {
		mon.Log(fmt.Sprintf("manifest %s finished successfully\n", manifestPath), "osbuild.main_cli")
	} else {
		mon.Log(fmt.Sprintf("manifest %s failed\n", manifestPath), "osbuild.main_cli")
	}

	if success && len(exports) > 0 {
		for _, e := range exports {
			if err := build.Export(ctx, e, *outputDir, st, m); err != nil {
				fmt.Fprintf(os.Stderr, "Error exporting %s: %v\n", e, err)
				return 1
			}
		}
	}

	if *jsonOutput {
		output := build.FormatResult(ctx, m, res, st)
		enc := json.NewEncoder(os.Stdout)
		if err := enc.Encode(output); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			return 1
		}
	} else if !*quiet {
		if success {
			fmt.Fprintln(os.Stderr, "\nPipelines")
			for i := range m.Pipelines {
				fmt.Fprintf(os.Stderr, "  %-10s\t%s\n", m.Pipelines[i].Name+":", m.Pipelines[i].ID())
			}

			if marked != nil && len(marked) > 0 {
				fmt.Fprintln(os.Stderr, "\nCheckpoints")
				for id := range marked {
					if st.Contains(id) {
						fmt.Fprintf(os.Stderr, "  %s: cached\n", id)
					} else {
						fmt.Fprintf(os.Stderr, "  %s: not cached\n", id)
					}
				}
			}
		} else {
			fmt.Fprintln(os.Stderr, "Failed")
		}
	}

	logger.Info("build complete", "success", success)

	if success {
		return 0
	}
	return 1
}
