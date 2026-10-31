package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

type commandRunner interface {
	Run(name string, args []string, env []string) error
}

type osRunner struct{ dir string }

func (r osRunner) Run(name string, args []string, env []string) error {
	command := exec.Command(name, args...)
	command.Dir = r.dir
	command.Stdin = os.Stdin
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	command.Env = env
	return command.Run()
}

var lookPath = exec.LookPath

func npmInvocation(goos, node, npm string, args []string) (string, []string) {
	extension := strings.ToLower(filepath.Ext(npm))
	if goos == "windows" && (extension == ".cmd" || extension == ".bat") {
		npmCLI := filepath.Join(filepath.Dir(npm), "node_modules", "npm", "bin", "npm-cli.js")
		return node, append([]string{npmCLI}, args...)
	}
	return npm, args
}

func (a app) executeNPM(node, npm string, args []string) error {
	program, programArgs := npmInvocation(runtime.GOOS, node, npm, args)
	if runtime.GOOS == "windows" && program == node && len(programArgs) > 0 && !fileExists(programArgs[0]) {
		return fmt.Errorf("npm CLI was not found next to npm.cmd: %s", programArgs[0])
	}
	return a.runner.Run(program, programArgs, os.Environ())
}

func (a app) prepareNode(install bool) (node, npm string, err error) {
	node, err = lookPath("node")
	if err != nil {
		return "", "", errors.New("node was not found; install Node.js 22 or newer")
	}
	npm, err = lookPath("npm")
	if err != nil {
		return "", "", errors.New("npm was not found; install Node.js with npm")
	}
	if install {
		astro := filepath.Join(a.root, "node_modules", "astro", "bin", "astro.mjs")
		if _, statErr := os.Stat(astro); errors.Is(statErr, os.ErrNotExist) {
			fmt.Fprintln(a.out, "[setup] installing npm dependencies")
			if runErr := a.executeNPM(node, npm, []string{"ci"}); runErr != nil {
				return "", "", fmt.Errorf("npm ci: %w", runErr)
			}
		}
	}
	return node, npm, nil
}

func (a app) runNPM(prepare bool, args ...string) error {
	node, npm, err := a.prepareNode(prepare)
	if err != nil {
		return err
	}
	if err := a.executeNPM(node, npm, args); err != nil {
		return fmt.Errorf("npm %s: %w", strings.Join(args, " "), err)
	}
	return nil
}

func (a app) runDiagrams(args []string) error {
	if len(args) > 1 {
		return errors.New("usage: blogctl diagrams [plantuml|drawio]")
	}
	target := "diagrams"
	if len(args) == 1 {
		if args[0] != "plantuml" && args[0] != "drawio" {
			return errors.New("usage: blogctl diagrams [plantuml|drawio]")
		}
		target += ":" + args[0]
	}
	return a.runNPM(true, "run", target)
}
