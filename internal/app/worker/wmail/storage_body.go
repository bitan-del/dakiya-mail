package wmail

import (
	"bytes"
	"context"

	"github.com/bitan-del/dakiya-mail/internal/config"
	"github.com/bitan-del/dakiya-mail/internal/pkg/emsg"
	"github.com/google/uuid"
)

func (c *WMail) Exists(ctx context.Context, messageID uuid.UUID) (bool, error) {
	key := config.StorageEndpointEmailBody(c.UserID, c.ID, messageID)
	return c.Storage.Has(ctx, key)
}

func (c *WMail) StoreBody(ctx context.Context, emailMessageID uuid.UUID, data *emsg.EmailBlob) error {
	bytedata, err := data.EncodeBinary()
	if err != nil {
		return err
	}

	key := config.StorageEndpointEmailBody(c.UserID, c.ID, emailMessageID)

	return c.Storage.Put(ctx, key, bytes.NewReader(bytedata), "")
}
