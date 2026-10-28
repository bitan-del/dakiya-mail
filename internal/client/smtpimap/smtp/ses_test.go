package smtp

import "testing"

func TestSESMessageID(t *testing.T) {
	cases := []struct {
		host, reply, want string
	}{
		{"email-smtp.us-east-1.amazonaws.com", "Ok 0100018c4b9e2f6a-1d2c3b4a-000000", "0100018c4b9e2f6a-1d2c3b4a-000000"},
		{"email-smtp.eu-west-1.amazonaws.com.", "OK <0102018c-abc-000000>", "0102018c-abc-000000"},
		{"smtp.gmail.com", "2.0.0 OK 1698 abc - gsmtp", ""},
		{"email-smtp.us-east-1.amazonaws.com", "2.0.0 queued", ""},
		{"email-smtp.us-east-1.amazonaws.com", "", ""},
	}
	for _, c := range cases {
		if got := SESMessageID(c.host, c.reply); got != c.want {
			t.Errorf("SESMessageID(%q, %q) = %q, want %q", c.host, c.reply, got, c.want)
		}
	}
}
