package tools

import "errors"

// ExitError はプロセスの終了コードを伴うエラーです。
// 各ツールの MainCommandByOptions は、IPC 障害や引数不正など固有の終了コードを
// 返したいときにこれを返します（例: ttx-ai-send の 2/3、ttx-ai-recv の 2/4）。
type ExitError struct {
	Code int
	Err  error
}

func (e *ExitError) Error() string { return e.Err.Error() }

func (e *ExitError) Unwrap() error { return e.Err }

// NewExitError は終了コード付きエラーを作ります。
func NewExitError(code int, err error) *ExitError {
	return &ExitError{Code: code, Err: err}
}

// ExitCodeOf はエラーに対応する終了コードを返します。
// nil は 0、*ExitError（ラップ含む）はその Code、それ以外は 1 とします。
func ExitCodeOf(err error) int {
	if err == nil {
		return 0
	}
	var ee *ExitError
	if errors.As(err, &ee) {
		return ee.Code
	}
	return 1
}
