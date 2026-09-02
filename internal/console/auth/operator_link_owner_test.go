package auth

import (
	"errors"
	"testing"
)

// The owner predicate is the only thing standing between a stranger and an operator session.
// Making IssueOperatorLink ignore it left every test in the repository passing.
func TestIssueOperatorLinkHonoursTheOwnerPredicate(t *testing.T) {
	const owner, stranger = int64(7), int64(8)
	manager, err := New(Config{
		BotToken:        "123:token",
		OperatorAllowed: func(id int64) bool { return id == owner },
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := manager.IssueOperatorLink(owner); err != nil {
		t.Fatalf("the owner was refused a link: %v", err)
	}
	if _, _, err := manager.IssueOperatorLink(stranger); !errors.Is(err, ErrOperatorNotAllowed) {
		t.Fatalf("a stranger got %v, want %v; that link is an operator session",
			err, ErrOperatorNotAllowed)
	}
}
