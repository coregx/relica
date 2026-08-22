package core

import (
	"errors"
	"strings"
	"testing"
	"time"
)

// --- test models ---

type beforeInsertUser struct {
	ID        int64     `db:"id,pk"`
	PublicID  string    `db:"public_id,autoid:usr"`
	Name      string    `db:"name"`
	CreatedAt time.Time `db:"created_at"`
}

func (u *beforeInsertUser) BeforeInsert() error {
	if u.CreatedAt.IsZero() {
		u.CreatedAt = time.Now()
	}
	return nil
}

type beforeInsertFail struct {
	ID   int64  `db:"id,pk"`
	Name string `db:"name"`
}

func (b *beforeInsertFail) BeforeInsert() error {
	return errors.New("validation failed: name required")
}

type beforeInsertSetsAutoID struct {
	ID       int64  `db:"id,pk"`
	PublicID string `db:"public_id,autoid:usr"`
}

func (b *beforeInsertSetsAutoID) BeforeInsert() error {
	if b.PublicID == "" {
		b.PublicID = "usr_custom-from-hook"
	}
	return nil
}

type noHookModel struct {
	ID   int64  `db:"id,pk"`
	Name string `db:"name"`
}

// --- tests ---

func TestBeforeInsert_Called(t *testing.T) {
	user := &beforeInsertUser{Name: "Alice"}
	mq := &ModelQuery{model: user, db: mockDBFull("postgres")}

	err := mq.callBeforeInsert()
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if user.CreatedAt.IsZero() {
		t.Error("expected false: CreatedAt should be set by BeforeInsert")
	}
}

func TestBeforeInsert_NotCalledWithoutInterface(t *testing.T) {
	model := &noHookModel{Name: "Bob"}
	mq := &ModelQuery{model: model, db: mockDBFull("postgres")}

	err := mq.callBeforeInsert()
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestBeforeInsert_ErrorAborts(t *testing.T) {
	model := &beforeInsertFail{Name: ""}
	mq := &ModelQuery{model: model, db: mockDBFull("postgres")}

	err := mq.callBeforeInsert()
	if err == nil {
		t.Error("expected error")
	}
	if !strings.Contains(err.Error(), "validation failed") {
		t.Errorf("%q does not contain %q", err.Error(), "validation failed")
	}
}

func TestBeforeInsert_SetsAutoID_NotOverwritten(t *testing.T) {
	model := &beforeInsertSetsAutoID{}
	mq := &ModelQuery{model: model, db: mockDBFull("postgres")}

	// BeforeInsert sets PublicID
	err := mq.callBeforeInsert()
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if model.PublicID != "usr_custom-from-hook" {
		t.Errorf("got %v, want %v", model.PublicID, "usr_custom-from-hook")
	}

	// populateAutoIDFields should NOT overwrite
	mq.populateAutoIDFields()
	if model.PublicID != "usr_custom-from-hook" {
		t.Errorf("autoid should not overwrite value set by BeforeInsert: got %v", model.PublicID)
	}
}

func TestBeforeInsert_ThenAutoID(t *testing.T) {
	user := &beforeInsertUser{Name: "Alice"}
	mq := &ModelQuery{model: user, db: mockDBFull("postgres")}

	// Full Insert() flow: BeforeInsert → populateAutoIDFields
	err := mq.callBeforeInsert()
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if user.CreatedAt.IsZero() {
		t.Error("expected false: CreatedAt should be set")
	}

	mq.populateAutoIDFields()
	if !strings.HasPrefix(user.PublicID, "usr_") {
		t.Errorf("expected usr_ prefix, got %s", user.PublicID)
	}
}

func TestBeforeInsert_InTransaction(t *testing.T) {
	user := &beforeInsertUser{Name: "Alice"}
	mq := &ModelQuery{model: user, db: mockDBFull("postgres")}

	err := mq.callBeforeInsert()
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if user.CreatedAt.IsZero() {
		t.Error("expected false: CreatedAt should be set")
	}
}
