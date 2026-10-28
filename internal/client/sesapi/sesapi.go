// Package sesapi sends a mailbox's mail through the Amazon SES v2 API with the
// mailbox's own access keys, as an alternative to SES SMTP credentials.
//
// A mailbox opts in by storing the SES API endpoint, email.<region>.amazonaws.com
// (not the email-smtp host), as its SMTP host, its access key id as the
// username and its secret access key as the password.
package sesapi

import (
	"context"
	"errors"
	"net"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/sesv2"
	"github.com/aws/aws-sdk-go-v2/service/sesv2/types"
	"github.com/aws/smithy-go"
	"github.com/bitan-del/dakiya-mail/internal/client/netbind"
)

// Port is what an SES API mailbox stores as its SMTP port.
const Port = 443

// endpointOverride points the client at a test server.
var endpointOverride string

var hostPattern = regexp.MustCompile(`^email\.([a-z0-9-]+)\.amazonaws\.com$`)

// Region returns the AWS region when host is an SES API endpoint.
func Region(host string) (string, bool) {
	m := hostPattern.FindStringSubmatch(strings.ToLower(strings.TrimSuffix(strings.TrimSpace(host), ".")))
	if m == nil {
		return "", false
	}
	return m[1], true
}

func client(region, accessKeyID, secret string, local *net.TCPAddr) *sesv2.Client {
	dialer := netbind.Dialer(local)
	httpClient := &http.Client{
		Timeout:   30 * time.Second,
		Transport: &http.Transport{DialContext: dialer.DialContext, TLSHandshakeTimeout: 10 * time.Second},
	}
	opts := sesv2.Options{
		Region:      region,
		Credentials: aws.NewCredentialsCache(credentials.NewStaticCredentialsProvider(accessKeyID, secret, "")),
		HTTPClient:  httpClient,
	}
	if endpointOverride != "" {
		opts.BaseEndpoint = aws.String(endpointOverride)
	}
	return sesv2.New(opts)
}

// Failure classifies why a send was refused.
type Failure int

const (
	FailureTransient Failure = iota
	FailureCredentials
	// FailureSender is the sender's fault (unverified identity, paused account),
	// never the recipient's, so it must not suppress anyone.
	FailureSender
	FailureRecipient
)

// SendError carries the classification next to the API error.
type SendError struct {
	Kind Failure
	Err  error
}

func (e *SendError) Error() string { return e.Err.Error() }
func (e *SendError) Unwrap() error { return e.Err }

func classify(err error) *SendError {
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		code := apiErr.ErrorCode()
		msg := strings.ToLower(apiErr.ErrorMessage())
		switch code {
		case "UnrecognizedClientException", "InvalidSignatureException", "SignatureDoesNotMatch", "AccessDeniedException", "InvalidClientTokenId":
			return &SendError{Kind: FailureCredentials, Err: err}
		case "MailFromDomainNotVerifiedException", "AccountSuspendedException", "SendingPausedException":
			return &SendError{Kind: FailureSender, Err: err}
		case "MessageRejected":
			if strings.Contains(msg, "not verified") || strings.Contains(msg, "identity") {
				return &SendError{Kind: FailureSender, Err: err}
			}
			return &SendError{Kind: FailureRecipient, Err: err}
		}
	}
	return &SendError{Kind: FailureTransient, Err: err}
}

// Send submits a raw RFC 5322 message and returns SES's message id.
func Send(ctx context.Context, region, accessKeyID, secret string, local *net.TCPAddr, from string, recipients []string, raw []byte) (string, error) {
	out, err := client(region, accessKeyID, secret, local).SendEmail(ctx, &sesv2.SendEmailInput{
		FromEmailAddress: aws.String(from),
		Destination:      &types.Destination{ToAddresses: recipients},
		Content:          &types.EmailContent{Raw: &types.RawMessage{Data: raw}},
	})
	if err != nil {
		return "", classify(err)
	}
	return aws.ToString(out.MessageId), nil
}

// Verify checks the keys can reach the account and that sending is enabled.
func Verify(ctx context.Context, region, accessKeyID, secret string) error {
	out, err := client(region, accessKeyID, secret, nil).GetAccount(ctx, &sesv2.GetAccountInput{})
	if err != nil {
		return classify(err)
	}
	if !out.SendingEnabled {
		return &SendError{Kind: FailureSender, Err: errors.New("sending is paused on this SES account")}
	}
	return nil
}
