package handler

import (
	"net/http"

	"github.com/bitan-del/dakiya-mail/internal/errx"
	"github.com/bitan-del/dakiya-mail/internal/tasks/proto"
	"github.com/gin-gonic/gin"
	protob "google.golang.org/protobuf/proto"
)

func (h *Handler) HandleCampaignTasks(c *gin.Context) {
	rawData, err := c.GetRawData()
	if err != nil {
		errx.Handle(c, errx.ErrInvalid)
		return
	}

	var taskPayload proto.ProcessTask

	if err := protob.Unmarshal(rawData, &taskPayload); err != nil {
		errx.Handle(c, errx.ErrInvalid)
		return
	}

	if err := h.TasksService.HandleCampaignTask(&taskPayload); err != nil {
		errx.Handle(c, err)
		return
	}

	c.Status(http.StatusNoContent)
}
