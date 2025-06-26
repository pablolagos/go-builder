package main

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

/* ──────────────── YAML data types ──────────────── */

// StringList accepts scalar or sequence in YAML.
type StringList []string

func (s *StringList) UnmarshalYAML(n *yaml.Node) error {
	switch n.Kind {
	case yaml.ScalarNode:
		*s = []string{n.Value}
	case yaml.SequenceNode:
		for _, c := range n.Content {
			*s = append(*s, c.Value)
		}
	default:
		return errors.New("StringList: expected scalar or sequence")
	}
	return nil
}

// Target = one GOOS / GOARCH build.
type Target struct {
	OS     string            `yaml:"os"`
	Arch   string            `yaml:"arch"`
	Output string            `yaml:"output"`
	Env    map[string]string `yaml:"env,omitempty"`
}

// DockerSection controls containerised builds.
type DockerSection struct {
	Image    string            `yaml:"image"`
	WorkDir  string            `yaml:"workdir"`
	Shell    string            `yaml:"shell"`
	Setup    []string          `yaml:"setup"`
	Env      map[string]string `yaml:"env"`
	CopyBack []string          `yaml:"copy_back"`
}

// Build-level flags.
type BuildSection struct {
	Tags     []string          `yaml:"tags"`
	LdFlags  StringList        `yaml:"ldflags"`
	Vars     map[string]string `yaml:"vars"`
	GcFlags  string            `yaml:"gcflags"`
	AsmFlags string            `yaml:"asmflags"`
	Mod      string            `yaml:"mod"`
	Race     bool              `yaml:"race"`
	TrimPath bool              `yaml:"trimpath"`
	Verbose  bool              `yaml:"verbose"`
	Debug    bool              `yaml:"debug"`
}

// Top-level config.
type Config struct {
	BuildDir string            `yaml:"build_dir"`
	Source   string            `yaml:"source"`
	Output   string            `yaml:"output"`
	Env      map[string]string `yaml:"env"`
	Build    BuildSection      `yaml:"build"`
	Targets  []Target          `yaml:"targets"`
	Docker   *DockerSection    `yaml:"docker,omitempty"`
}

/* ──────────────── Load & expand ──────────────── */

func LoadConfig(path string) (*Config, error) {
	if path == "" {
		path = ".gobuilder.yml"
	}
	b, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return nil, err
	}
	var cfg Config
	if err := yaml.Unmarshal(b, &cfg); err != nil {
		return nil, err
	}
	if cfg.BuildDir == "" {
		cfg.BuildDir = "builds"
	}
	return &cfg, nil
}

// expandEnv does ${VAR} / ${VAR:-def} replacement.
func expandEnv(cfg *Config) *Config {
	exp := func(s string) string {
		return os.Expand(s, func(k string) string {
			if i := strings.Index(k, ":-"); i >= 0 {
				name, def := k[:i], k[i+2:]
				if v, ok := os.LookupEnv(name); ok && v != "" {
					return v
				}
				return def
			}
			return os.Getenv(k)
		})
	}
	dupMap := func(m map[string]string) map[string]string {
		out := make(map[string]string, len(m))
		for k, v := range m {
			out[exp(k)] = exp(v)
		}
		return out
	}
	out := *cfg
	out.BuildDir = exp(cfg.BuildDir)
	out.Source = exp(cfg.Source)
	out.Output = exp(cfg.Output)
	out.Env = dupMap(cfg.Env)

	// build section
	out.Build.LdFlags = func(in StringList) StringList {
		o := make(StringList, len(in))
		for i, s := range in {
			o[i] = exp(s)
		}
		return o
	}(cfg.Build.LdFlags)
	out.Build.Vars = dupMap(cfg.Build.Vars)
	out.Build.Tags = func(in []string) []string {
		o := make([]string, len(in))
		for i, s := range in {
			o[i] = exp(s)
		}
		return o
	}(cfg.Build.Tags)
	out.Build.GcFlags = exp(cfg.Build.GcFlags)
	out.Build.AsmFlags = exp(cfg.Build.AsmFlags)
	out.Build.Mod = exp(cfg.Build.Mod)

	// targets
	out.Targets = make([]Target, len(cfg.Targets))
	for i, t := range cfg.Targets {
		out.Targets[i] = Target{
			OS:     exp(t.OS),
			Arch:   exp(t.Arch),
			Output: exp(t.Output),
			Env:    dupMap(t.Env),
		}
	}
	// docker env expansion
	if cfg.Docker != nil {
		d := *cfg.Docker
		d.Image = exp(d.Image)
		d.WorkDir = exp(d.WorkDir)
		d.Shell = exp(d.Shell)
		d.Env = dupMap(d.Env)
		out.Docker = &d
	}
	return &out
}

/* ──────────────── Build-dir helpers ──────────────── */

func ensureBuildDir(dir string) error {
	if st, err := os.Stat(dir); err == nil && st.IsDir() {
		ignored, _ := dirIgnored(dir)
		if !ignored {
			return fmt.Errorf("directory %s exists but is not in .gitignore", dir)
		}
		return nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return appendToGitignore(dir)
}

func dirIgnored(dir string) (bool, error) {
	f, err := os.Open(".gitignore")
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == dir || line == dir+"/" {
			return true, nil
		}
	}
	return false, sc.Err()
}

func appendToGitignore(dir string) error {
	i, err := dirIgnored(dir)
	if err != nil || i {
		return err
	}
	f, err := os.OpenFile(".gitignore", os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = fmt.Fprintf(f, "%s/\n", dir)
	return err
}

/* ──────────────── Env helpers ──────────────── */

func sliceToMap(in []string) map[string]string {
	m := make(map[string]string, len(in))
	for _, kv := range in {
		if i := strings.IndexByte(kv, '='); i >= 0 {
			m[kv[:i]] = kv[i+1:]
		}
	}
	return m
}

// mergeEnvLayers: base <- global <- local
func mergeEnvLayers(base, global, local map[string]string) map[string]string {
	out := make(map[string]string, len(base)+len(global)+len(local))
	for k, v := range base {
		out[k] = v
	}
	for k, v := range global {
		out[k] = v
	}
	for k, v := range local {
		out[k] = v
	}
	return out
}

func envSlice(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k, v := range m {
		out = append(out, k+"="+v)
	}
	return out
}

// diffEnv returns keys that differ between base and cur.
func diffEnv(base, cur map[string]string) map[string]string {
	out := map[string]string{}
	for k, v := range cur {
		if base[k] != v {
			out[k] = v
		}
	}
	return out
}

func composeLdflags(ld StringList, vars map[string]string) string {
	out := make([]string, len(ld))
	copy(out, ld)
	for k, v := range vars {
		out = append(out, fmt.Sprintf("-X '%s=%s'", k, v))
	}
	return strings.Join(out, " ")
}
