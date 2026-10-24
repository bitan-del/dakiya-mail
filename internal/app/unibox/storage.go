package unibox

import (
	"bytes"
	"context"

	"github.com/bitan-del/dakiya-mail/internal/pkg/emsg"
	"github.com/google/uuid"
)

func GetEmailKey(userID, id uuid.UUID) string {
	return "emails/" + userID.String() + "/" + id.String()

}

func (s *uniboxService) GetBody(
	ctx context.Context,
	userID, id uuid.UUID,
) (*emsg.EmailBlob, error) {
	key := GetEmailKey(userID, id)
	body, err := s.blob.Get(ctx, key)
	if err != nil {
		return nil, err
	}
	defer body.Close()

	obj, err := emsg.DecodeBinary(body)
	if err != nil {
		return nil, err
	}

	return obj, nil
}

func (s *uniboxService) PutBody(
	ctx context.Context,
	userID, id uuid.UUID,
	plainText string,
	htmlText string,
) error {
	key := GetEmailKey(userID, id)

	blob := &emsg.EmailBlob{
		PlainText: []byte(plainText),
		HTMLBody:  []byte(htmlText),
	}

	body, err := blob.EncodeBinary()
	if err != nil {
		return err
	}

	return s.blob.Put(ctx, key, bytes.NewReader(body), "")
}
