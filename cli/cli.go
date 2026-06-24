package cli

import (
	"bytes"
	"io"
	"os"
	"strings"
)

type ProcInout struct {
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer
	Env    EnvFunc
}

type EnvFunc func(name string) string

func NewProcInout() *ProcInout {
	return &ProcInout{
		Stdin:  os.Stdin,
		Stdout: os.Stdout,
		Stderr: os.Stderr,
		Env:    os.Getenv,
	}
}

func StubProcInout() *ProcInout {
	return &ProcInout{
		Stdin:  io.NopCloser(strings.NewReader("")),
		Stdout: io.Discard,
		Stderr: io.Discard,
		Env:    func(name string) string { return "" },
	}
}

type ProcInoutSpy struct {
	Stdin  io.Reader
	Stdout *bytes.Buffer
	Stderr *bytes.Buffer
	Env    map[string]string
}

func (s *ProcInoutSpy) NewProcInout() *ProcInout {
	return &ProcInout{
		Stdin:  s.Stdin,
		Stdout: s.Stdout,
		Stderr: s.Stderr,
		Env:    NewEnvFunc(s.Env),
	}
}

func SpyProcInout(stdin ...string) *ProcInoutSpy {
	return &ProcInoutSpy{
		Stdin:  strings.NewReader(strings.Join(stdin, "\n")),
		Stdout: &bytes.Buffer{},
		Stderr: &bytes.Buffer{},
		Env:    make(map[string]string),
	}
}

type Command func(args []string, inout *ProcInout) int

func Run(c Command) {
	args := os.Args[1:]
	exitStatus := c(args, NewProcInout())
	os.Exit(exitStatus)
}

func NewEnvFunc(env map[string]string) EnvFunc {
	return func(name string) string {
		return env[name]
	}
}

// OverrideIfSet overwrites *dst with the first non-empty value.
// Intended for CLI flag override: short flag > long flag > existing value.
func OverrideIfSet(dst *string, values ...string) {
	for _, v := range values {
		if v != "" {
			*dst = v
			return
		}
	}
}
