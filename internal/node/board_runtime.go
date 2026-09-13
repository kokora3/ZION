package node

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/kokora3/zion/internal/board"
	"github.com/kokora3/zion/internal/index"
	"github.com/kokora3/zion/internal/objects"
	"github.com/kokora3/zion/internal/protocol"
)

func (r *Runtime) SubmitBoardEvent(ctx context.Context, raw []byte) (any, error) {
	entry, added, err := r.admitBoardEvent(ctx, raw, true)
	if err != nil {
		return nil, err
	}
	if added {
		r.mu.RLock()
		service := r.board
		r.mu.RUnlock()
		if service != nil {
			_ = service.Announce(ctx, raw)
		}
	}
	return entry, nil
}

func (r *Runtime) AdmitRemoteBoardEvent(ctx context.Context, raw []byte) error {
	_, _, err := r.admitBoardEvent(ctx, raw, false)
	return err
}

func (r *Runtime) admitBoardEvent(ctx context.Context, raw []byte, local bool) (index.BoardEntry, bool, error) {
	if ctx == nil || len(raw) > objects.MaxObjectBytes {
		return index.BoardEntry{}, false, board.ErrInvalidEvent
	}
	object, err := objects.Decode(raw)
	if err != nil {
		return index.BoardEntry{}, false, board.ErrInvalidEvent
	}
	event, err := board.DecodeEventObject(object)
	if err != nil || event.Body.NetworkID != r.cfg.NetworkID {
		return index.BoardEntry{}, false, board.ErrInvalidEvent
	}
	postID, err := board.PostID(object)
	if err != nil {
		return index.BoardEntry{}, false, err
	}
	r.mu.RLock()
	authorization, currentMembership, err := board.VerifyAndClassify(event, r.state)
	r.mu.RUnlock()
	if err != nil {
		return index.BoardEntry{}, false, err
	}
	if local && authorization != board.CurrentlyAuthorized {
		return index.BoardEntry{}, false, board.ErrPublicationDenied
	}
	if local && event.Body.Kind == board.KindReply {
		if _, found := r.boardIndex.Get(event.Body.ParentPost); !found {
			return index.BoardEntry{}, false, board.ErrParentNotFound
		}
	}
	contentID, _ := protocol.ParseObjectID(event.Body.ContentObject)
	content, contentPresent, contentErr := r.localBoardContent(ctx, contentID)
	if local && contentErr != nil {
		if errors.Is(contentErr, objects.ErrObjectNotFound) {
			return index.BoardEntry{}, false, board.ErrBoardContentMissing
		}
		return index.BoardEntry{}, false, contentErr
	}
	if _, _, err := r.objects.Put(ctx, object); err != nil {
		return index.BoardEntry{}, false, err
	}
	entry := index.BoardEntry{PostID: postID.String(), Kind: event.Body.Kind, AuthorIdentity: event.Body.AuthorIdentity.String(),
		AuthorKeyID: event.Body.AuthorKeyID.String(), CreatedAt: int64(event.Body.CreatedAt), ParentPost: event.Body.ParentPost,
		ContentObject: event.Body.ContentObject, References: append([]board.Reference(nil), event.Body.References...),
		ContentPresent: contentPresent, SignatureStatus: "SIGNATURE_VALID", AuthorizationStatus: authorization,
		CurrentMembership: currentMembership, LocalVisibility: "VISIBLE", Title: content.Title, Body: content.Body}
	added, err := r.boardIndex.Add(entry)
	if err != nil {
		return index.BoardEntry{}, false, err
	}
	if !added {
		existing, _ := r.boardIndex.Get(postID.String())
		return r.decorateBoardEntry(ctx, existing), false, nil
	}
	if !local && !contentPresent {
		if fetched, fetchErr := r.FetchObject(ctx, contentID); fetchErr == nil {
			if valid, decodeErr := board.DecodeContentObject(fetched); decodeErr == nil {
				_ = r.boardIndex.UpdateContent(postID.String(), valid)
				entry.ContentPresent, entry.Title, entry.Body = true, valid.Title, valid.Body
			}
		}
	}
	return r.decorateBoardEntry(ctx, entry), true, nil
}

func (r *Runtime) localBoardContent(ctx context.Context, id protocol.ObjectID) (board.Content, bool, error) {
	object, err := r.objects.Get(ctx, id)
	if err != nil {
		return board.Content{}, false, err
	}
	content, err := board.DecodeContentObject(object)
	if err != nil {
		return board.Content{}, false, err
	}
	return content, true, nil
}

