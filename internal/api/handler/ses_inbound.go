package handler

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/bitan-del/dakiya-mail/internal/models"
	"github.com/bitan-del/dakiya-mail/internal/pkg/snsverify"
	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
)

var snsVerifier = snsverify.New()

// InboundAmazonSES receives SNS deliveries for an Amazon SES connection. The
// secret in the path picks the workspace; the SNS signature proves the body
// came from AWS; the first topic seen is pinned so no other topic can post.
func (h *Handler) InboundAmazonSES(c *gin.Context) {
	secret := strings.TrimSpace(c.Param("secret"))
	if secret == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "secret required"})
		return
	}
	ctx := c.Request.Context()
	repo := h.IntegrationService.Repo()
	conn, err := repo.GetConnectionByInboundSecret(ctx, models.IntegrationAmazonSES, secret)
	if err != nil || conn == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "unknown secret"})
		return
	}

	body, err := io.ReadAll(io.LimitReader(c.Request.Body, 256<<10))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "read body failed"})
		return
	}
	var msg snsverify.Message
	if err := json.Unmarshal(body, &msg); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "not an SNS message"})
		return
	}
	if err := snsVerifier.Verify(ctx, &msg); err != nil {
		log.Warn().Err(err).Str("connection_id", conn.ID.String()).Msg("ses inbound: sns signature rejected")
		c.JSON(http.StatusForbidden, gin.H{"error": "signature verification failed"})
		return
	}

	fields := map[string]any{}
	_ = json.Unmarshal(conn.DisplayFields, &fields)
	pinned, _ := fields["topic_arn"].(string)
	if pinned != "" && pinned != msg.TopicArn {
		c.JSON(http.StatusForbidden, gin.H{"error": "topic not linked to this connection"})
		return
	}
	pin := func() {
		if pinned != "" {
			return
		}
		fields["topic_arn"] = msg.TopicArn
		df, _ := json.Marshal(fields)
		if err := repo.MarkConnectionSynced(ctx, conn.ID, models.IntegrationStatusConnected, df, ""); err != nil {
			log.Warn().Err(err).Str("connection_id", conn.ID.String()).Msg("ses inbound: could not pin topic")
		}
	}

	switch msg.Type {
	case snsverify.TypeSubscriptionConfirmation:
		if err := snsVerifier.Confirm(ctx, &msg); err != nil {
			log.Warn().Err(err).Str("connection_id", conn.ID.String()).Msg("ses inbound: subscription confirm failed")
			c.JSON(http.StatusBadGateway, gin.H{"error": "could not confirm subscription"})
			return
		}
		pin()
		c.JSON(http.StatusOK, gin.H{"confirmed": true})
	case snsverify.TypeUnsubscribeConfirmation:
		c.JSON(http.StatusOK, gin.H{"received": true})
	case snsverify.TypeNotification:
		pin()
		res, xerr := h.AdvancedService.RecordSESNotification(ctx, conn.OrganizationID, []byte(msg.Message))
		if xerr != nil {
			// 5xx makes SNS retry; a malformed payload will never succeed, so ack it.
			if xerr.Code >= 500 {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "processing failed"})
				return
			}
			c.JSON(http.StatusOK, gin.H{"ignored": xerr.Message})
			return
		}
		c.JSON(http.StatusOK, res)
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "unsupported SNS message type"})
	}
}
