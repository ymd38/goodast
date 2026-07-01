package site

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ymd38/goodast/api/internal/db"
)

func TestToDomain(t *testing.T) {
	id := uuid.New()
	now := time.Now()
	base := db.Site{
		ID:                pgtype.UUID{Bytes: id, Valid: true},
		Name:              "corp",
		BaseUrl:           "https://example.com",
		OwnershipVerified: true,
		CreatedAt:         pgtype.Timestamptz{Time: now, Valid: true},
	}

	t.Run("with method and token", func(t *testing.T) {
		row := base
		row.VerifyMethod = pgtype.Text{String: "file", Valid: true}
		row.VerifyToken = pgtype.Text{String: tokenA, Valid: true}

		s, err := toDomain(row)
		if err != nil {
			t.Fatalf("toDomain: %v", err)
		}
		if s.ID != id || s.Name != "corp" || !s.OwnershipVerified {
			t.Errorf("unexpected mapping: %+v", s)
		}
		if s.VerifyMethod == nil || *s.VerifyMethod != VerifyMethodFile {
			t.Errorf("verify method = %v", s.VerifyMethod)
		}
		if s.VerifyToken == nil || s.VerifyToken.String() != tokenA {
			t.Errorf("verify token = %v", s.VerifyToken)
		}
	})

	t.Run("local site without method/token", func(t *testing.T) {
		s, err := toDomain(base)
		if err != nil {
			t.Fatalf("toDomain: %v", err)
		}
		if s.VerifyMethod != nil || s.VerifyToken != nil {
			t.Errorf("expected nil method/token, got %v / %v", s.VerifyMethod, s.VerifyToken)
		}
	})

	t.Run("invalid stored method errors", func(t *testing.T) {
		row := base
		row.VerifyMethod = pgtype.Text{String: "carrier-pigeon", Valid: true}
		if _, err := toDomain(row); err == nil {
			t.Fatal("expected error for invalid method")
		}
	})

	t.Run("invalid stored token errors", func(t *testing.T) {
		row := base
		row.VerifyToken = pgtype.Text{String: "short", Valid: true}
		if _, err := toDomain(row); err == nil {
			t.Fatal("expected error for invalid token")
		}
	})
}

func TestTextHelpers(t *testing.T) {
	if got := methodText(nil); got.Valid {
		t.Error("methodText(nil) should be invalid (NULL)")
	}
	m := VerifyMethodDNSTXT
	if got := methodText(&m); !got.Valid || got.String != "dns-txt" {
		t.Errorf("methodText(&m) = %+v", got)
	}

	if got := tokenText(nil); got.Valid {
		t.Error("tokenText(nil) should be invalid (NULL)")
	}
	tok := mustToken(t, tokenA)
	if got := tokenText(&tok); !got.Valid || got.String != tokenA {
		t.Errorf("tokenText(&tok) = %+v", got)
	}

	id := uuid.New()
	if got := pgUUID(id); !got.Valid || uuid.UUID(got.Bytes) != id {
		t.Errorf("pgUUID(%s) = %+v", id, got)
	}
}
