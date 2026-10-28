package advanced

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/bitan-del/dakiya-mail/internal/errx"
	"github.com/bitan-del/dakiya-mail/internal/models"
	"github.com/google/uuid"
)

// sesNotification covers both SES feedback notifications ("notificationType")
// and configuration-set event publishing ("eventType").
type sesNotification struct {
	NotificationType string `json:"notificationType"`
	EventType        string `json:"eventType"`
	Mail             struct {
		MessageID     string `json:"messageId"`
		CommonHeaders struct {
			MessageID string `json:"messageId"`
		} `json:"commonHeaders"`
	} `json:"mail"`
	Bounce *struct {
		BounceType        string `json:"bounceType"`
		BounceSubType     string `json:"bounceSubType"`
		FeedbackID        string `json:"feedbackId"`
		BouncedRecipients []struct {
			EmailAddress   string `json:"emailAddress"`
			Status         string `json:"status"`
			DiagnosticCode string `json:"diagnosticCode"`
		} `json:"bouncedRecipients"`
	} `json:"bounce"`
	Complaint *struct {
		FeedbackID            string `json:"feedbackId"`
		ComplaintFeedbackType string `json:"complaintFeedbackType"`
		ComplainedRecipients  []struct {
			EmailAddress string `json:"emailAddress"`
		} `json:"complainedRecipients"`
	} `json:"complaint"`
	Delivery *struct {
		Recipients   []string `json:"recipients"`
		SMTPResponse string   `json:"smtpResponse"`
	} `json:"delivery"`
	DeliveryDelay *struct {
		DelayType         string `json:"delayType"`
		ExpirationTime    string `json:"expirationTime"`
		DelayedRecipients []struct {
			EmailAddress   string `json:"emailAddress"`
			DiagnosticCode string `json:"diagnosticCode"`
		} `json:"delayedRecipients"`
	} `json:"deliveryDelay"`
}

// SESEventResult says what one SES notification turned into.
type SESEventResult struct {
	Recorded int    `json:"recorded"`
	Ignored  string `json:"ignored,omitempty"`
}

// RecordSESNotification ingests one SES notification (the SNS Message body).
// It only acts on mail this organization sent: the SES message id must resolve
// to a task on one of the organization's mailboxes.
func (s *service) RecordSESNotification(ctx context.Context, organizationID uuid.UUID, message []byte) (*SESEventResult, *errx.Error) {
	var n sesNotification
	if err := json.Unmarshal(message, &n); err != nil {
		return nil, errx.New(errx.BadRequest, "not an SES notification")
	}
	kind := n.NotificationType
	if kind == "" {
		kind = n.EventType
	}
	if n.Mail.MessageID == "" {
		return &SESEventResult{Ignored: "no message id"}, nil
	}

	task, err := s.taskRepo.GetTaskByProviderMessageID(ctx, n.Mail.MessageID)
	if (err != nil || task == nil) && n.Mail.CommonHeaders.MessageID != "" {
		task, err = s.taskRepo.GetTaskByMessageID(ctx, n.Mail.CommonHeaders.MessageID)
	}
	if err != nil || task == nil {
		return &SESEventResult{Ignored: "message not sent by this workspace"}, nil
	}
	account, aerr := s.emailRepo.GetByID(ctx, task.EmailAccountID)
	if aerr != nil || account == nil || account.OrganizationID == nil || *account.OrganizationID != organizationID {
		return &SESEventResult{Ignored: "message not sent by this workspace"}, nil
	}

	base := models.IngestDeliverabilityEventRequest{Provider: "ses", TaskID: &task.ID}
	if ct, cerr := s.taskRepo.GetCampaignTask(ctx, task.ID); cerr == nil && ct != nil {
		base.CampaignID = ct.CampaignID
		base.ContactID = ct.ContactID
	}
	sesID := n.Mail.MessageID
	res := &SESEventResult{}
	record := func(eventType models.DeliverabilityEventType, recipient, reason, key string, meta map[string]any) *errx.Error {
		recipient = strings.TrimSpace(recipient)
		if recipient == "" {
			return nil
		}
		req := base
		req.EventType = eventType
		req.RecipientEmail = recipient
		req.Reason = reason
		req.IdempotencyKey = key
		meta["ses_message_id"] = sesID
		req.Metadata = meta
		if xerr := s.IngestDeliverabilityEvent(ctx, organizationID, &req); xerr != nil {
			return xerr
		}
		res.Recorded++
		return nil
	}

	switch kind {
	case "Bounce":
		if n.Bounce == nil {
			break
		}
		// Only a permanent bounce is a bounce; transient ones are a deferral and never suppress.
		eventType, bounceType := models.DeliverabilityEventDeferred, "soft"
		if n.Bounce.BounceType == "Permanent" {
			eventType, bounceType = models.DeliverabilityEventBounce, "hard"
		}
		for _, r := range n.Bounce.BouncedRecipients {
			reason := firstNonEmpty(r.DiagnosticCode, n.Bounce.BounceType+" "+n.Bounce.BounceSubType)
			meta := map[string]any{"bounce_type": bounceType, "bounce_sub_type": n.Bounce.BounceSubType, "status": r.Status}
			if xerr := record(eventType, r.EmailAddress, reason, "ses:bounce:"+n.Bounce.FeedbackID+":"+strings.ToLower(r.EmailAddress), meta); xerr != nil {
				return nil, xerr
			}
		}
	case "Complaint":
		if n.Complaint == nil {
			break
		}
		for _, r := range n.Complaint.ComplainedRecipients {
			meta := map[string]any{"feedback_type": n.Complaint.ComplaintFeedbackType}
			if xerr := record(models.DeliverabilityEventComplaint, r.EmailAddress, firstNonEmpty(n.Complaint.ComplaintFeedbackType, "complaint"), "ses:complaint:"+n.Complaint.FeedbackID+":"+strings.ToLower(r.EmailAddress), meta); xerr != nil {
				return nil, xerr
			}
		}
	case "Delivery":
		if n.Delivery == nil {
			break
		}
		for _, r := range n.Delivery.Recipients {
			if xerr := record(models.DeliverabilityEventDelivered, r, n.Delivery.SMTPResponse, "ses:delivery:"+sesID+":"+strings.ToLower(r), map[string]any{}); xerr != nil {
				return nil, xerr
			}
		}
	case "DeliveryDelay":
		if n.DeliveryDelay == nil {
			break
		}
		for _, r := range n.DeliveryDelay.DelayedRecipients {
			meta := map[string]any{"bounce_type": "soft", "delay_type": n.DeliveryDelay.DelayType, "expires_at": n.DeliveryDelay.ExpirationTime}
			if xerr := record(models.DeliverabilityEventDeferred, r.EmailAddress, firstNonEmpty(r.DiagnosticCode, n.DeliveryDelay.DelayType), "ses:delay:"+sesID+":"+strings.ToLower(r.EmailAddress), meta); xerr != nil {
				return nil, xerr
			}
		}
	default:
		res.Ignored = "event type " + kind + " is not tracked"
	}
	return res, nil
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v = strings.TrimSpace(v); v != "" {
			return v
		}
	}
	return ""
}
