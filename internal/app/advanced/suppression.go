package advanced

import (
	"context"
	"encoding/csv"
	"encoding/hex"
	"errors"
	"io"
	"net/mail"
	"strings"

	"github.com/bitan-del/dakiya-mail/internal/errx"
	"github.com/bitan-del/dakiya-mail/internal/models"
	"github.com/google/uuid"
)

const (
	maxSuppressionBatch     = 1000
	suppressionImportChunk  = 5000
	maxSuppressionImportRow = 5_000_000
)

// SuppressionFormat is the column format of an import or export file.
type SuppressionFormat string

const (
	SuppressionFormatPlain  SuppressionFormat = "plain"
	SuppressionFormatMD5    SuppressionFormat = "md5"
	SuppressionFormatSHA256 SuppressionFormat = "sha256"
)

func (f SuppressionFormat) valid() bool {
	return f == SuppressionFormatPlain || f == SuppressionFormatMD5 || f == SuppressionFormatSHA256
}

func validSuppressionEmail(raw string) (string, bool) {
	e := models.NormalizeSuppressionEmail(strings.Trim(raw, "<>\"' "))
	if e == "" || !strings.Contains(e, "@") {
		return "", false
	}
	if _, err := mail.ParseAddress(e); err != nil {
		return "", false
	}
	return e, true
}

func validDigest(raw string, algo models.SuppressionHashAlgo) (string, bool) {
	h := strings.ToLower(strings.TrimSpace(raw))
	if len(h) != algo.HexLen() {
		return "", false
	}
	if _, err := hex.DecodeString(h); err != nil {
		return "", false
	}
	return h, true
}

func (s *service) ListSuppressions(ctx context.Context, organizationID uuid.UUID, query, source string, limit int, cursor *uuid.UUID) (*models.SuppressionListResult, *errx.Error) {
	res, err := s.repo.ListSuppressions(ctx, organizationID, query, source, limit, cursor)
	if err != nil {
		return nil, toErrx(err)
	}
	return res, nil
}

func (s *service) AddSuppressions(ctx context.Context, organizationID uuid.UUID, req *models.AddSuppressionsRequest) (*models.SuppressionImportResult, *errx.Error) {
	if req == nil || len(req.Emails) == 0 {
		return nil, errx.New(errx.BadRequest, "emails is required")
	}
	if len(req.Emails) > maxSuppressionBatch {
		return nil, errx.New(errx.BadRequest, "at most 1000 emails per request; import a file for more")
	}
	res := &models.SuppressionImportResult{}
	valid := make([]string, 0, len(req.Emails))
	for _, raw := range req.Emails {
		if e, ok := validSuppressionEmail(raw); ok {
			valid = append(valid, e)
		} else {
			res.Invalid++
		}
	}
	reason := strings.TrimSpace(req.Reason)
	if reason == "" {
		reason = "added manually"
	}
	added, err := s.repo.BulkInsertSuppressedRecipients(ctx, organizationID, valid, reason, models.SuppressionSourceManual, req.ExpiresAt)
	if err != nil {
		return nil, toErrx(err)
	}
	res.Added = added
	res.Skipped = len(valid) - added
	return res, nil
}

func (s *service) RemoveSuppression(ctx context.Context, organizationID, id uuid.UUID) *errx.Error {
	ok, err := s.repo.DeleteSuppressedRecipient(ctx, organizationID, id)
	if err != nil {
		return toErrx(err)
	}
	if !ok {
		return errx.New(errx.NotFound, "suppression not found")
	}
	return nil
}

func (s *service) RemoveSuppressionEmails(ctx context.Context, organizationID uuid.UUID, emails []string) (int, *errx.Error) {
	if len(emails) == 0 {
		return 0, errx.New(errx.BadRequest, "emails is required")
	}
	if len(emails) > maxSuppressionBatch {
		return 0, errx.New(errx.BadRequest, "at most 1000 emails per request")
	}
	n, err := s.repo.DeleteSuppressedEmails(ctx, organizationID, emails)
	if err != nil {
		return 0, toErrx(err)
	}
	return n, nil
}

