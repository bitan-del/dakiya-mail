package models

import (
	"time"

	"github.com/google/uuid"
)

const (
	MaxTransactionalDelaySeconds = 43_199
	MaxTransactionalTrackIDs     = 5
	MaxTransactionalTrackIDLen   = 255
)

type TransactionalRecipient struct {
	Email string `json:"email"`
	Name  string `json:"name"`
}

// TransactionalSendRequest sends one email to one recipient from a mailbox.
type TransactionalSendRequest struct {
	EmailAccountID string                 `json:"email_account_id"`
	To             TransactionalRecipient `json:"to"`
	Subject        string                 `json:"subject"`
	HTMLBody       string                 `json:"html_body"`
	TextBody       string                 `json:"text_body"`
	TemplateID     string                 `json:"template_id"`
	Substitutions  map[string]string      `json:"substitutions"`
	DelaySeconds   int                    `json:"delay_seconds"`
	TrackIDs       []string               `json:"track_ids"`
}

type TransactionalSendResponse struct {
	TaskID       uuid.UUID `json:"task_id"`
	MessageID    string    `json:"message_id"`
	AccountEmail string    `json:"account_email"`
	ScheduledAt  time.Time `json:"scheduled_at"`
	TrackIDs     []string  `json:"track_ids"`
}

// TransactionalMessage is the status of one transactional send.
type TransactionalMessage struct {
	TaskID            uuid.UUID                `json:"task_id"`
	Status            string                   `json:"status"`
	Recipient         string                   `json:"recipient"`
	MessageID         string                   `json:"message_id"`
	ProviderMessageID string                   `json:"provider_message_id,omitempty"`
	TemplateID        *uuid.UUID               `json:"template_id,omitempty"`
	TrackIDs          []string                 `json:"track_ids"`
	ScheduledAt       *time.Time               `json:"scheduled_at,omitempty"`
	CompletedAt       *time.Time               `json:"completed_at,omitempty"`
	CreatedAt         time.Time                `json:"created_at"`
	Events            []DeliverabilityEventRow `json:"events"`
}
