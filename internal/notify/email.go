package notify

import (
	"context"
	"fmt"
	"html/template"
	"net/smtp"
	"strings"
)

type SMTPMailer struct {
	host     string
	port     string
	username string
	password string
	from     string
	appURL   string
}

func NewSMTPMailer(host, port, username, password, from, appURL string) *SMTPMailer {
	return &SMTPMailer{
		host:     host,
		port:     port,
		username: username,
		password: password,
		from:     from,
		appURL:   strings.TrimRight(appURL, "/"),
	}
}

func (m *SMTPMailer) SendVerificationEmail(_ context.Context, toEmail, token string) error {
	if m.host == "" || m.port == "" || m.from == "" {
		return fmt.Errorf("smtp is not configured")
	}

	verifyURL := fmt.Sprintf("%s/verify-email?email=%s&token=%s", m.appURL, toEmail, token)
	subject := "Verify your Xend account"

	textBody := "Welcome to Xend!\n\n" +
		"Your verification code: " + token + "\n\n" +
		"Or verify with this link: " + verifyURL + "\n\n" +
		"This code expires in 24 hours."

	htmlBody, err := renderVerificationHTML(token, verifyURL)
	if err != nil {
		return err
	}

	msg := "From: " + m.from + "\r\n" +
		"To: " + toEmail + "\r\n" +
		"Subject: " + subject + "\r\n" +
		"MIME-Version: 1.0\r\n" +
		"Content-Type: multipart/alternative; boundary=boundary123\r\n\r\n" +
		"--boundary123\r\n" +
		"Content-Type: text/plain; charset=UTF-8\r\n\r\n" + textBody + "\r\n\r\n" +
		"--boundary123\r\n" +
		"Content-Type: text/html; charset=UTF-8\r\n\r\n" + htmlBody + "\r\n\r\n" +
		"--boundary123--"

	addr := m.host + ":" + m.port
	var auth smtp.Auth
	if m.username != "" && m.password != "" {
		auth = smtp.PlainAuth("", m.username, m.password, m.host)
	}
	return smtp.SendMail(addr, auth, m.from, []string{toEmail}, []byte(msg))
}

func (m *SMTPMailer) SendRelationshipInviteEmail(_ context.Context, toEmail, inviterDisplayName, inviterIdentifier, note string) error {
	if m.host == "" || m.port == "" || m.from == "" {
		return fmt.Errorf("smtp is not configured")
	}

	subject := "You received a relationship invite on Xend"
	textBody := inviterDisplayName + " invited you on Xend.\n\n" +
		"Identifier: " + inviterIdentifier + "\n"
	if strings.TrimSpace(note) != "" {
		textBody += "\nMessage:\n" + note + "\n"
	}
	textBody += "\nOpen Xend app and check your invite inbox."

	msg := "From: " + m.from + "\r\n" +
		"To: " + toEmail + "\r\n" +
		"Subject: " + subject + "\r\n" +
		"MIME-Version: 1.0\r\n" +
		"Content-Type: text/plain; charset=UTF-8\r\n\r\n" +
		textBody

	addr := m.host + ":" + m.port
	var auth smtp.Auth
	if m.username != "" && m.password != "" {
		auth = smtp.PlainAuth("", m.username, m.password, m.host)
	}
	return smtp.SendMail(addr, auth, m.from, []string{toEmail}, []byte(msg))
}

func renderVerificationHTML(token, verifyURL string) (string, error) {
	const tpl = `<!doctype html>
<html>
  <body style="margin:0;padding:0;background:#f5f2ff;font-family:Arial,sans-serif;color:#1f1633;">
    <table width="100%" height="100%" cellpadding="0" cellspacing="0" style="margin:0;padding:0;">
      <tr>
        <td align="center" valign="top" style="padding:0;">
          <table width="100%" height="100%" cellpadding="0" cellspacing="0" style="background:#ffffff;overflow:hidden;border:0;">
            <tr>
              <td style="background:linear-gradient(135deg,#6f2cff,#8f4dff);padding:20px 24px;color:#ffffff;font-size:22px;font-weight:700;">
                Xend
              </td>
            </tr>
            <tr>
              <td style="padding:24px;">
                <p style="margin:0 0 12px 0;font-size:18px;font-weight:700;">Verify your email</p>
                <p style="margin:0 0 16px 0;font-size:14px;line-height:1.6;color:#4a3b6b;">
                  Welcome to Xend. Use this verification code to finish creating your account.
                </p>
                <div style="margin:18px 0;padding:14px 16px;background:#f3edff;border:1px dashed #b99cff;border-radius:10px;font-size:28px;font-weight:800;letter-spacing:4px;text-align:center;color:#5b21b6;">
                  {{.Code}}
                </div>
                <p style="margin:0 0 18px 0;font-size:13px;color:#6b5a92;">Code expires in 24 hours.</p>
                <a href="{{.URL}}" style="display:inline-block;background:#6f2cff;color:#ffffff;text-decoration:none;padding:12px 18px;border-radius:10px;font-weight:700;font-size:14px;">Verify Email</a>
                <p style="margin:18px 0 0 0;font-size:12px;line-height:1.6;color:#8a7aa8;">
                  If you did not request this, you can ignore this email.
                </p>
              </td>
            </tr>
          </table>
        </td>
      </tr>
    </table>
  </body>
</html>`

	t, err := template.New("verify_email").Parse(tpl)
	if err != nil {
		return "", err
	}

	var b strings.Builder
	data := struct {
		Code string
		URL  string
	}{
		Code: token,
		URL:  verifyURL,
	}
	if err := t.Execute(&b, data); err != nil {
		return "", err
	}
	return b.String(), nil
}
