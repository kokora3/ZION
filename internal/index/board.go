package index

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/kokora3/zion/internal/board"
	"github.com/kokora3/zion/internal/membership"
)

const (
	BoardIndexVersion = 1
	DefaultQueryLimit = 20
	HardQueryLimit    = 100
	MaxSearchBytes    = 512
)

var ErrInvalidBoardQuery = errors.New("invalid board query")

type BoardEntry struct {
	PostID               string                    `json:"post_id"`
	Kind                 board.EventKind           `json:"kind"`
	AuthorIdentity       string                    `json:"author_identity"`
	AuthorKeyID          string                    `json:"author_key_id"`
	CreatedAt            int64                     `json:"created_at"`
	ParentPost           string                    `json:"parent_post,omitempty"`
	ContentObject        string                    `json:"content_object"`
	References           []board.Reference         `json:"references"`
	UnresolvedReferences []board.Reference         `json:"unresolved_references,omitempty"`
	ContentPresent       bool                      `json:"content_present"`
	SignatureStatus      string                    `json:"signature_status"`
	AuthorizationStatus  board.AuthorizationStatus `json:"authorization_status"`
	CurrentMembership    membership.Status         `json:"author_current_membership"`
	LocalVisibility      string                    `json:"local_visibility"`
	ParentPresent        bool                      `json:"parent_present"`
	Title                string                    `json:"title,omitempty"`
	Body                 string                    `json:"body,omitempty"`
}

type boardDisk struct {
	Version int                   `json:"version"`
	Entries map[string]BoardEntry `json:"entries"`
	Hidden  map[string]bool       `json:"hidden"`
}

type BoardIndex struct {
	mu      sync.RWMutex
	path    string
	entries map[string]BoardEntry
	hidden  map[string]bool
}

