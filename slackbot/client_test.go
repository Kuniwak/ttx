package slackbot

import "testing"

func TestStripMention(t *testing.T) {
	tests := []struct {
		text, bot, want string
	}{
		{"<@U123> 直近のリリースは?", "U123", "直近のリリースは?"},
		{"<@U123>", "U123", ""},
		{"  /abort  ", "U123", "/abort"},
		{"先頭以外でも <@U123> 除去", "U123", "先頭以外でも  除去"},
	}
	for _, tt := range tests {
		if got := stripMention(tt.text, tt.bot); got != tt.want {
			t.Fatalf("stripMention(%q) = %q, want %q", tt.text, got, tt.want)
		}
	}
}
