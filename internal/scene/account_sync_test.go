package scene

import (
	"errors"
	"strings"
	"testing"

	"github.com/halpworld/halpwords/internal/link"
)

// TestSyncProblemSuspended: a suspended account (halpwords-server#130)
// shows the calm grown-up line, not the error, and nothing about
// being unlinked; a good sync shows nothing.
func TestSyncProblemSuspended(t *testing.T) {
	s, _ := syncProblem(link.Status{Linked: true, Suspended: true, Err: link.ErrSuspended})
	if !strings.HasPrefix(s, "Can't sync right now. Ask your grown-up.") || strings.Contains(strings.ToLower(s), "link") {
		t.Errorf("suspended: %q", s)
	}
	if s, _ := syncProblem(link.Status{Linked: true, Offline: true, Err: errors.New("down")}); !strings.HasPrefix(s, "Can't reach the website") {
		t.Errorf("offline: %q", s)
	}
	if s, _ := syncProblem(link.Status{Linked: true}); s != "" {
		t.Errorf("fine: %q", s)
	}
}
