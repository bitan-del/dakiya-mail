package validate

import (
	"github.com/bitan-del/dakiya-mail/internal/errx"
	"github.com/google/uuid"
)

func Uuid(id string) (*string, *errx.Error) {
	if id == "" {
		return nil, nil
	}

	uid, err := uuid.Parse(id)
	if err != nil {
		return nil, errx.ErrUuid
	}

	uidstr := uid.String()

	return &uidstr, nil
}
