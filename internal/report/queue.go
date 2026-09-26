package report

import (
	"encoding/json"
	"errors"
	"io/fs"
	"sync"
)

// QueueFile is the queue's file in the user's folder.
const QueueFile = "reports/queue.json"

// MaxQueued is the most reports the queue keeps; adding one more drops
// the oldest.
const MaxQueued = 20

// Store keeps the queue's file: the user's folder (internal/save) in
// the game, a map in tests.
type Store interface {
	Read(name string) ([]byte, error)
	Write(name string, data []byte) error
}

// Queue is the reports not yet sent, oldest first, kept in QueueFile so
// they survive the game closing.
type Queue struct {
	mu    sync.Mutex
	store Store
}

// NewQueue returns the queue kept in store.
func NewQueue(store Store) *Queue { return &Queue{store: store} }

// Pending returns the queued reports, oldest first. A damaged queue file
// counts as empty.
func (q *Queue) Pending() ([]Report, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.read()
}

func (q *Queue) read() ([]Report, error) {
	data, err := q.store.Read(QueueFile)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var rs []Report
	if json.Unmarshal(data, &rs) != nil {
		return nil, nil
	}
	return rs, nil
}

func (q *Queue) write(rs []Report) error {
	if rs == nil {
		rs = []Report{}
	}
	data, err := json.Marshal(rs)
	if err != nil {
		return err
	}
	return q.store.Write(QueueFile, data)
}

// Add queues r, dropping the oldest report if the queue is full.
func (q *Queue) Add(r Report) error {
	if err := r.Check(); err != nil {
		return err
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	rs, err := q.read()
	if err != nil {
		return err
	}
	rs = append(rs, r)
	if len(rs) > MaxQueued {
		rs = rs[len(rs)-MaxQueued:]
	}
	return q.write(rs)
}

// Remove takes the report with the ID out of the queue.
func (q *Queue) Remove(id string) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	rs, err := q.read()
	if err != nil {
		return err
	}
	out := rs[:0]
	for _, r := range rs {
		if r.ID != id {
			out = append(out, r)
		}
	}
	return q.write(out)
}
