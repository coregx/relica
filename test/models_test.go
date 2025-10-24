//go:build integration
// +build integration

package test

import "time"

// Message represents an email message for IrisMX use case tests.
// This model mirrors the actual IrisMX database schema.
type Message struct {
	ID          int          `db:"id"`
	MailboxID   int          `db:"mailbox_id"`
	UserID      int          `db:"user_id"`
	UID         int          `db:"uid"`
	Status      int          `db:"status"`
	Size        int          `db:"size"`
	Subject     string       `db:"subject"`
	CreatedAt   time.Time    `db:"created_at"`
	Attachments []Attachment `db:"-"` // For N+1 test, not a DB column
}

// Attachment represents an email attachment.
type Attachment struct {
	ID        int    `db:"id"`
	MessageID int    `db:"message_id"`
	Filename  string `db:"filename"`
	Size      int    `db:"size"`
}

// MessageWithStats is used for JOIN queries with aggregated data.
type MessageWithStats struct {
	Message
	AttachmentCount int `db:"attachment_count"`
}

// Post represents a blog post for multi-JOIN tests.
type Post struct {
	ID      int    `db:"id"`
	UserID  int    `db:"user_id"`
	Title   string `db:"title"`
	Content string `db:"content"`
}

// Comment represents a comment on a post.
type Comment struct {
	ID      int    `db:"id"`
	PostID  int    `db:"post_id"`
	UserID  int    `db:"user_id"`
	Content string `db:"content"`
}

// AggregateResult is used for aggregate function tests.
type AggregateResult struct {
	Total        int     `db:"total"`
	Sum          int64   `db:"sum"`
	Avg          float64 `db:"avg"`
	Min          int     `db:"min"`
	Max          int     `db:"max"`
	MailboxID    int     `db:"mailbox_id,omitempty"`
	MessageCount int     `db:"message_count,omitempty"`
}
