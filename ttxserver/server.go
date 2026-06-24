package ttxserver

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/Kuniwak/ttx/ttxipc"
)

// Hooks は呼び出し側（ttx-human / Slack ゲートウェイなど）固有の差分を注入します。
// いずれも必須です。呼び出し側の意図を明示させ、書き忘れによる沈黙的な機能低下を防ぐため、
// 「何もしない」場合も no-op を明示的に渡してください（nil フォールバックは設けません）。
type Hooks struct {
	// OnListening は listen 開始後に socket パスを通知する必須フックです。
	// ttx-human は接続先バナーを表示します。通知が不要な呼び出し側（Slack ゲートウェイ等）は
	// no-op 関数 func(string){} を渡してください（NullObject）。
	OnListening func(socketPath string)
	// LogWriter は参加者に見えたメッセージ（public_log）の必須の記録先です。
	// 記録を抑制したい場合は io.Discard を明示的に渡してください。
	LogWriter io.Writer
}

// Serve は1本の socket を listen し、接続ごとに req/resp を処理します。
// 接続が閉じてもプロセスは終了せず、次の接続を待ち続けます（GM 側ツールは
// 接続＝1回の req/resp で send/recv するため、socket を開きっぱなしにしません）。
//
// 終了契機は次の3系統です。
//   - Frontend.ReadLine が EOF（ttx-human の stdin 終端、Slack の Frontend.Close など）
//   - ctx のキャンセル（Slack ゲートウェイが演習終了を検知したとき）
//   - 演習終了条件の検知 → Frontend.Finish（保留中の表示を出し切ってから ReadLine を EOF にする）。
//     条件: ポストモーテム表示（handleConn）／ /abort 入力（pumpHumanInput）／
//     recv のアイドルタイムアウト（handleRecv が timeout 応答を返した後、handleConn）。
//
// ctx キャンセルは socket 側（accept ループ・recv の待機）を畳みます。入力ポンプの
// ブロック（ReadLine）は Frontend.Close で解除する想定なので、確実に停止させる呼び出し側は
// ctx のキャンセルと Frontend.Close の両方を行ってください。
//
// recvTimeout は recv 待機のアイドルタイムアウトです。会話の最後のアクティビティから
// この時間だけ参加者入力が無いと、recv は timeout 応答（ttxipc.TypeTimeout）を返します。
// 0 以下なら無期限（タイムアウト無し）です。
func Serve(ctx context.Context, socketPath string, fe Frontend, hooks Hooks, recvTimeout time.Duration) error {
	// 必須の依存は fail-fast で検証する（nil フォールバックを設けない方針）。
	if hooks.LogWriter == nil {
		panic("ttxserver.Serve: Hooks.LogWriter は必須です（記録を抑制する場合は io.Discard を渡す）")
	}
	if hooks.OnListening == nil {
		panic("ttxserver.Serve: Hooks.OnListening は必須です（不要なら no-op func(string){} を渡す）")
	}

	_ = os.Remove(socketPath)

	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		return fmt.Errorf("Unix Domain Socketの起動に失敗しました: %w", err)
	}
	defer listener.Close()
	defer os.Remove(socketPath)

	logWriter := hooks.LogWriter

	// 起動時に、演習で使ってよい道具を全参加者へ明示する。入力ポンプと accept ループより
	// 前に同期表示するので、/help 応答や GM の最初のメッセージより必ず先に出る。
	fe.DisplayLocal(announceText)

	queue := newInputQueue()
	var writeMu sync.Mutex

	// 人間入力ポンプを常駐させる。frontend が EOF/エラーになったら listener を閉じて
	// accept ループを抜けさせ、ブロック中の recv も解除する。
	pumpErr := make(chan error, 1)
	go func() {
		err := pumpHumanInput(fe, queue, logWriter, &writeMu)
		queue.Close()
		listener.Close()
		pumpErr <- err
	}()

	// ctx キャンセルで socket 側を畳む（accept ループを抜けさせ、ブロック中の recv も解除）。
	// Serve を返した後に監視 goroutine が残らないよう、終了時に stop を閉じて抜けさせる。
	stop := make(chan struct{})
	defer close(stop)
	go func() {
		select {
		case <-ctx.Done():
			queue.Close()
			listener.Close()
		case <-stop:
		}
	}()

	// 接続先の案内は呼び出し側のフックへ委ねる（端末では frontend 経由で表示し、
	// raw mode 中に stderr へ直書きして初期画面を崩さないため）。
	hooks.OnListening(socketPath)
	for {
		conn, err := listener.Accept()
		if err != nil {
			// listener が閉じられた（ポンプ終了 or ctx キャンセル）なら抜ける。
			break
		}
		go handleConn(conn, fe, queue, logWriter, &writeMu, recvTimeout)
	}

	// ポンプの戻り（Frontend が EOF/Close で解除）を待つ。ctx キャンセルが先なら
	// それを優先して戻る（ポンプは Frontend.Close で解除され、buffered な pumpErr へ送るため
	// goroutine は滞留しない）。
	select {
	case err = <-pumpErr:
		if errors.Is(err, io.EOF) {
			return nil
		}
		return err
	case <-ctx.Done():
		return nil
	}
}

