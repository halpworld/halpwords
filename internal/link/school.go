package link

import (
	"context"
	"errors"
	"net/http"
	"strings"
)

// The ways a game is signed in (W2.5, the server's docs/api "Signing in
// at school").
const (
	WayPairing = "pairing" // a pairing code from a grown-up's website
	WayCard    = "card"    // a login card from a teacher
	WayClass   = "class"   // the class code, a name and 3 pictures
)

// Lengths of the codes, without hyphens or spaces.
const (
	PairingCodeLen = 8
	CardCodeLen    = 12
	ClassCodeLen   = 8
	// Picks is how many pictures a child taps; Grid is how many they
	// choose from.
	Picks = 3
	Grid  = 9
)

// Errors of signing in at school, for the sign-in screens.
var (
	// ErrWrongSignIn is a login card, class code or pictures the server
	// doesn't take.
	ErrWrongSignIn = errors.New("that didn't work: check and try again")
	// ErrLocked is a card, pictures or class code locked after too many
	// wrong tries.
	ErrLocked = errors.New("too many wrong tries: wait a while, or ask your teacher to unlock it")
)

// SignIn is how a learner signs in: Code (a pairing code or a login
// card), or ClassCode with LearnerID or Username and Pictures.
type SignIn struct {
	Code      string
	ClassCode string
	LearnerID string
	Username  string
	Pictures  []int
}

// NormCode is a code as the server reads it: upper case, without spaces
// or hyphens, with O read as 0 and I and L as 1 (Crockford base 32).
func NormCode(code string) string {
	return strings.NewReplacer("-", "", " ", "", "O", "0", "I", "1", "L", "1").Replace(strings.ToUpper(strings.TrimSpace(code)))
}

// Way is how the sign-in is made: WayPairing, WayCard or WayClass.
func (s SignIn) Way() string {
	switch {
	case s.ClassCode != "":
		return WayClass
	case len(NormCode(s.Code)) == CardCodeLen:
		return WayCard
	}
	return WayPairing
}

// body is the request to POST /api/v1/link, or ErrBadCode (a pairing
// code) or ErrWrongSignIn when it can't be right.
func (s SignIn) body() (map[string]any, error) {
	if s.ClassCode != "" {
		if len(NormCode(s.ClassCode)) != ClassCodeLen || len(s.ClassCode) > 16 ||
			(s.LearnerID == "") == (s.Username == "") || len(s.Pictures) != Picks {
			return nil, ErrWrongSignIn
		}
		b := map[string]any{"class_code": s.ClassCode, "pictures": s.Pictures}
		if s.LearnerID != "" {
			b["learner_id"] = s.LearnerID
		} else {
			b["username"] = strings.TrimSpace(s.Username)
		}
		return b, nil
	}
	code := strings.TrimSpace(s.Code)
	switch n := len(NormCode(code)); {
	case n == PairingCodeLen && len(code) <= 16:
	case n == CardCodeLen && len(code) <= 24:
	case n > PairingCodeLen:
		return nil, ErrWrongSignIn
	default:
		return nil, ErrBadCode
	}
	return map[string]any{"code": code}, nil
}

// explain turns the server's error answer to a sign-in into the error
// the screens explain.
func (s SignIn) explain(err error) error {
	var e *Error
	if !errors.As(err, &e) {
		return err
	}
	bad := ErrBadCode
	if s.Way() != WayPairing {
		bad = ErrWrongSignIn
	}
	switch {
	case e.Code == codeLocked:
		return ErrLocked
	case e.Code == codeInvalidCode || (e.Status == http.StatusBadRequest && e.Code == codeInvalidRequest):
		return bad
	case e.Code == codeNotLinkable || e.Status == http.StatusForbidden:
		return ErrNotLinkable
	}
	return err
}

// Class is a class, for signing in with its code, from POST
// /api/v1/link/class.
type Class struct {
	Class struct {
		Name     string `json:"name"`
		Language string `json:"language"`
	} `json:"class"`
	// NamesShown is false when the teacher turned the name list off:
	// the child types their username instead.
	NamesShown bool `json:"names_shown"`
	Learners   []struct {
		ID          string `json:"id"`
		DisplayName string `json:"display_name"`
	} `json:"learners"`
	// Pictures are the 9 pictures to show the child, by their index in
	// pkg/proc's picture set, when a child was named.
	Pictures []int `json:"pictures,omitempty"`
}

// FindClass asks the server for the class with the code, and, when
// learnerID or username names a child, the 9 pictures to show them. It
// waits for the answer, so call it from a goroutine. A game that is
// already linked can't sign in again.
func (c *Client) FindClass(ctx context.Context, code, learnerID, username string) (*Class, error) {
	if c.Linked() {
		return nil, ErrLinked
	}
	if len(NormCode(code)) != ClassCodeLen || len(code) > 16 {
		return nil, ErrWrongSignIn
	}
	body := map[string]string{"class_code": code}
	switch {
	case learnerID != "":
		body["learner_id"] = learnerID
	case strings.TrimSpace(username) != "":
		body["username"] = strings.TrimSpace(username)
	}
	var cl Class
	_, _, err := c.call(ctx, http.MethodPost, "/api/v1/link/class", "", nil, body, &cl)
	if err = (SignIn{ClassCode: code}).explain(err); err != nil {
		return nil, err
	}
	if (learnerID != "" || username != "") && len(cl.Pictures) != Grid {
		return nil, errors.New("link: the server sent no pictures")
	}
	return &cl, nil
}

// Way is how the linked game signed in: WayPairing, WayCard, WayClass or
// WaySSO; "" when it isn't linked, or was linked before games said.
func (c *Client) Way() string {
	if c == nil {
		return ""
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.st.Way
}

// KeepOnSignOut reports whether the learner's progress may stay on this
// computer after they sign out: what the server's "me" says (a school
// decides for its classes), or, before the game has heard, true for a
// game a grown-up linked at home and false for one signed in at school.
func (c *Client) KeepOnSignOut() bool {
	if c == nil {
		return true
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if m := c.st.Me; m != nil && m.KeepOnSignOut != nil {
		return *m.KeepOnSignOut
	}
	return c.st.Way != WayCard && c.st.Way != WayClass && c.st.Way != WaySSO
}