func (r *Runtime) BoardInventory(ctx context.Context, after string, limit int) ([][]byte, string, bool, error) {
	ids := r.boardIndex.Inventory(after, limit+1)
	more := len(ids) > limit
	if more {
		ids = ids[:limit]
	}
	events := make([][]byte, 0, len(ids))
	for _, value := range ids {
		id, err := protocol.ParseObjectID(value)
		if err != nil {
			continue
		}
		object, err := r.objects.Get(ctx, id)
		if err != nil {
			continue
		}
		raw, err := object.CanonicalBytes()
		if err == nil {
			events = append(events, raw)
		}
	}
	next := ""
	if len(ids) > 0 {
		next = ids[len(ids)-1]
	}
	return events, next, more, ctx.Err()
}

func (r *Runtime) rebuildBoardIndex(ctx context.Context) error {
	ids, err := r.objects.ListObjectIDs(ctx, 100000)
	if err != nil {
		return err
	}
	entries := []index.BoardEntry{}
	for _, id := range ids {
		object, err := r.objects.Get(ctx, id)
		if err != nil {
			continue
		}
		event, err := board.DecodeEventObject(object)
		if err != nil || event.Body.NetworkID != r.cfg.NetworkID {
			continue
		}
		r.mu.RLock()
		authorization, currentMembership, verifyErr := board.VerifyAndClassify(event, r.state)
		r.mu.RUnlock()
		if verifyErr != nil {
			continue
		}
		contentID, _ := protocol.ParseObjectID(event.Body.ContentObject)
		content, present, _ := r.localBoardContent(ctx, contentID)
		entries = append(entries, index.BoardEntry{PostID: id.String(), Kind: event.Body.Kind,
			AuthorIdentity: event.Body.AuthorIdentity.String(), AuthorKeyID: event.Body.AuthorKeyID.String(),
			CreatedAt: int64(event.Body.CreatedAt), ParentPost: event.Body.ParentPost, ContentObject: event.Body.ContentObject,
			References: append([]board.Reference(nil), event.Body.References...), ContentPresent: present,
			SignatureStatus: "SIGNATURE_VALID", AuthorizationStatus: authorization, CurrentMembership: currentMembership,
			LocalVisibility: "VISIBLE", Title: content.Title, Body: content.Body})
	}
	return r.boardIndex.Replace(entries)
}

func (r *Runtime) BoardFeed(offset, limit int) (any, error) {
	values, err := r.boardIndex.Feed(offset, limit, false)
	return r.decorateBoardEntries(context.Background(), values), err
}

func (r *Runtime) BoardPost(value string) (any, bool, error) {
	id, err := protocol.ParseObjectID(value)
	if err != nil {
		return nil, false, err
	}
	entry, ok := r.boardIndex.Get(id.String())
	if !ok {
		return nil, false, nil
	}
	return r.decorateBoardEntry(context.Background(), entry), true, nil
}

func (r *Runtime) BoardReplies(value string, offset, limit int) (any, error) {
	id, err := protocol.ParseObjectID(value)
	if err != nil {
		return nil, err
	}
	values, err := r.boardIndex.Replies(id.String(), offset, limit, false)
	return r.decorateBoardEntries(context.Background(), values), err
}

func (r *Runtime) BoardSearch(query string, offset, limit int) (any, error) {
	values, err := r.boardIndex.Search(query, offset, limit, false)
	return r.decorateBoardEntries(context.Background(), values), err
}

func (r *Runtime) SetBoardHidden(value string, hidden bool) error {
	id, err := protocol.ParseObjectID(value)
	if err != nil {
		return err
	}
	if err := r.boardIndex.Hide(id.String(), hidden); errors.Is(err, os.ErrNotExist) {
		return objects.ErrObjectNotFound
	} else {
		return err
	}
}

func (r *Runtime) decorateBoardEntries(ctx context.Context, entries []index.BoardEntry) []index.BoardEntry {
	result := make([]index.BoardEntry, len(entries))
	for position, entry := range entries {
		result[position] = r.decorateBoardEntry(ctx, entry)
	}
	return result
}

func (r *Runtime) decorateBoardEntry(ctx context.Context, entry index.BoardEntry) index.BoardEntry {
	id, err := protocol.ParseObjectID(entry.PostID)
	if err != nil {
		return entry
	}
	object, err := r.objects.Get(ctx, id)
	if err != nil {
		return entry
	}
	event, err := board.DecodeEventObject(object)
	if err != nil {
		return entry
	}
	r.mu.RLock()
	authorization, status, err := board.VerifyAndClassify(event, r.state)
	r.mu.RUnlock()
	if err == nil {
		entry.AuthorizationStatus, entry.CurrentMembership = authorization, status
	}
	return entry
}

func (r *Runtime) SyncBoard(ctx context.Context) error {
	r.mu.RLock()
	service := r.board
	r.mu.RUnlock()
	if service == nil {
		return fmt.Errorf("board service is not running")
	}
	return service.SyncNow(ctx)
}
