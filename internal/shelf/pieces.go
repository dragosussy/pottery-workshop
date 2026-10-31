package shelf

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"slices"
	"time"

	"humidity/internal/store"
)

// Text limits.
const (
	maxLabel = 80
	maxNotes = 2000
)

// AddPieces is the POST /pieces body.
type AddPieces struct {
	Quantity       *int     `json:"quantity"`
	Form           *string  `json:"form"`
	InitialDryness *float64 `json:"initial_dryness"`
	Size           *string  `json:"size"`
	Thickness      *string  `json:"thickness"`
	PhotoID        *int64   `json:"photo_id"`
	Label          *string  `json:"label"`
	Notes          *string  `json:"notes"`
	AddedAt        *string  `json:"added_at"`
}

// lowestFree returns the lowest positive number not in used.
func lowestFree(used map[int64]bool) int64 {
	n := int64(1)
	for used[n] {
		n++
	}
	return n
}

func usedNumbers(ctx context.Context, q store.Querier) (map[int64]bool, error) {
	nums, err := store.UsedNumbers(ctx, q)
	if err != nil {
		return nil, err
	}
	used := make(map[int64]bool, len(nums))
	for _, n := range nums {
		used[n] = true
	}
	return used, nil
}

func checkEnum(v *ValidationError, field string, val *string, opts []Option, required bool) string {
	if val == nil {
		if required {
			v.add(field, CodeRequired, "required")
		}
		return ""
	}
	if !validKey(opts, *val) {
		v.add(field, CodeInvalidEnum, "not a valid option")
	}
	return *val
}

func checkText(v *ValidationError, field string, val *string, limit int) string {
	if val == nil {
		return ""
	}
	if len([]rune(*val)) > limit {
		v.add(field, CodeTooLong, "too long")
	}
	return *val
}

func (s *Service) checkPhoto(ctx context.Context, q store.Querier, v *ValidationError, field string, id int64) {
	if _, err := store.GetPhoto(ctx, q, id); errors.Is(err, store.ErrNotFound) {
		v.add(field, CodePhotoNotFound, "no photo with this id")
	}
}

// Add creates quantity pieces in one transaction, each with the lowest free
// shelf number in turn and its add observation (api-contract §3.3).
func (s *Service) Add(ctx context.Context, in AddPieces) ([]PieceView, error) {
	now := s.Now()
	v := &ValidationError{}
	qty := 1
	if in.Quantity != nil {
		qty = *in.Quantity
		if qty < 1 || qty > 20 {
			v.add("quantity", CodeOutOfRange, "must be between 1 and 20")
		}
	}
	p := store.Piece{
		Form:      checkEnum(v, "form", in.Form, forms, true),
		Size:      checkEnum(v, "size", in.Size, sizes, true),
		Thickness: checkEnum(v, "thickness", in.Thickness, thicknesses, true),
		Label:     checkText(v, "label", in.Label, maxLabel),
		Notes:     checkText(v, "notes", in.Notes, maxNotes),
		PhotoID:   in.PhotoID,
		AddedAt:   now.UTC().Truncate(time.Second),
	}
	var d float64
	if in.InitialDryness == nil {
		v.add("initial_dryness", CodeRequired, "required")
	} else {
		d, _ = dryness(v, "initial_dryness", *in.InitialDryness)
	}
	if in.AddedAt != nil {
		if t, ok := parseTime(v, "added_at", *in.AddedAt, now); ok {
			p.AddedAt = t
		}
	}
	if in.PhotoID != nil {
		s.checkPhoto(ctx, s.st.DB(), v, "photo_id", *in.PhotoID)
	}
	if err := v.err(); err != nil {
		return nil, err
	}

	var created []store.Piece
	err := s.st.Tx(ctx, func(q store.Querier) error {
		used, err := usedNumbers(ctx, q)
		if err != nil {
			return err
		}
		for i := 0; i < qty; i++ {
			piece := p
			piece.Number = lowestFree(used)
			used[piece.Number] = true
			if qty > 1 && len(created) > 0 {
				piece.GroupID = ptr(created[0].ID)
			}
			id, err := store.InsertPiece(ctx, q, piece)
			if err != nil {
				return err
			}
			piece.ID = id
			if qty > 1 && i == 0 {
				piece.GroupID = ptr(id)
				if err := store.UpdatePiece(ctx, q, piece); err != nil {
					return err
				}
			}
			_, err = store.InsertObservation(ctx, q, store.Observation{
				PieceID: id, Dryness: d, ObservedAt: piece.AddedAt, Source: store.SourceAdd, CreatedAt: now,
			})
			if err != nil {
				return err
			}
			created = append(created, piece)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	s.changed([]string{ScopePieces, ScopeObservations, ScopePredictions}, pieceIDs(created))
	return s.views(ctx, created)
}

func pieceIDs(ps []store.Piece) []int64 {
	out := make([]int64, len(ps))
	for i, p := range ps {
		out[i] = p.ID
	}
	return out
}

// Get returns one piece, including one entered by mistake.
func (s *Service) Get(ctx context.Context, id int64) (PieceView, error) {
	p, err := store.GetPiece(ctx, s.st.DB(), id)
	if errors.Is(err, store.ErrNotFound) {
		return PieceView{}, errorf(ErrNotFound, "no piece %d", id)
	}
	if err != nil {
		return PieceView{}, err
	}
	return s.view(ctx, p)
}

// OnShelf returns the pieces on the shelf, ordered by number.
func (s *Service) OnShelf(ctx context.Context) ([]PieceView, error) {
	pieces, err := store.OnShelfPieces(ctx, s.st.DB())
	if err != nil {
		return nil, err
	}
	return s.views(ctx, pieces)
}

// Page is a page of a paginated listing.
type Page[T any] struct {
	Items      []T     `json:"items"`
	NextCursor *string `json:"next_cursor"`
}

// ErrBadCursor is returned for a cursor the server didn't issue.
var ErrBadCursor = errors.New("invalid cursor")

func encodeCursor(c any) *string {
	b, _ := json.Marshal(c)
	s := base64.RawURLEncoding.EncodeToString(b)
	return &s
}

func decodeCursor(s string, dst any) error {
	b, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil || json.Unmarshal(b, dst) != nil {
		return ErrBadCursor
	}
	return nil
}

// ListPieces lists ended pieces (or all pieces, on-shelf first), newest
// ended_at first, excluding mistakes.
func (s *Service) ListPieces(ctx context.Context, includeOnShelf bool, cursor string, limit int) (Page[PieceView], error) {
	var after *store.PieceCursor
	if cursor != "" {
		after = &store.PieceCursor{}
		if err := decodeCursor(cursor, after); err != nil {
			return Page[PieceView]{}, err
		}
	}
	pieces, next, err := store.PiecesPage(ctx, s.st.DB(), includeOnShelf, after, limit)
	if err != nil {
		return Page[PieceView]{}, err
	}
	items, err := s.views(ctx, pieces)
	if err != nil {
		return Page[PieceView]{}, err
	}
	page := Page[PieceView]{Items: items}
	if next != nil {
		page.NextCursor = encodeCursor(next)
	}
	return page, nil
}

// sortObs orders observations by time, then ID.
func sortObs(obs []store.Observation) {
	slices.SortFunc(obs, func(a, b store.Observation) int {
		if c := a.ObservedAt.Compare(b.ObservedAt); c != 0 {
			return c
		}
		return int(a.ID - b.ID)
	})
}
