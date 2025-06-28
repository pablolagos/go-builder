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

/* ──────────── embed example template ─────────── */

//go:embed examples/example.yml
var exampleYAML string

/* ──────────── CLI flags ─────────── */

var (
	cfgPath    = flag.String("config", ".gobuilder.yml", "Config file (default .gobuilder.yml)")
	initCfg    = flag.Bool("init", false, "Write .gobuilder.yml template (alias -i)")
	force      = flag.Bool("force", false, "Overwrite existing file without asking (alias -f)")
	dryRun     = flag.Bool("dry-run", false, "Print commands, do not execute (alias -n)")
	envMode    = flag.String("env", "diff", "Env print mode in dry-run: diff | all | none")
	skipDocker = flag.Bool("skip-docker", false, "Ignore docker section (alias -D)")
)

func init() {
	flag.BoolVar(initCfg, "i", false, "Alias for --init")
	flag.BoolVar(force, "f", false, "Alias for --force")
	flag.BoolVar(dryRun, "n", false, "Alias for --dry-run")
	flag.BoolVar(skipDocker, "D", false, "Alias for --skip-docker")
}

/* ──────────── main ─────────── */

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

	/* docker mode ------------------------------------------------------- */
	if cfg.Docker != nil && !*skipDocker {
		inner := append([]string{}, cfg.Docker.Setup...)
		inner = append(inner, "go install github.com/pablolagos/go-builder@latest")
		inner = append(inner, "go-builder --skip-docker --config=.gobuilder.yml")
		if err := dockerRun(cfg, inner, *dryRun); err != nil {
			log.Fatalf("go-builder: %v", err)
		}
		// Copy-back not implemented for --rm containers (stub in dockerutil.go).
		return
	}

	/* local build path -------------------------------------------------- */
	if err := ensureBuildDir(cfg.BuildDir); err != nil {
		log.Fatalf("go-builder: %v", err)
	}
	baseEnv := sliceToMap(os.Environ())

	baseName := cfg.Output
	if baseName == "" {
		baseName = filepath.Base(cfg.Source)
	}

	if len(cfg.Targets) == 0 {
		out := filepath.Join(cfg.BuildDir, runtime.GOOS, runtime.GOARCH, baseName)
		if runtime.GOOS == "windows" && !strings.HasSuffix(out, ".exe") {
			out += ".exe"
		}
		env := mergeEnvLayers(baseEnv, cfg.Env, nil)
		if err := runBuild(cfg, baseEnv, envSlice(env), out, *dryRun); err != nil {
			log.Fatalf("go-builder: %v", err)
		}
		return
	}

	for _, t := range cfg.Targets {
		env := mergeEnvLayers(baseEnv, cfg.Env, t.Env)
		env["GOOS"], env["GOARCH"] = t.OS, t.Arch
		out := t.Output
		if out == "" {
			out = filepath.Join(cfg.BuildDir, t.OS, t.Arch, baseName)
			if t.OS == "windows" && !strings.HasSuffix(out, ".exe") {
				out += ".exe"
			}
		}
		fmt.Printf(">>> Building %s/%s → %s\n", t.OS, t.Arch, out)
		if err := runBuild(cfg, baseEnv, envSlice(env), out, *dryRun); err != nil {
			log.Fatalf("go-builder: %v", err)
		}
	}
}

/* ──────────── build executor ─────────── */

func runBuild(cfg *Config, base map[string]string, env []string,
	output string, dry bool) error {

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

	/* dry-run ----------------------------------------------------------- */
	if dry {
		cur := sliceToMap(env)
		var show map[string]string
		switch *envMode {
		case "none":
			show = nil
		case "all":
			show = cur
		default: // diff
			show = diffEnv(base, cur)
		}
		fmt.Println("\n# Dry-run:")
		if show != nil {
			keys := make([]string, 0, len(show))
			for k := range show {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				fmt.Printf("%s=%q \\\n", k, show[k])
			}
		}
		fmt.Printf("go %s\n\n", strings.Join(args, " "))
		return nil
	}

	/* real build -------------------------------------------------------- */
	start := time.Now()
	cmd := exec.Command("go", args...)
	cmd.Env = env
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		return err
	}
	fmt.Printf("✔ completed in %s\n", time.Since(start).Round(time.Millisecond))
	return nil
}

/* ──────────── init template helper ─────────── */

func createExampleConfig(path string, overwrite bool) error {
	if _, err := os.Stat(path); err == nil && !overwrite {
		fmt.Printf("%s exists — overwrite? [y/N]: ", path)
		var ans string
		fmt.Scanln(&ans)
		ans = strings.ToLower(strings.TrimSpace(ans))
		if ans != "y" && ans != "yes" {
			return fmt.Errorf("aborted by user")
		}
	}
	return os.WriteFile(path, []byte(exampleYAML), 0o644)
}
