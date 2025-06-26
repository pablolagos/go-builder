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
	"strings"
	"time"
)

/* ---------- embedded template ---------- */

//go:embed example/example.yml
var exampleYAML string

/* ---------- CLI flags ---------- */

var (
	cfgPath = flag.String("config", ".gobuilder.yml",
		"Path to YAML config file (default .gobuilder.yml)")
	initCfg = flag.Bool("init", false,
		"Create a sample .gobuilder.yml and exit (alias -i)")
	dryRun = flag.Bool("dry-run", false,
		"Print commands but do not execute (alias -n)")
	force = flag.Bool("force", false,
		"Overwrite existing file in --init mode without asking (alias -f)")
)

func init() {
	flag.BoolVar(initCfg, "i", false, "Alias for --init")
	flag.BoolVar(dryRun, "n", false, "Alias for --dry-run")
	flag.BoolVar(force, "f", false, "Alias for --force")
}

/* ---------- main ---------- */

func main() {
	flag.Parse()

	/* --init ------------------------------------------------------------ */
	if *initCfg {
		if err := createExampleConfig(".gobuilder.yml", *force); err != nil {
			log.Fatalf("go-builder: %v", err)
		}
		fmt.Println(".gobuilder.yml written.")
		return
	}

	/* load & expand ----------------------------------------------------- */
	cfg, err := LoadConfig(*cfgPath)
	if err != nil {
		log.Fatalf("go-builder: %v", err)
	}
	cfg = expandEnv(cfg)
	if cfg.Build.Debug {
		*dryRun = true
	}

	/* build-dir --------------------------------------------------------- */
	if err := ensureBuildDir(cfg.BuildDir); err != nil {
		log.Fatalf("go-builder: %v", err)
	}

	/* compile (single or matrix) --------------------------------------- */
	base := cfg.Output
	if base == "" {
		base = filepath.Base(cfg.Source)
	}

	if len(cfg.Targets) == 0 {
		out := filepath.Join(cfg.BuildDir, runtime.GOOS, runtime.GOARCH, base)
		if runtime.GOOS == "windows" && !strings.HasSuffix(out, ".exe") {
			out += ".exe"
		}
		if err := runBuild(cfg, envSlice(mergeEnvs(cfg.Env, nil)), out, *dryRun); err != nil {
			log.Fatalf("go-builder: %v", err)
		}
		return
	}

	for _, t := range cfg.Targets {
		envMap := mergeEnvs(cfg.Env, t.Env)
		envMap["GOOS"], envMap["GOARCH"] = t.OS, t.Arch
		out := t.Output
		if out == "" {
			out = filepath.Join(cfg.BuildDir, t.OS, t.Arch, base)
			if t.OS == "windows" && !strings.HasSuffix(out, ".exe") {
				out += ".exe"
			}
		}
		fmt.Printf(">>> Building %s/%s → %s\n", t.OS, t.Arch, out)
		if err := runBuild(cfg, envSlice(envMap), out, *dryRun); err != nil {
			log.Fatalf("go-builder: %v", err)
		}
	}
}

/* ---------- executor / dry-run printer ---------- */

func runBuild(cfg *Config, env []string, output string, dry bool) error {
	args := []string{"build"}
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
	if lf := composeLdflags(cfg.Build.LdFlags, cfg.Build.Vars); lf != "" {
		args = append(args, "-ldflags", lf)
	}
	if output != "" {
		args = append(args, "-o", output)
	}
	args = append(args, cfg.Source)

	if dry {
		fmt.Println("\n# Dry-run:")
		for _, kv := range env {
			if strings.HasPrefix(kv, "PWD=") {
				continue
			}
			fmt.Printf("%s \\\n", kv)
		}
		fmt.Printf("go %s\n\n", strings.Join(args, " "))
		return nil
	}

	t0 := time.Now()
	cmd := exec.Command("go", args...)
	cmd.Env = env
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		return err
	}
	fmt.Printf("✔ Completed in %s\n", time.Since(t0).Round(time.Millisecond))
	return nil
}

/* ---------- --init helper ---------- */

// createExampleConfig writes the template, optionally asking before overwrite.
func createExampleConfig(path string, overwrite bool) error {
	if _, err := os.Stat(path); err == nil { // exists
		if !overwrite {
			fmt.Printf("%s already exists – overwrite? [y/N]: ", path)
			var ans string
			fmt.Scanln(&ans)
			ans = strings.ToLower(strings.TrimSpace(ans))
			if ans != "y" && ans != "yes" {
				return fmt.Errorf("aborted by user")
			}
		}
	}
	return os.WriteFile(path, []byte(exampleYAML), 0o644)
}
