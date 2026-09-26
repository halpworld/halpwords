package profile

import (
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/halpworld/halpwords/internal/save"
)

// Several learners can play on one computer, such as a shared computer
// at school (W2.5, PLAN §7 Children). Each has their own folder,
// profiles/<id>/, with everything that is theirs: settings, word memory,
// Hall of Fame, the saved adventure, their own and assigned word lists,
// and the link to their account. What the computer shares stays in the
// user's folder itself: the list of learners (learners.json), the AI
// helper's settings, the reports queue and crash reports.

// The files and folders of the learners.
const (
	learnersFile = "learners.json"
	// ProfilesDir is the folder that holds each learner's folder.
	ProfilesDir = "profiles"
)

// Lockouts of a learner's folder: wrong tries in a row before it locks,
// and for how long.
const (
	MaxTries = 5
	LockFor  = 15 * time.Minute
)

// shared reports whether a file in the user's folder is the computer's,
// not a learner's: it stays put when the first learner's files move into
// their folder.
func shared(name string) bool {
	switch {
	case name == learnersFile, name == learnersFile+".bad",
		strings.HasPrefix(name, ProfilesDir+"/"),
		name == "ai.json", name == "ai-spend.json", strings.HasPrefix(name, "ai/"),
		name == "crash.txt", strings.HasPrefix(name, "reports/"):
		return true
	}
	return false
}

// Learner is one learner who plays on this computer.
type Learner struct {
	// ID names their folder, profiles/<ID>.
	ID string
	// Name is what the Switch learner screen calls them: their name on
	// the website once linked, or the name typed for them.
	Name string
	// LearnerID is their learner ID on the website, once linked.
	LearnerID string `json:",omitempty"`
	// Lock, when set, is the sign-in that opens their folder: the same
	// login card or pictures they signed in with at school.
	Lock *Lock `json:",omitempty"`
	// Tries counts wrong tries at the lock in a row; LockedUntil is when
	// too many of them stop locking it.
	Tries       int       `json:",omitempty"`
	LockedUntil time.Time `json:",omitempty"`
	LastUsed    time.Time `json:",omitempty"`
}

// Folder is the learner's folder.
func (l *Learner) Folder() save.Folder { return save.Folder(ProfilesDir + "/" + l.ID) }

// Lock is what opens a learner's folder, kept as a salted, slow hash: a
// login card's code, or the 3 pictures they tap out of Grid.
type Lock struct {
	Kind string // "card" or "pictures"
	// Grid are the 9 pictures to show, for "pictures": the server's,
	// which are the same every time for a child.
	Grid []int `json:",omitempty"`
	// ClassName is the class's name, to remind the child.
	ClassName string `json:",omitempty"`
	Salt      string
	Hash      string
}

// Lock kinds.
const (
	LockCard     = "card"
	LockPictures = "pictures"
)

// lockRounds is how many rounds of PBKDF2 a lock's hash takes: slow
// enough to make guessing a card offline slow, quick enough for an old
// school computer and the web game.
const lockRounds = 20000

// NewLock returns a lock opened by secret, a login card's code as
// link.NormCode gives it, or the pictures as PicturesSecret gives them.
func NewLock(kind, secret string) (*Lock, error) {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return nil, err
	}
	l := &Lock{Kind: kind, Salt: hex.EncodeToString(salt)}
	h, err := l.hash(secret)
	if err != nil {
		return nil, err
	}
	l.Hash = h
	return l, nil
}

func (l *Lock) hash(secret string) (string, error) {
	salt, err := hex.DecodeString(l.Salt)
	if err != nil {
		return "", err
	}
	key, err := pbkdf2.Key(sha256.New, l.Kind+":"+secret, salt, lockRounds, 32)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(key), nil
}

// Opens reports whether secret opens the lock.
func (l *Lock) Opens(secret string) bool {
	h, err := l.hash(secret)
	return err == nil && subtle.ConstantTimeCompare([]byte(h), []byte(l.Hash)) == 1
}