// ImportSuppressions reads the first column of a CSV or plain-text file. A
// first row that is not a valid value is treated as a header and skipped.
func (s *service) ImportSuppressions(ctx context.Context, organizationID uuid.UUID, r io.Reader, format SuppressionFormat, listName, reason string) (*models.SuppressionImportResult, *errx.Error) {
	if !format.valid() {
		return nil, errx.New(errx.BadRequest, "format must be plain, md5 or sha256")
	}
	listName = strings.TrimSpace(listName)
	if format != SuppressionFormatPlain && listName == "" {
		return nil, errx.New(errx.BadRequest, "list_name is required for hashed lists")
	}
	reason = strings.TrimSpace(reason)
	if reason == "" {
		reason = "imported list"
		if listName != "" {
			reason += ": " + listName
		}
	}

	cr := csv.NewReader(r)
	cr.LazyQuotes = true
	cr.FieldsPerRecord = -1
	cr.ReuseRecord = true

	res := &models.SuppressionImportResult{}
	batch := make([]string, 0, suppressionImportChunk)
	flush := func() *errx.Error {
		if len(batch) == 0 {
			return nil
		}
		var (
			added int
			err   error
		)
		if format == SuppressionFormatPlain {
			added, err = s.repo.BulkInsertSuppressedRecipients(ctx, organizationID, batch, reason, models.SuppressionSourceImport, nil)
		} else {
			added, err = s.repo.InsertSuppressedHashes(ctx, organizationID, models.SuppressionHashAlgo(format), listName, batch)
		}
		if err != nil {
			return toErrx(err)
		}
		res.Added += added
		res.Skipped += len(batch) - added
		batch = batch[:0]
		return nil
	}

	rowNum := 0
	for {
		rec, err := cr.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, errx.New(errx.BadRequest, "could not read the file as CSV or plain text")
		}
		rowNum++
		if rowNum > maxSuppressionImportRow {
			return nil, errx.New(errx.BadRequest, "file has more than 5,000,000 rows; split it")
		}
		cell := ""
		for _, c := range rec {
			if strings.TrimSpace(c) != "" {
				cell = c
				break
			}
		}
		if cell == "" {
			continue
		}
		var (
			value string
			ok    bool
		)
		if format == SuppressionFormatPlain {
			value, ok = validSuppressionEmail(cell)
		} else {
			value, ok = validDigest(cell, models.SuppressionHashAlgo(format))
		}
		if !ok {
			if rowNum > 1 {
				res.Invalid++
			}
			continue
		}
		batch = append(batch, value)
		if len(batch) >= suppressionImportChunk {
			if e := flush(); e != nil {
				return nil, e
			}
		}
	}
	if e := flush(); e != nil {
		return nil, e
	}
	return res, nil
}

func (s *service) ListSuppressionHashLists(ctx context.Context, organizationID uuid.UUID) ([]models.SuppressionHashList, *errx.Error) {
	lists, err := s.repo.ListSuppressionHashLists(ctx, organizationID)
	if err != nil {
		return nil, toErrx(err)
	}
	return lists, nil
}

func (s *service) DeleteSuppressionHashList(ctx context.Context, organizationID uuid.UUID, algo models.SuppressionHashAlgo, listName string) (int64, *errx.Error) {
	if algo.HexLen() == 0 {
		return 0, errx.New(errx.BadRequest, "algo must be md5 or sha256")
	}
	n, err := s.repo.DeleteSuppressionHashList(ctx, organizationID, algo, listName)
	if err != nil {
		return 0, toErrx(err)
	}
	if n == 0 {
		return 0, errx.New(errx.NotFound, "hashed list not found")
	}
	return n, nil
}

// ExportSuppressions writes the active plaintext list as CSV, optionally as digests.
func (s *service) ExportSuppressions(ctx context.Context, organizationID uuid.UUID, w io.Writer, format SuppressionFormat) (int, *errx.Error) {
	if !format.valid() {
		return 0, errx.New(errx.BadRequest, "format must be plain, md5 or sha256")
	}
	cw := csv.NewWriter(w)
	header := []string{"email", "reason", "source", "created_at"}
	if format != SuppressionFormatPlain {
		header = []string{string(format)}
	}
	if err := cw.Write(header); err != nil {
		return 0, toErrx(err)
	}
	rows := 0
	err := s.repo.EachSuppressedEmail(ctx, organizationID, func(entry *models.SuppressedRecipient) error {
		rows++
		switch format {
		case SuppressionFormatMD5:
			m, _ := models.SuppressionDigests(entry.Email)
			return cw.Write([]string{m})
		case SuppressionFormatSHA256:
			_, sh := models.SuppressionDigests(entry.Email)
			return cw.Write([]string{sh})
		}
		return cw.Write([]string{entry.Email, entry.Reason, string(entry.Source), entry.CreatedAt.UTC().Format("2006-01-02T15:04:05Z")})
	})
	if err != nil {
		return rows, toErrx(err)
	}
	cw.Flush()
	if err := cw.Error(); err != nil {
		return rows, toErrx(err)
	}
	return rows, nil
}

func (s *service) ListDeliverabilityEvents(ctx context.Context, organizationID uuid.UUID, f models.DeliverabilityEventFilter, limit int, cursor *uuid.UUID) (*models.DeliverabilityEventListResult, *errx.Error) {
	res, err := s.repo.ListDeliverabilityEvents(ctx, organizationID, f, limit, cursor)
	if err != nil {
		return nil, toErrx(err)
	}
	return res, nil
}
