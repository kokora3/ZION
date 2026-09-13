package index

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kokora3/zion/internal/board"
	"github.com/kokora3/zion/internal/membership"
	"github.com/kokora3/zion/internal/protocol"
)

func indexTestID(value string) string {
	return (protocol.ObjectID{HashDigest: protocol.HashBytes([]byte(value))}).String()
}

func TestBoardIndexFeedRepliesSearchHideAndPersistence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "board-index.json")
	index, err := OpenBoard(path)
	if err != nil {
		t.Fatal(err)
	}
	parentID, replyID := indexTestID("parent"), indexTestID("reply")
	reply := BoardEntry{PostID: replyID, Kind: board.KindReply, ParentPost: parentID, CreatedAt: 2,
		AuthorIdentity: "alice", Title: "ตอบกลับ", Body: "เนื้อหาภาษาไทย", SignatureStatus: "SIGNATURE_VALID",
		AuthorizationStatus: board.CurrentlyAuthorized, CurrentMembership: membership.Active}
	if added, err := index.Add(reply); err != nil || !added {
		t.Fatalf("add orphan = %v %v", added, err)
	}
	stored, _ := index.Get(replyID)
	if stored.ParentPresent {
		t.Fatal("reply-before-parent was not indexed as orphan")
	}
	parent := BoardEntry{PostID: parentID, Kind: board.KindPost, CreatedAt: 1, AuthorIdentity: "alice",
		Title: "Decentralized ZION", Body: "ค้นหาเครือข่าย community", SignatureStatus: "SIGNATURE_VALID"}
	if added, err := index.Add(parent); err != nil || !added {
		t.Fatalf("add parent = %v %v", added, err)
	}
	stored, _ = index.Get(replyID)
	if !stored.ParentPresent {
		t.Fatal("orphan did not resolve after parent arrived")
	}
	feed, _ := index.Feed(0, DefaultQueryLimit, false)
	if len(feed) != 1 || feed[0].PostID != parentID {
		t.Fatalf("feed = %+v", feed)
	}
	replies, _ := index.Replies(parentID, 0, DefaultQueryLimit, false)
	if len(replies) != 1 || replies[0].PostID != replyID {
		t.Fatalf("replies = %+v", replies)
	}
	results, err := index.Search("ภาษาไทย", 0, DefaultQueryLimit, false)
	if err != nil || len(results) != 1 || results[0].PostID != replyID {
		t.Fatalf("unicode search = %+v %v", results, err)
	}
	if err := index.Hide(parentID, true); err != nil {
		t.Fatal(err)
	}
	hiddenEntry, found := index.Get(parentID)
	if !found || hiddenEntry.LocalVisibility != "LOCALLY_HIDDEN" {
		t.Fatalf("hidden entry visibility = %q, found=%v", hiddenEntry.LocalVisibility, found)
	}
	feed, _ = index.Feed(0, DefaultQueryLimit, false)
	if len(feed) != 0 {
		t.Fatal("hidden post remained in default feed")
	}
	reopened, err := OpenBoard(path)
	if err != nil {
		t.Fatal(err)
	}
	hidden, found := reopened.Get(parentID)
	if !found {
		t.Fatal("persisted index entry missing")
	}
	all, _ := reopened.Feed(0, DefaultQueryLimit, true)
	if len(all) != 1 || all[0].LocalVisibility != "LOCALLY_HIDDEN" || hidden.PostID != parentID {
		t.Fatalf("hidden persistence = %+v", all)
	}
	if err := reopened.Hide(parentID, false); err != nil {
		t.Fatal(err)
	}
	feed, _ = reopened.Feed(0, DefaultQueryLimit, false)
	if len(feed) != 1 {
		t.Fatal("unhide did not restore post")
	}
}

func TestBoardIndexBoundsDedupAndCorruptRecovery(t *testing.T) {
	path := filepath.Join(t.TempDir(), "board-index.json")
	index, _ := OpenBoard(path)
	entry := BoardEntry{PostID: indexTestID("dedup"), Kind: board.KindPost, CreatedAt: 1}
	if added, _ := index.Add(entry); !added {
		t.Fatal("first add not admitted")
	}
	if added, _ := index.Add(entry); added {
		t.Fatal("duplicate Board entry admitted twice")
	}
	if _, err := index.Search("", 0, 20, false); err == nil {
		t.Fatal("empty search accepted")
	}
	if _, err := index.Feed(0, HardQueryLimit+1, false); err == nil {
		t.Fatal("oversized feed limit accepted")
	}
	if err := os.WriteFile(path, []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	recovered, err := OpenBoard(path)
	if err != nil {
		t.Fatal(err)
	}
	posts, replies, hidden := recovered.Counts()
	if posts != 0 || replies != 0 || hidden != 0 {
		t.Fatal("corrupt index did not recover empty for rebuild")
	}
}
