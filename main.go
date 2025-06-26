// main.go
//
// Entry point for go-builder. Handles CLI flags, YAML loading, build-dir checks,
// environment composition and “go build” invocation (with optional dry-run).

package main

import (
	_ "embed"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"
)

/* ───────────────────────────────── embed template ───────────────────────── */

//go:embed example/example.yml
var exampleYAML string

/* ───────────────────────────────── CLI flags ────────────────────────────── */

var (
	cfgPath = flag.String("config", ".gobuilder.yml",
		"Path to YAML config file (default .gobuilder.yml)")
	initCfg = flag.Bool("init", false,
		"Create a sample .gobuilder.yml and exit (alias -i)")
	dryRun = flag.Bool("dry-run", false,
		"Print commands but do not execute (alias -n)")
	force = flag.Bool("force", false,
		"Overwrite existing file in --init mode without asking (alias -f)")
	envMode = flag.String("env", "diff",
		`Which env vars to show in --dry-run:
     diff  - only vars added/changed by go-builder (default)
     all   - full environment
     none  - do not print env section`)
)

func init() {
	flag.BoolVar(initCfg, "i", false, "Alias for --init")
	flag.BoolVar(dryRun, "n", false, "Alias for --dry-run")
	flag.BoolVar(force, "f", false, "Alias for --force")
}

/* ────────────────────────────────── main ────────────────────────────────── */

func main() {
	flag.Parse()

	/* ---------- --init mode ---------------------------------------------- */
	if *initCfg {
		if err := createExampleConfig(".gobuilder.yml", *force); err != nil {
			log.Fatalf("go-builder: %v", err)
		}
		fmt.Println(".gobuilder.yml written.")
		return
	}

	/* ---------- load YAML ------------------------------------------------ */
	cfg, err := LoadConfig(*cfgPath)
	if err != nil {
		log.Fatalf("go-builder: %v", err)
	}
	cfg = expandEnv(cfg) // placeholder expansion
	if cfg.Build.Debug {
		*dryRun = true // YAML can enable dry-run
	}

	/* ---------- build directory check ----------------------------------- */
	if err := ensureBuildDir(cfg.BuildDir); err != nil {
		log.Fatalf("go-builder: %v", err)
	}

	/* ---------- prepare base environment -------------------------------- */
	baseEnv := sliceToMap(os.Environ()) // keep PATH, GOPATH, HOME, …

	/* ---------- single-build branch (no matrix) ------------------------- */
	base := cfg.Output
	if base == "" {
		base = filepath.Base(cfg.Source)
	}

	if len(cfg.Targets) == 0 {
		out := filepath.Join(cfg.BuildDir, runtime.GOOS, runtime.GOARCH, base)
		if runtime.GOOS == "windows" && !strings.HasSuffix(out, ".exe") {
			out += ".exe"
		}
		env := mergeEnvLayers(baseEnv, cfg.Env, nil)
		if err := runBuild(cfg, baseEnv, envSlice(env), out, *dryRun); err != nil {
			log.Fatalf("go-builder: %v", err)
		}
		return
	}

	/* ---------- matrix build ------------------------------------------- */
	for _, t := range cfg.Targets {
		envMap := mergeEnvLayers(baseEnv, cfg.Env, t.Env)
		envMap["GOOS"], envMap["GOARCH"] = t.OS, t.Arch

		out := t.Output
		if out == "" {
			out = filepath.Join(cfg.BuildDir, t.OS, t.Arch, base)
			if t.OS == "windows" && !strings.HasSuffix(out, ".exe") {
				out += ".exe"
			}
		}
		fmt.Printf(">>> Building %s/%s → %s\n", t.OS, t.Arch, out)
		if err := runBuild(cfg, baseEnv, envSlice(envMap), out, *dryRun); err != nil {
			log.Fatalf("go-builder: %v", err)
		}
	}
}

/* ─────────────────────────── build executor ────────────────────────────── */

// runBuild assembles flags, executes “go build”, or prints them in dry-run mode.
func runBuild(cfg *Config, base map[string]string, env []string, output string, dry bool) error {
	args := []string{"build"}

	// generic flags
	if cfg.Build.Verbose {
		args = append(args, "-v")
	}
	if len(cfg.Build.Tags) > 0 {
		args = append(args, "-tags", strings.Join(cfg.Build.Tags, ","))
	}
	if cfg.Build.TrimPath {
		args = append(args, "-trimpath")
	}
	if cfg.Build.GcFlags != "" {
		args = append(args, "-gcflags", cfg.Build.GcFlags)
	}
	if cfg.Build.AsmFlags != "" {
		args = append(args, "-asmflags", cfg.Build.AsmFlags)
	}
	if cfg.Build.Mod != "" {
		args = append(args, "-mod", cfg.Build.Mod)
	}
	if cfg.Build.Race {
		args = append(args, "-race")
	}

	// ldflags & output
	if lf := composeLdflags(cfg.Build.LdFlags, cfg.Build.Vars); lf != "" {
		args = append(args, "-ldflags", lf)
	}
	if output != "" {
		args = append(args, "-o", output)
	}
	args = append(args, cfg.Source)

	/* ----- dry-run ------------------------------------------------------ */
	if dry {
		// Decide which env vars to show
		var toShow map[string]string
		switch *envMode {
		case "none":
			toShow = nil
		case "all":
			toShow = sliceToMap(env) // cur env as map
		default: // "diff"
			toShow = diffEnv(base, sliceToMap(env))
		}

		fmt.Println("\n# Dry-run:")
		if toShow != nil {
			// PWD y similares quedan fuera para no ensuciar
			keys := make([]string, 0, len(toShow))
			for k := range toShow {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				fmt.Printf("%s=%q \\\n", k, toShow[k])
			}
		}
		fmt.Printf("go %s\n\n", strings.Join(args, " "))
		return nil
	}

	/* ----- real execution ---------------------------------------------- */
	start := time.Now()
	cmd := exec.Command("go", args...)
	cmd.Env = env
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		return err
	}
	fmt.Printf("✔ Completed in %s\n", time.Since(start).Round(time.Millisecond))
	return nil
}

/* ─────────────────────── --init write helper ───────────────────────────── */

// createExampleConfig writes the embedded template, asking before overwrite unless forced.
func createExampleConfig(path string, overwrite bool) error {
	if _, err := os.Stat(path); err == nil && !overwrite {
		fmt.Printf("%s already exists — overwrite? [y/N]: ", path)
		var ans string
		fmt.Scanln(&ans)
		ans = strings.ToLower(strings.TrimSpace(ans))
		if ans != "y" && ans != "yes" {
			return fmt.Errorf("aborted by user")
		}
	}
	return os.WriteFile(path, []byte(exampleYAML), 0o644)
}
