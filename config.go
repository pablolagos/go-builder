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

/* ---------- YAML data types ---------- */

type StringList []string

// Accepts scalar or sequence in YAML.
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

type Target struct {
	OS     string            `yaml:"os"`
	Arch   string            `yaml:"arch"`
	Output string            `yaml:"output"`
	Env    map[string]string `yaml:"env,omitempty"`
}

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

type Config struct {
	BuildDir string            `yaml:"build_dir"`
	Source   string            `yaml:"source"`
	Output   string            `yaml:"output"`
	Env      map[string]string `yaml:"env"`
	Build    BuildSection      `yaml:"build"`
	Targets  []Target          `yaml:"targets"`
}

/* ---------- Loader ---------- */

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

/* ---------- Placeholder expansion ---------- */

func expandEnv(cfg *Config) *Config {
	expand := func(s string) string {
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
			out[expand(k)] = expand(v)
		}
		return out
	}
	out := *cfg
	out.BuildDir = expand(cfg.BuildDir)
	out.Source = expand(cfg.Source)
	out.Output = expand(cfg.Output)
	out.Env = dupMap(cfg.Env)

	// build section
	out.Build.LdFlags = func(in StringList) StringList {
		out := make(StringList, len(in))
		for i, s := range in {
			out[i] = expand(s)
		}
		return out
	}(cfg.Build.LdFlags)
	out.Build.Vars = dupMap(cfg.Build.Vars)
	out.Build.Tags = func(in []string) []string {
		out := make([]string, len(in))
		for i, s := range in {
			out[i] = expand(s)
		}
		return out
	}(cfg.Build.Tags)
	out.Build.GcFlags = expand(cfg.Build.GcFlags)
	out.Build.AsmFlags = expand(cfg.Build.AsmFlags)
	out.Build.Mod = expand(cfg.Build.Mod)

	// targets
	out.Targets = make([]Target, len(cfg.Targets))
	for i, t := range cfg.Targets {
		out.Targets[i] = Target{
			OS:     expand(t.OS),
			Arch:   expand(t.Arch),
			Output: expand(t.Output),
			Env:    dupMap(t.Env),
		}
	}
	return &out
}

/* ---------- Build-dir helpers ---------- */

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
	} else if err != nil {
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
	ign, err := dirIgnored(dir)
	if err != nil || ign {
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

/* ---------- Misc ---------- */

func mergeEnvs(g, l map[string]string) map[string]string {
	m := map[string]string{}
	for k, v := range g {
		m[k] = v
	}
	for k, v := range l {
		m[k] = v
	}
	return m
}

func envSlice(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k, v := range m {
		out = append(out, k+"="+v)
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
