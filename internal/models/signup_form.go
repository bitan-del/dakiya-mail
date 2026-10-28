package models

import (
	"time"

	"github.com/google/uuid"
)

// SignupFormFields are the optional fields a form may show besides email.
var SignupFormFields = []string{"first_name", "last_name", "company", "phone"}

type SignupForm struct {
	ID             uuid.UUID   `json:"id"`
	OrganizationID uuid.UUID   `json:"organization_id"`
	CreatedBy      *uuid.UUID  `json:"created_by,omitempty"`
	Name           string      `json:"name"`
	Slug           string      `json:"slug"`
	Title          string      `json:"title"`
	Description    string      `json:"description"`
	Fields         []string    `json:"fields"`
	CategoryIDs    []uuid.UUID `json:"category_ids"`
	DoubleOptIn    bool        `json:"double_opt_in"`
	EmailAccountID *uuid.UUID  `json:"email_account_id,omitempty"`
	SuccessMessage string      `json:"success_message"`
	RedirectURL    string      `json:"redirect_url"`
	Enabled        bool        `json:"enabled"`
	Submissions    int         `json:"submissions"`
	Confirmations  int         `json:"confirmations"`
	CreatedAt      time.Time   `json:"created_at"`
	UpdatedAt      time.Time   `json:"updated_at"`
	// OwnerUserID is who new contacts are created under: the creator, or the org owner.
	OwnerUserID uuid.UUID `json:"-"`
}

type SaveSignupFormRequest struct {
	Name           string   `json:"name"`
	Slug           string   `json:"slug"`
	Title          string   `json:"title"`
	Description    string   `json:"description"`
	Fields         []string `json:"fields"`
	CategoryIDs    []string `json:"category_ids"`
	DoubleOptIn    bool     `json:"double_opt_in"`
	EmailAccountID string   `json:"email_account_id"`
	SuccessMessage string   `json:"success_message"`
	RedirectURL    string   `json:"redirect_url"`
	Enabled        *bool    `json:"enabled"`
}

// SignupSubmission is what a visitor posts to a form.
type SignupSubmission struct {
	Email     string `json:"email" form:"email"`
	FirstName string `json:"first_name" form:"first_name"`
	LastName  string `json:"last_name" form:"last_name"`
	Company   string `json:"company" form:"company"`
	Phone     string `json:"phone" form:"phone"`
	// Honeypot is a hidden field; anything in it marks the post as a bot.
	Honeypot string `json:"website_url" form:"website_url"`
}
