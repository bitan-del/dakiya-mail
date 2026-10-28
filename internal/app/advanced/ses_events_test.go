package advanced

import (
	"context"
	"testing"

	"github.com/bitan-del/dakiya-mail/internal/errx"
	"github.com/bitan-del/dakiya-mail/internal/models"
	"github.com/bitan-del/dakiya-mail/internal/repository"
	"github.com/google/uuid"
)

type sesFakeOutreach struct {
	repository.AdvancedOutreachRepository
	events     []models.DeliverabilityEvent
	suppressed []string
}

func (f *sesFakeOutreach) CreateDeliverabilityEvent(_ context.Context, e *models.DeliverabilityEvent) (bool, error) {
	for _, prev := range f.events {
		if prev.IdempotencyKey == e.IdempotencyKey {
			return false, nil
		}
	}
	f.events = append(f.events, *e)
	return true, nil
}

func (f *sesFakeOutreach) TransactionalTrackIDs(context.Context, uuid.UUID) ([]string, error) {
	return nil, nil
}

func (f *sesFakeOutreach) GetOutreachSettings(context.Context, uuid.UUID) (*models.AdvancedOutreachSettings, error) {
	s := &models.AdvancedOutreachSettings{}
	s.BouncePipeline.AutoSuppressOnBounce = true
	s.BouncePipeline.AutoSuppressOnComplaint = true
	return s, nil
}

func (f *sesFakeOutreach) UpsertSuppressedRecipient(_ context.Context, e *models.SuppressedRecipient) error {
	f.suppressed = append(f.suppressed, e.Email)
	return nil
}

type sesFakeTasks struct {
	repository.TaskRepository
	byProvider map[string]*repository.Task
}

func (f *sesFakeTasks) GetTaskByProviderMessageID(_ context.Context, id string) (*repository.Task, error) {
	return f.byProvider[id], nil
}

func (f *sesFakeTasks) GetTaskByMessageID(context.Context, string) (*repository.Task, error) {
	return nil, nil
}

func (f *sesFakeTasks) GetCampaignTask(context.Context, uuid.UUID) (*repository.CampaignTask, error) {
	return nil, nil
}

type sesFakeEmails struct {
	repository.EmailRepository
	org uuid.UUID
}

func (f *sesFakeEmails) GetByID(context.Context, uuid.UUID) (*models.Email, *errx.Error) {
	return &models.Email{OrganizationID: &f.org}, nil
}

func TestRecordSESNotification(t *testing.T) {
	org := uuid.New()
	outreach := &sesFakeOutreach{}
	s := &service{
		repo:      outreach,
		taskRepo:  &sesFakeTasks{byProvider: map[string]*repository.Task{"ses-1": {ID: uuid.New(), EmailAccountID: uuid.New()}}},
		emailRepo: &sesFakeEmails{org: org},
	}
	ctx := context.Background()

	hard := `{"notificationType":"Bounce","mail":{"messageId":"ses-1"},"bounce":{"bounceType":"Permanent","bounceSubType":"General","feedbackId":"fb1","bouncedRecipients":[{"emailAddress":"Gone@Example.com","status":"5.1.1","diagnosticCode":"smtp; 550 5.1.1 user unknown"}]}}`
	soft := `{"notificationType":"Bounce","mail":{"messageId":"ses-1"},"bounce":{"bounceType":"Transient","bounceSubType":"MailboxFull","feedbackId":"fb2","bouncedRecipients":[{"emailAddress":"full@example.com"}]}}`
	delivered := `{"eventType":"Delivery","mail":{"messageId":"ses-1"},"delivery":{"recipients":["ok@example.com"],"smtpResponse":"250 2.0.0 OK"}}`
	complaint := `{"notificationType":"Complaint","mail":{"messageId":"ses-1"},"complaint":{"feedbackId":"fb3","complaintFeedbackType":"abuse","complainedRecipients":[{"emailAddress":"angry@example.com"}]}}`
	foreign := `{"notificationType":"Bounce","mail":{"messageId":"someone-else"},"bounce":{"bounceType":"Permanent","feedbackId":"fb4","bouncedRecipients":[{"emailAddress":"x@example.com"}]}}`

	for _, msg := range []string{hard, soft, delivered, complaint, hard} {
		if _, xerr := s.RecordSESNotification(ctx, org, []byte(msg)); xerr != nil {
			t.Fatalf("RecordSESNotification: %v", xerr)
		}
	}
	res, xerr := s.RecordSESNotification(ctx, org, []byte(foreign))
	if xerr != nil || res.Recorded != 0 || res.Ignored == "" {
		t.Fatalf("foreign message should be ignored, got %+v %v", res, xerr)
	}
	if res, _ := s.RecordSESNotification(ctx, uuid.New(), []byte(hard)); res.Recorded != 0 {
		t.Fatal("a message sent by another workspace must not be recorded")
	}

	want := []models.DeliverabilityEventType{
		models.DeliverabilityEventBounce,
		models.DeliverabilityEventDeferred,
		models.DeliverabilityEventDelivered,
		models.DeliverabilityEventComplaint,
	}
	if len(outreach.events) != len(want) {
		t.Fatalf("got %d events, want %d (the repeated hard bounce must dedupe)", len(outreach.events), len(want))
	}
	for i, w := range want {
		if outreach.events[i].EventType != w {
			t.Errorf("event %d = %s, want %s", i, outreach.events[i].EventType, w)
		}
	}
	if got := outreach.events[0].Metadata["bounce_type"]; got != "hard" {
		t.Errorf("hard bounce metadata bounce_type = %v", got)
	}
	if len(outreach.suppressed) != 2 || outreach.suppressed[0] != "Gone@Example.com" || outreach.suppressed[1] != "angry@example.com" {
		t.Errorf("suppressed = %v, want the hard bounce and the complaint only", outreach.suppressed)
	}
}
