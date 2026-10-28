package models

import (
	"time"

	"github.com/google/uuid"
)

// Segment is a saved contacts filter, evaluated fresh each time it is used.
type Segment struct {
	ID             uuid.UUID      `json:"id"`
	OrganizationID uuid.UUID      `json:"organization_id"`
	Name           string         `json:"name"`
	Description    string         `json:"description"`
	Filter         SearchContacts `json:"filter"`
	CreatedBy      *uuid.UUID     `json:"created_by,omitempty"`
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
	// ContactCount is how many contacts match right now (list and get only).
	ContactCount *int64 `json:"contact_count,omitempty"`
}

type SaveSegmentRequest struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Filter      SearchContacts `json:"filter"`
}

type EnrollSegmentRequest struct {
	CampaignID string `json:"campaign_id"`
}

type EnrollSegmentResult struct {
	Matched  int  `json:"matched"`
	Enrolled int  `json:"enrolled"`
	Capped   bool `json:"capped"`
}

// MaxSegmentEnroll bounds one enroll call; larger segments enroll in repeat calls.
const MaxSegmentEnroll = 100_000
