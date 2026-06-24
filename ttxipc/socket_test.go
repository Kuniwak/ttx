package ttxipc

import (
	"os"
	"strings"
	"testing"
)

func envFrom(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestDefaultSocketPathPrefersEnvSocket(t *testing.T) {
	// TTX_SOCKET があれば XDG_RUNTIME_DIR より優先する。
	getenv := envFrom(map[string]string{
		EnvSocket:         "/custom/ttx.sock",
		"XDG_RUNTIME_DIR": "/run/user/1000",
	})
	if got := DefaultSocketPath(getenv); got != "/custom/ttx.sock" {
		t.Fatalf("DefaultSocketPath = %q, want /custom/ttx.sock", got)
	}
}

func TestDefaultSocketPathUsesXDGRuntimeDir(t *testing.T) {
	getenv := envFrom(map[string]string{"XDG_RUNTIME_DIR": "/run/user/1000"})
	if got := DefaultSocketPath(getenv); got != "/run/user/1000/ttx.sock" {
		t.Fatalf("DefaultSocketPath = %q, want /run/user/1000/ttx.sock", got)
	}
}

func TestDefaultSocketPathFallsBackToTempDirWithUID(t *testing.T) {
	// env が空なら os.TempDir() 配下に uid 付きの名前で作る。
	getenv := envFrom(map[string]string{})
	got := DefaultSocketPath(getenv)
	if !strings.HasPrefix(got, os.TempDir()) {
		t.Fatalf("DefaultSocketPath = %q, want prefix %q", got, os.TempDir())
	}
	if !strings.Contains(got, "ttx-") || !strings.HasSuffix(got, ".sock") {
		t.Fatalf("DefaultSocketPath = %q, want ttx-<uid>.sock", got)
	}
}
