package handler

import (
	"html"
	"net/http"
	"os"
	"slices"
	"strings"

	"github.com/bitan-del/dakiya-mail/internal/api/middleware"
	"github.com/bitan-del/dakiya-mail/internal/errx"
	"github.com/bitan-del/dakiya-mail/internal/models"
	"github.com/gin-gonic/gin"
)

// ---- dashboard ----

func (h *Handler) ListSignupForms(c *gin.Context) {
	orgID, _, ok := domainParams(c, false)
	if !ok {
		return
	}
	list, xerr := h.SignupFormService.List(c.Request.Context(), orgID)
	if xerr != nil {
		errx.JSON(c, xerr)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": list})
}

func (h *Handler) saveSignupForm(c *gin.Context, create bool) {
	orgID, id, ok := domainParams(c, !create)
	if !ok {
		return
	}
	userID, err := middleware.GetUserUUID(c)
	if err != nil {
		errx.JSON(c, errx.ErrUnauthorized)
		return
	}
	var req models.SaveSignupFormRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		errx.JSON(c, errx.ErrInvalid)
		return
	}
	target, action, status := &id, models.AuditActionUpdate, http.StatusOK
	if create {
		target, action, status = nil, models.AuditActionCreate, http.StatusCreated
	}
	f, xerr := h.SignupFormService.Save(c.Request.Context(), orgID, target, userID, &req)
	if xerr != nil {
		errx.JSON(c, xerr)
		return
	}
	h.auditOrg(c, action, models.AuditEntitySignupForm, &f.ID, nil, map[string]string{"name": f.Name, "slug": f.Slug})
	c.JSON(status, f)
}

func (h *Handler) CreateSignupForm(c *gin.Context) { h.saveSignupForm(c, true) }
func (h *Handler) UpdateSignupForm(c *gin.Context) { h.saveSignupForm(c, false) }

func (h *Handler) DeleteSignupForm(c *gin.Context) {
	orgID, id, ok := domainParams(c, true)
	if !ok {
		return
	}
	if xerr := h.SignupFormService.Delete(c.Request.Context(), orgID, id); xerr != nil {
		errx.JSON(c, xerr)
		return
	}
	h.auditOrg(c, models.AuditActionDelete, models.AuditEntitySignupForm, &id, nil, nil)
	c.Status(http.StatusNoContent)
}

// ---- public: /f/:slug ----

func publicBaseURL(c *gin.Context) string {
	if v := strings.TrimSpace(os.Getenv("API_PUBLIC_URL")); v != "" {
		return strings.TrimSuffix(v, "/")
	}
	scheme := "https"
	if c.Request.TLS == nil && c.GetHeader("X-Forwarded-Proto") != "https" {
		scheme = "http"
	}
	return scheme + "://" + c.Request.Host
}

var signupFieldLabels = map[string]string{"first_name": "First name", "last_name": "Last name", "company": "Company", "phone": "Phone"}

func signupPage(title, body string) []byte {
	return []byte(`<!doctype html><html lang="en"><head><meta charset="utf-8">` +
		`<meta name="viewport" content="width=device-width,initial-scale=1"><title>` + html.EscapeString(title) + `</title>` +
		`<style>body{font-family:system-ui,-apple-system,sans-serif;max-width:28rem;margin:3rem auto;padding:0 1rem;color:#0f172a}` +
		`h1{font-size:1.25rem;margin:0 0 .5rem}p{color:#475569;line-height:1.5}label{display:block;font-size:.8rem;color:#475569;margin:.75rem 0 .25rem}` +
		`input{width:100%;box-sizing:border-box;padding:.55rem .65rem;border:1px solid #cbd5e1;border-radius:.4rem;font-size:1rem}` +
		`button{margin-top:1rem;width:100%;padding:.6rem;border:0;border-radius:.4rem;background:#0284c7;color:#fff;font-size:1rem;cursor:pointer}` +
		`.hp{position:absolute;left:-9999px}.err{color:#be123c}</style></head><body>` + body + `</body></html>`)
}