func OpenBoard(path string) (*BoardIndex, error) {
	if path == "" {
		return nil, fmt.Errorf("board index path is empty")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	result := &BoardIndex{path: abs, entries: map[string]BoardEntry{}, hidden: map[string]bool{}}
	data, err := os.ReadFile(abs)
	if errors.Is(err, os.ErrNotExist) {
		return result, nil
	}
	if err != nil || len(data) > 8<<20 {
		return result, nil
	}
	var disk boardDisk
	if json.Unmarshal(data, &disk) != nil || disk.Version != BoardIndexVersion || disk.Entries == nil || disk.Hidden == nil {
		return result, nil
	}
	result.entries, result.hidden = disk.Entries, disk.Hidden
	result.normalizeLocked()
	return result, nil
}

func (i *BoardIndex) Replace(entries []BoardEntry) error {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.entries = make(map[string]BoardEntry, len(entries))
	for _, entry := range entries {
		if entry.PostID != "" {
			i.entries[entry.PostID] = cloneEntry(entry)
		}
	}
	i.normalizeLocked()
	return i.saveLocked()
}

func (i *BoardIndex) Add(entry BoardEntry) (bool, error) {
	i.mu.Lock()
	defer i.mu.Unlock()
	if _, exists := i.entries[entry.PostID]; exists {
		return false, nil
	}
	i.entries[entry.PostID] = cloneEntry(entry)
	i.normalizeLocked()
	return true, i.saveLocked()
}

func (i *BoardIndex) UpdateContent(postID string, content board.Content) error {
	i.mu.Lock()
	defer i.mu.Unlock()
	entry, exists := i.entries[postID]
	if !exists {
		return os.ErrNotExist
	}
	entry.ContentPresent, entry.Title, entry.Body = true, content.Title, content.Body
	i.entries[postID] = entry
	return i.saveLocked()
}

func (i *BoardIndex) Get(postID string) (BoardEntry, bool) {
	i.mu.RLock()
	defer i.mu.RUnlock()
	entry, ok := i.entries[postID]
	if !ok {
		return BoardEntry{}, false
	}
	return i.visibleLocked(entry), true
}

func (i *BoardIndex) Feed(offset, limit int, includeHidden bool) ([]BoardEntry, error) {
	if offset < 0 || limit < 1 || limit > HardQueryLimit {
		return nil, ErrInvalidBoardQuery
	}
	i.mu.RLock()
	defer i.mu.RUnlock()
	items := make([]BoardEntry, 0, len(i.entries))
	for _, entry := range i.entries {
		if entry.Kind == board.KindReply || (!includeHidden && i.hidden[entry.PostID]) {
			continue
		}
		items = append(items, i.visibleLocked(entry))
	}
	sort.Slice(items, func(a, b int) bool {
		if items[a].CreatedAt != items[b].CreatedAt {
			return items[a].CreatedAt > items[b].CreatedAt
		}
		return items[a].PostID > items[b].PostID
	})
	return page(items, offset, limit), nil
}

func (i *BoardIndex) Replies(parent string, offset, limit int, includeHidden bool) ([]BoardEntry, error) {
	if offset < 0 || limit < 1 || limit > HardQueryLimit {
		return nil, ErrInvalidBoardQuery
	}
	i.mu.RLock()
	defer i.mu.RUnlock()
	items := []BoardEntry{}
	for _, entry := range i.entries {
		if entry.Kind == board.KindReply && entry.ParentPost == parent && (includeHidden || !i.hidden[entry.PostID]) {
			items = append(items, i.visibleLocked(entry))
		}
	}
	sort.Slice(items, func(a, b int) bool {
		if items[a].CreatedAt != items[b].CreatedAt {
			return items[a].CreatedAt < items[b].CreatedAt
		}
		return items[a].PostID < items[b].PostID
	})
	return page(items, offset, limit), nil
}

func (i *BoardIndex) Search(query string, offset, limit int, includeHidden bool) ([]BoardEntry, error) {
	if query == "" || !utf8.ValidString(query) || len([]byte(query)) > MaxSearchBytes || offset < 0 || limit < 1 || limit > HardQueryLimit {
		return nil, ErrInvalidBoardQuery
	}
	want := strings.ToLower(query)
	i.mu.RLock()
	defer i.mu.RUnlock()
	items := []BoardEntry{}
	for _, entry := range i.entries {
		if !includeHidden && i.hidden[entry.PostID] {
			continue
		}
		haystack := strings.ToLower(entry.Title + "\n" + entry.Body + "\n" + entry.AuthorIdentity)
		for _, reference := range entry.References {
			haystack += "\n" + strings.ToLower(reference.Relation+" "+reference.TargetKind+" "+reference.TargetID)
		}
		if strings.Contains(haystack, want) {
			items = append(items, i.visibleLocked(entry))
		}
	}
	sort.Slice(items, func(a, b int) bool {
		if items[a].CreatedAt != items[b].CreatedAt {
			return items[a].CreatedAt > items[b].CreatedAt
		}
		return items[a].PostID > items[b].PostID
	})
	return page(items, offset, limit), nil
}

func (i *BoardIndex) Hide(postID string, hidden bool) error {
	i.mu.Lock()
	defer i.mu.Unlock()
	if _, exists := i.entries[postID]; !exists {
		return os.ErrNotExist
	}
	if hidden {
		i.hidden[postID] = true
	} else {
		delete(i.hidden, postID)
	}
	return i.saveLocked()
}

func (i *BoardIndex) Inventory(after string, limit int) []string {
	i.mu.RLock()
	defer i.mu.RUnlock()
	ids := make([]string, 0, len(i.entries))
	for id := range i.entries {
		if id > after {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	if len(ids) > limit {
		ids = ids[:limit]
	}
	return ids
}

func (i *BoardIndex) Counts() (posts, replies, hidden int) {
	i.mu.RLock()
	defer i.mu.RUnlock()
	for _, entry := range i.entries {
		if entry.Kind == board.KindReply {
			replies++
		} else {
			posts++
		}
	}
	return posts, replies, len(i.hidden)
}

func (i *BoardIndex) RefreshMembership(values map[string]membership.Status) {
	i.mu.Lock()
	defer i.mu.Unlock()
	for id, entry := range i.entries {
		entry.CurrentMembership = values[entry.AuthorIdentity]
		i.entries[id] = entry
	}
}

func (i *BoardIndex) normalizeLocked() {
	for id, entry := range i.entries {
		entry.References = append([]board.Reference(nil), entry.References...)
		entry.UnresolvedReferences = append([]board.Reference(nil), entry.UnresolvedReferences...)
		entry.ParentPresent = entry.ParentPost == ""
		if entry.ParentPost != "" {
			_, entry.ParentPresent = i.entries[entry.ParentPost]
		}
		i.entries[id] = entry
	}
	for id := range i.hidden {
		if _, exists := i.entries[id]; !exists {
			delete(i.hidden, id)
		}
	}
}

func (i *BoardIndex) visibleLocked(entry BoardEntry) BoardEntry {
	entry = cloneEntry(entry)
	if i.hidden[entry.PostID] {
		entry.LocalVisibility = "LOCALLY_HIDDEN"
	} else {
		entry.LocalVisibility = "VISIBLE"
	}
	return entry
}

func (i *BoardIndex) saveLocked() error {
	if err := os.MkdirAll(filepath.Dir(i.path), 0o700); err != nil {
		return err
	}
	data, err := json.Marshal(boardDisk{Version: BoardIndexVersion, Entries: i.entries, Hidden: i.hidden})
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(i.path), ".board-index-*.tmp")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	_ = tmp.Chmod(0o600)
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	backup := i.path + ".previous"
	_ = os.Remove(backup)
	if err := os.Rename(i.path, backup); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.Rename(name, i.path); err != nil {
		_ = os.Rename(backup, i.path)
		return err
	}
	_ = os.Remove(backup)
	return nil
}

func cloneEntry(entry BoardEntry) BoardEntry {
	entry.References = append([]board.Reference(nil), entry.References...)
	entry.UnresolvedReferences = append([]board.Reference(nil), entry.UnresolvedReferences...)
	return entry
}

func page(items []BoardEntry, offset, limit int) []BoardEntry {
	if offset >= len(items) {
		return []BoardEntry{}
	}
	end := min(len(items), offset+limit)
	return append([]BoardEntry(nil), items[offset:end]...)
}

func (i *BoardIndex) SaveContext(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	i.mu.Lock()
	defer i.mu.Unlock()
	return i.saveLocked()
}