// PicturesSecret is the secret of a picture password: the pictures, in
// the order tapped.
func PicturesSecret(picks []int) string {
	s := make([]string, len(picks))
	for i, p := range picks {
		s[i] = strconv.Itoa(p)
	}
	return strings.Join(s, ",")
}

// Learners are the learners who play on this computer, in the user's
// folder's learners.json.
type Learners struct {
	List []*Learner
	// Current is the ID of the learner playing now.
	Current string
	// Migrated is set once the files of the game from before W2.5 moved
	// into the first learner's folder.
	Migrated bool `json:",omitempty"`

	root save.Folder
	now  func() time.Time
}

// ErrLocked is a learner's folder locked after too many wrong tries.
var ErrLocked = errors.New("too many wrong tries: wait a few minutes")

// OpenLearners reads the list of learners from root (save.Root). The
// first time, it makes the first learner and moves the files of the
// game from before (one player per computer) into their folder. A
// damaged list is rebuilt from the folders there are.
func OpenLearners(root save.Folder) (*Learners, error) {
	ls := &Learners{root: root, now: time.Now}
	data, err := root.Read(learnersFile)
	switch {
	case err == nil:
		if err := json.Unmarshal(data, ls); err != nil {
			root.Write(learnersFile+".bad", data)
			ls.List, ls.Current = nil, ""
			if err := ls.rebuild(); err != nil {
				return nil, err
			}
		}
	case errors.Is(err, fs.ErrNotExist):
		if err := ls.rebuild(); err != nil {
			return nil, err
		}
	default:
		return nil, err
	}
	ls.List = slices.DeleteFunc(ls.List, func(l *Learner) bool { return l == nil || !validID(l.ID) })
	if !ls.Migrated {
		if err := ls.migrate(); err != nil {
			return nil, err
		}
	}
	if ls.Find(ls.Current) == nil {
		if len(ls.List) == 0 {
			if _, err := ls.Add(""); err != nil {
				return nil, err
			}
		}
		ls.Current = ls.List[0].ID
	}
	return ls, ls.Save()
}

// rebuild makes the list from the learners' folders.
func (ls *Learners) rebuild() error {
	names, err := ls.root.All()
	if err != nil {
		return err
	}
	seen := map[string]bool{}
	for _, n := range names {
		rest, ok := strings.CutPrefix(n, ProfilesDir+"/")
		id, _, found := strings.Cut(rest, "/")
		if !ok || !found || seen[id] || !validID(id) {
			continue
		}
		seen[id] = true
		ls.List = append(ls.List, &Learner{ID: id, Name: ls.nextName()})
	}
	return nil
}

// migrate moves the files of a game from before W2.5 into the first
// learner's folder: copied first, then the list saved, then the old
// files removed, so a crash part way loses nothing.
func (ls *Learners) migrate() error {
	names, err := ls.root.All()
	if err != nil {
		return err
	}
	var old []string
	for _, n := range names {
		if !shared(n) {
			old = append(old, n)
		}
	}
	if len(old) > 0 {
		first := ls.first()
		if first == nil {
			if first, err = ls.Add(""); err != nil {
				return err
			}
		}
		dst := first.Folder()
		for _, n := range old {
			data, err := ls.root.Read(n)
			if err != nil {
				return err
			}
			write := dst.Write
			if n == "link.json" {
				write = dst.WritePrivate // the tokens
			}
			if err := write(n, data); err != nil {
				return err
			}
		}
		if first.Name == "" || strings.HasPrefix(first.Name, "Player ") {
			if name := fameName(dst); name != "" {
				first.Name = name
			}
		}
		ls.Current = first.ID
	}
	ls.Migrated = true
	if err := ls.Save(); err != nil {
		return err
	}
	for _, n := range old {
		ls.root.Remove(n)
	}
	return nil
}