func renderSignupForm(f *models.SignupForm, errMsg string) []byte {
	title := f.Title
	if title == "" {
		title = f.Name
	}
	var b strings.Builder
	b.WriteString(`<h1>` + html.EscapeString(title) + `</h1>`)
	if f.Description != "" {
		b.WriteString(`<p>` + strings.ReplaceAll(html.EscapeString(f.Description), "\n", "<br>") + `</p>`)
	}
	if errMsg != "" {
		b.WriteString(`<p class="err">` + html.EscapeString(errMsg) + `</p>`)
	}
	b.WriteString(`<form method="post" action="/f/` + html.EscapeString(f.Slug) + `">`)
	b.WriteString(`<label for="email">Email</label><input id="email" name="email" type="email" required autocomplete="email">`)
	for _, fld := range models.SignupFormFields {
		if slices.Contains(f.Fields, fld) {
			b.WriteString(`<label for="` + fld + `">` + signupFieldLabels[fld] + `</label><input id="` + fld + `" name="` + fld + `" type="text">`)
		}
	}
	b.WriteString(`<div class="hp" aria-hidden="true"><label for="website_url">Website</label><input id="website_url" name="website_url" tabindex="-1" autocomplete="off"></div>`)
	b.WriteString(`<button type="submit">Sign up</button></form>`)
	return signupPage(title, b.String())
}

func (h *Handler) publicForm(c *gin.Context) *models.SignupForm {
	f, xerr := h.SignupFormService.Public(c.Request.Context(), c.Param("slug"))
	if xerr != nil || f == nil {
		c.Data(http.StatusNotFound, "text/html; charset=utf-8", signupPage("Not found", `<h1>This form is not available.</h1>`))
		return nil
	}
	return f
}

func (h *Handler) ShowSignupForm(c *gin.Context) {
	if f := h.publicForm(c); f != nil {
		c.Header("Cache-Control", "no-store")
		c.Data(http.StatusOK, "text/html; charset=utf-8", renderSignupForm(f, ""))
	}
}

// SubmitSignupForm accepts the HTML form post, or JSON for sites posting with fetch.
func (h *Handler) SubmitSignupForm(c *gin.Context) {
	f := h.publicForm(c)
	if f == nil {
		return
	}
	asJSON := strings.HasPrefix(c.ContentType(), "application/json")
	var sub models.SignupSubmission
	if err := c.ShouldBind(&sub); err != nil {
		if asJSON {
			errx.JSON(c, errx.ErrInvalid)
			return
		}
		c.Data(http.StatusBadRequest, "text/html; charset=utf-8", renderSignupForm(f, "Something was wrong with that submission."))
		return
	}
	needsConfirm, xerr := h.SignupFormService.Submit(c.Request.Context(), f, &sub, publicBaseURL(c))
	if xerr != nil {
		if asJSON {
			errx.JSON(c, xerr)
			return
		}
		msg := "We could not sign you up right now. Please try again."
		if xerr.Code == errx.BadRequest {
			msg = xerr.Message
		}
		c.Data(http.StatusBadRequest, "text/html; charset=utf-8", renderSignupForm(f, msg))
		return
	}
	msg := f.SuccessMessage
	if needsConfirm {
		msg = "Check your inbox and click the link we sent to confirm your subscription."
	} else if msg == "" {
		msg = "Thanks, you're signed up."
	}
	if asJSON {
		c.JSON(http.StatusOK, gin.H{"ok": true, "needs_confirmation": needsConfirm, "message": msg})
		return
	}
	if !needsConfirm && f.RedirectURL != "" {
		c.Redirect(http.StatusSeeOther, f.RedirectURL)
		return
	}
	c.Data(http.StatusOK, "text/html; charset=utf-8", signupPage(f.Name, `<h1>`+html.EscapeString(msg)+`</h1>`))
}

func (h *Handler) ConfirmSignupForm(c *gin.Context) {
	f := h.publicForm(c)
	if f == nil {
		return
	}
	if xerr := h.SignupFormService.Confirm(c.Request.Context(), f, c.Query("t")); xerr != nil {
		msg := "We could not confirm your subscription right now."
		if xerr.Code == errx.BadRequest {
			msg = xerr.Message
		}
		c.Data(http.StatusBadRequest, "text/html; charset=utf-8", signupPage(f.Name, `<h1>`+html.EscapeString(msg)+`</h1>`))
		return
	}
	if f.RedirectURL != "" {
		c.Redirect(http.StatusSeeOther, f.RedirectURL)
		return
	}
	msg := f.SuccessMessage
	if msg == "" {
		msg = "You're subscribed. Thanks for confirming."
	}
	c.Data(http.StatusOK, "text/html; charset=utf-8", signupPage(f.Name, `<h1>`+html.EscapeString(msg)+`</h1>`))
}
