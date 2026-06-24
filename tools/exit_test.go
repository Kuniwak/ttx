package tools

import (
	"errors"
	"fmt"
	"testing"
)

func TestExitCodeOf(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want int
	}{
		{"nil は 0", nil, 0},
		{"素のエラーは 1", errors.New("boom"), 1},
		{"ExitError は Code を返す", NewExitError(3, errors.New("usage")), 3},
		{"ExitError をラップしても Code を返す", fmt.Errorf("wrap: %w", NewExitError(4, errors.New("empty"))), 4},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ExitCodeOf(tt.err); got != tt.want {
				t.Fatalf("ExitCodeOf(%v) = %d, want %d", tt.err, got, tt.want)
			}
		})
	}
}

func TestExitErrorUnwrapAndMessage(t *testing.T) {
	base := errors.New("接続できません")
	ee := NewExitError(2, base)
	if ee.Error() != base.Error() {
		t.Fatalf("Error() = %q, want %q", ee.Error(), base.Error())
	}
	if !errors.Is(ee, base) {
		t.Fatal("Unwrap で元エラーをたどれるべき")
	}
}