// first is the learner used longest ago, or nil.
func (ls *Learners) first() *Learner {
	if len(ls.List) == 0 {
		return nil
	}
	return ls.List[0]
}

// fameName is the name last put in the Hall of Fame in folder f.
func fameName(f save.Folder) string {
	data, err := f.Read(fameFile)
	if err != nil {
		return ""
	}
	var fame struct{ Name string }
	json.Unmarshal(data, &fame)
	return strings.TrimSpace(fame.Name)
}

// Save writes the list.
func (ls *Learners) Save() error {
	data, err := json.MarshalIndent(ls, "", "  ")
	if err != nil {
		return err
	}
	return ls.root.Write(learnersFile, data)
}

// Find returns the learner with the ID, or nil.
func (ls *Learners) Find(id string) *Learner {
	for _, l := range ls.List {
		if l.ID == id {
			return l
		}
	}
	return nil
}

// CurrentLearner is the learner playing now.
func (ls *Learners) CurrentLearner() *Learner { return ls.Find(ls.Current) }

// nextName is a name for a new learner: "Player 2" and so on.
func (ls *Learners) nextName() string {
	for n := len(ls.List) + 1; ; n++ {
		name := "Player " + strconv.Itoa(n)
		if !slices.ContainsFunc(ls.List, func(l *Learner) bool { return l.Name == name }) {
			return name
		}
	}
}

// Add adds a learner with an empty folder, named name or "Player N".
// It doesn't switch to them, or save the list.
func (ls *Learners) Add(name string) (*Learner, error) {
	b := make([]byte, 5)
	if _, err := rand.Read(b); err != nil {
		return nil, err
	}
	id := strings.ToLower(fmt.Sprintf("%x", b))
	if name = strings.TrimSpace(name); name == "" {
		name = ls.nextName()
	}
	l := &Learner{ID: id, Name: name}
	ls.List = append(ls.List, l)
	return l, nil
}

// Remove deletes the learner's folder, with everything in it, and takes
// them off the list. Removing the current learner leaves Current empty
// until the game switches to another. It doesn't save the list.
func (ls *Learners) Remove(id string) error {
	l := ls.Find(id)
	if l == nil {
		return nil
	}
	if err := l.Folder().RemoveAll(); err != nil {
		return err
	}
	ls.List = slices.DeleteFunc(ls.List, func(o *Learner) bool { return o.ID == id })
	if ls.Current == id {
		ls.Current = ""
	}
	return nil
}

// Use makes the learner current, and their folder the one the save
// package uses, and saves the list.
func (ls *Learners) Use(id string) error {
	l := ls.Find(id)
	if l == nil {
		return fmt.Errorf("profile: no learner %q", id)
	}
	ls.Current = id
	l.LastUsed = ls.now()
	save.Use(l.Folder())
	return ls.Save()
}

// Unlock tries secret on the learner's lock: it counts wrong tries, and
// after MaxTries in a row the lock stays shut for LockFor. A learner
// without a lock opens without one. It saves the list.
func (ls *Learners) Unlock(id, secret string) (bool, error) {
	l := ls.Find(id)
	if l == nil {
		return false, fmt.Errorf("profile: no learner %q", id)
	}
	if l.Lock == nil {
		return true, nil
	}
	now := ls.now()
	if now.Before(l.LockedUntil) {
		return false, ErrLocked
	}
	if l.Lock.Opens(secret) {
		l.Tries, l.LockedUntil = 0, time.Time{}
		return true, ls.Save()
	}
	l.Tries++
	if l.Tries >= MaxTries {
		l.Tries, l.LockedUntil = 0, now.Add(LockFor)
		ls.Save()
		return false, ErrLocked
	}
	return false, ls.Save()
}

// validID reports whether id can name a folder: letters and digits.
func validID(id string) bool {
	if id == "" || len(id) > 32 {
		return false
	}
	for _, r := range id {
		if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9') {
			return false
		}
	}
	return true
}
