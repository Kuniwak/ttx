// Package slackbot は Slack（Socket Mode）への接続・投稿・メンション受信を提供します。
// slack-go への依存はこのパッケージに隔離し、利用側（ttx-slack ゲートウェイなど）は
// 投稿（PostToThread）とメンション受信（Run）の最小面だけに依存します。
package slackbot

import (
	"context"
	"fmt"
	"strings"

	"github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"
	"github.com/slack-go/slack/socketmode"
)

// Mention は正規化済みの @bot メンションです。
type Mention struct {
	ChannelID string
	ThreadTS  string // スレッド内なら親スレッドの ts。チャンネル直下なら空。
	MessageTS string // このメッセージ自身の ts。
	UserText  string // 先頭の bot メンションを除いた本文。
}

// Client は Slack ワークスペースへの接続です。Socket Mode（outbound のみ）を使います。
type Client struct {
	api       *slack.Client
	sm        *socketmode.Client
	botUserID string
}

// New は Bot トークンと App-Level トークンで接続し、AuthTest で bot 自身の user ID を
// 取得します（自己発言の無視に使う）。
func New(botToken, appToken string) (*Client, error) {
	api := slack.New(botToken, slack.OptionAppLevelToken(appToken))
	auth, err := api.AuthTest()
	if err != nil {
		return nil, fmt.Errorf("Slack 認証に失敗しました: %w", err)
	}
	return &Client{api: api, sm: socketmode.New(api), botUserID: auth.UserID}, nil
}

// PostToThread は channelID の threadTS スレッドへ text を投稿します。
func (c *Client) PostToThread(channelID, threadTS, text string) error {
	_, _, err := c.api.PostMessage(channelID,
		slack.MsgOptionText(text, false),
		slack.MsgOptionTS(threadTS))
	return err
}

// Run は Socket Mode の受信ループを回し、app_mention を Mention へ正規化して handle へ
// 渡します。bot 自身の発言とメンション以外は無視します。ctx のキャンセルで終了します。
func (c *Client) Run(ctx context.Context, handle func(Mention)) error {
	go func() {
		for evt := range c.sm.Events {
			if evt.Type != socketmode.EventTypeEventsAPI {
				continue
			}
			eventsAPI, ok := evt.Data.(slackevents.EventsAPIEvent)
			if !ok {
				continue
			}
			if evt.Request != nil {
				c.sm.Ack(*evt.Request)
			}
			if eventsAPI.Type != slackevents.CallbackEvent {
				continue
			}
			mention, ok := eventsAPI.InnerEvent.Data.(*slackevents.AppMentionEvent)
			if !ok {
				continue
			}
			// bot 自身や user 不明の発言は無視（メンションループ防止）。
			if mention.User == "" || mention.User == c.botUserID {
				continue
			}
			handle(Mention{
				ChannelID: mention.Channel,
				ThreadTS:  mention.ThreadTimeStamp,
				MessageTS: mention.TimeStamp,
				UserText:  stripMention(mention.Text, c.botUserID),
			})
		}
	}()
	return c.sm.RunContext(ctx)
}

// stripMention は本文先頭の bot メンション（<@BOTID> 形式）を取り除き、前後空白を整えます。
func stripMention(text, botUserID string) string {
	text = strings.ReplaceAll(text, "<@"+botUserID+">", "")
	return strings.TrimSpace(text)
}