// pumpHumanInput は人間入力を読み続け、入力キューへ積みます。
// /help（別名 ?）は会話プロセスがローカルで応答し、キューへは積みません（GMへ送らない）。
func pumpHumanInput(fe Frontend, queue *inputQueue, logFile io.Writer, writeMu *sync.Mutex) error {
	for {
		line, err := fe.ReadLine()
		if err != nil {
			if errors.Is(err, io.EOF) {
				return io.EOF
			}
			return err
		}
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}

		cmd := matchCommand(trimmed)
		if cmd != nil && cmd.Name == cmdHelp {
			fe.DisplayLocal(HelpText())
			writeMu.Lock()
			err := writeJSONLine(logFile, ttxipc.Message{Type: "help", Text: trimmed})
			writeMu.Unlock()
			if err != nil {
				return err
			}
			continue
		}

		if cmd != nil && cmd.Name == cmdAbort {
			// /abort: 演習を中断し会話プロセスを自動終了する。まず GM へ確実に届ける
			// （GM の --wait recv は exit 0 で /abort を取得＝定義済みの中断パスに乗る。
			// 届けずにキューを閉じると recv が exit 4 を返すが、GM 側はその扱いを定義して
			// いないため避ける）。次にローカルへ別れの文言を出してから Finish で終端する
			// （GM の中断通知は会話プロセス終了後に届くので表示されない）。
			msg := HumanMessage(trimmed) // type:"control", text:"/abort …"
			writeMu.Lock()
			err := writeJSONLine(logFile, msg)
			writeMu.Unlock()
			if err != nil {
				return err
			}
			queue.Push(msg)
			fe.DisplayLocal(abortFarewellText)
			fe.Finish()
			continue
		}

		msg := HumanMessage(trimmed)
		writeMu.Lock()
		err = writeJSONLine(logFile, msg)
		writeMu.Unlock()
		if err != nil {
			return err
		}
		// キューへ積み、応答待ち表示に入る（GMメッセージ表示で解除）。
		queue.Push(msg)
		fe.SetThinking(true)
	}
}

