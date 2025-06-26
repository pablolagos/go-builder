package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

/* ------------------------------------------------------------------
   Utilities to run a build inside Docker by shelling out to `docker`
   ------------------------------------------------------------------ */

// dockerRun executes the given shell commands inside a disposable container.
func dockerRun(cfg *Config, cmds []string, dry bool) error {
	c := cfg.Docker

	image := c.Image
	if image == "" {
		image = "docker.io/golang:latest"
	}
	workdir := c.WorkDir
	if workdir == "" {
		workdir = "/work"
	}
	shell := c.Shell
	if shell == "" {
		shell = "sh"
	}

	hostDir, _ := os.Getwd()
	mount := fmt.Sprintf("%s:%s", hostDir, workdir)

	// Merge env layers: host env kept, global env + docker.env appended.
	envArgs := []string{}
	for k, v := range mergeEnvLayers(nil, cfg.Env, c.Env) {
		envArgs = append(envArgs, "-e", fmt.Sprintf("%s=%s", k, v))
	}
	runArgs := append([]string{
		"run", "--rm", "-w", workdir, "-v", mount,
	}, envArgs...)
	runArgs = append(runArgs, image, shell, "-c", strings.Join(cmds, " && "))

	if dry {
		fmt.Printf("\n# Dry-run: docker %s\n", strings.Join(runArgs, " "))
		return nil
	}
	cmd := exec.Command("docker", runArgs...)
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	return cmd.Run()
}
