package telegram

import (
	"context"
	"testing"

	"github.com/mymmrac/telego"
)

func privMsg(text string) telego.Update {
	return telego.Update{Message: &telego.Message{Chat: telego.Chat{Type: "private"}, Text: text}}
}

func TestPrivateNonStartPredicate(t *testing.T) {
	predicate := privateNonStart(testCommandModules(t).MemberCommandNames())
	handled := []string{
		"/pkg vim", "/use vim", "/bug 1", "/news", "/wiki x", "/bbs x",
		"/pkgs firefox", "/distro firefox", "/arm htop", "/armpkgs htop",
		"/kernel", "/man ls", "/cve CVE-2024-3094", "/repology bash",
		"/help", "/ping", "/stats", "/start", "/start verify", "/pkg@GentooZhVerifyBot vim",
	}
	for _, m := range handled {
		if predicate(context.TODO(), privMsg(m)) {
			t.Errorf("%q should reach its handler, not the auto-reply", m)
		}
	}
	autoReply := []string{"/sb", "/ban", "/warn", "/clearwarn", "/bc", "/rich", "/autodel", "/stop", "hello", "随便聊聊"}
	for _, m := range autoReply {
		if !predicate(context.TODO(), privMsg(m)) {
			t.Errorf("%q should get the auto-reply", m)
		}
	}
	// Non-private messages never match the direct-message predicate.
	if predicate(context.TODO(), telego.Update{Message: &telego.Message{Chat: telego.Chat{Type: "supergroup"}, Text: "/pkg x"}}) {
		t.Errorf("group message should not match privateNonStart")
	}
}