// handleConn は1接続を1回の req/resp として処理します。
// 先頭1行で種別を判定し、送信（GM→人間表示）か recv（人間入力の取り出し）に分岐します。
func handleConn(conn net.Conn, fe Frontend, queue *inputQueue, logFile io.Writer, writeMu *sync.Mutex, recvTimeout time.Duration) {
	defer conn.Close()

	req, err := readLine(conn)
	if err != nil {
		// 空接続・壊れた行は黙って閉じる。
		return
	}

	if req.Type == ttxipc.TypeRecv {
		if handleRecv(conn, queue, req, recvTimeout) {
			// アイドルタイムアウト（/abort 相当の打ち切り）。timeout 応答を GM へ返した後に
			// 会話プロセスを自動終了する（以後 GM は送信しないため開きっぱなしは誤解を招く）。
			fe.Finish()
		}
		return
	}
	if _, err := ttxipc.ParseSendType(req.Type); err == nil {
		// GM 送信は会話アクティビティ。アイドル基準を更新して、参加者の次返信に猶予を与える。
		queue.Touch()
		displayAgentMessage(fe, logFile, writeMu, req)
		// ポストモーテム表示＝演習の正常終了。表示後に会話プロセスを自動終了する
		// （以後 GM は送信しないため、開きっぱなしは追加入力が拾われず誤解を招く）。
		if req.Type == string(ttxipc.TypePostmortem) {
			fe.Finish()
		}
		return
	}
	// 想定外の type は GM 側の誤り。error を返して閉じる（送信側は読まなくてもよい）。
	_ = writeJSONLine(conn, ttxipc.Message{Type: string(ttxipc.TypeError), Text: fmt.Sprintf("不正なtype: %q", req.Type)})
}

// handleRecv は入力キューから1件取り出して応答します。
// wait なら入力が来るまで（recvTimeout が正ならアイドルタイムアウトまで）ブロック、
// そうでなければ滞留が無ければ応答せず閉じます。
// アイドルタイムアウトに達した場合（timeout 応答を返した場合）だけ timedOut=true を返し、
// 呼び出し側（handleConn）が会話プロセスを自動終了させます。
func handleRecv(conn net.Conn, queue *inputQueue, req ttxipc.Message, recvTimeout time.Duration) (timedOut bool) {
	if req.Text != ttxipc.RecvWait {
		// no-wait: 滞留があれば1件返し、無ければ応答せず閉じる（クライアントは「入力なし」と判定）。
		if msg, ok := queue.PopNow(); ok {
			deliver(conn, queue, msg)
		}
		return false
	}

	msg, outcome := queue.PopWaitTimeout(recvTimeout)
	switch outcome {
	case popMessage:
		deliver(conn, queue, msg)
	case popTimeout:
		// アイドルタイムアウト。timeout 応答を返してから（GM 側は終了コードで打ち切りを受け取り
		// abort 相当で終了する）会話プロセスを自動終了させる。参加者入力ではないので
		// 失敗しても Unpop はしない。
		_ = writeJSONLine(conn, ttxipc.Message{Type: ttxipc.TypeTimeout})
		return true
	default:
		// popClosed（キュー閉鎖）。応答せず閉じる → クライアントは「入力なし」と判定。
	}
	return false
}

// deliver は recv 応答を1行書き、配信失敗時は取りこぼさないようキュー先頭へ戻します。
func deliver(conn net.Conn, queue *inputQueue, msg ttxipc.Message) {
	if err := writeJSONLine(conn, msg); err != nil {
		queue.Unpop(msg)
	}
}

// displayAgentMessage は GM メッセージを人間へ表示し、公開ログへ記録します。
func displayAgentMessage(fe Frontend, logFile io.Writer, writeMu *sync.Mutex, msg ttxipc.Message) {
	fe.DisplayAgent(msg)
	// 表示後の応答待ち表示は msg.Thinking で決める。既定（false）はいずれのGMメッセージ
	// 到着でも解除（IPCは応答相関を持たない）。thinking:true（演習開始通知など）のときは
	// 表示後も点けたままにし、次のフラグなしメッセージの表示で解除される。表示自体も
	// 応答待ちを解除するため、ここは表示の後に呼んで最終状態を確定させる。
	fe.SetThinking(msg.Thinking)
	writeMu.Lock()
	_ = writeJSONLine(logFile, msg)
	writeMu.Unlock()
}
