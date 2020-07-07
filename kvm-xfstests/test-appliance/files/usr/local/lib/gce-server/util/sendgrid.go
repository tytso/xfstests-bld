package util

import (
	"fmt"

	"github.com/sendgrid/sendgrid-go"
	"github.com/sendgrid/sendgrid-go/helpers/mail"
)

// SendEmail sends an email with subject and content to the receiver.
func SendEmail(subject string, content string, receiver string) error {
	if receiver == "" {
		return fmt.Errorf("No destination for report to be sent to")
	}

	config, err := GetConfig(GceConfigFile)
	if err != nil {
		return err
	}
	apiKey := config.Get("SENDGRID_API_KEY")
	if apiKey == "" {
		return fmt.Errorf("No sendgrid api key found")
	}

	sender := config.Get("GCE_REPORT_SENDER")
	if sender == "" {
		sender = receiver
	}

	m := mail.NewV3Mail()
	from := mail.NewEmail("Xfstests Reporter", sender)
	m.SetFrom(from)
	m.Subject = subject

	p := mail.NewPersonalization()
	to := mail.NewEmail("", receiver)
	p.AddTos(to)
	m.AddPersonalizations(p)

	c := mail.NewContent("text/plain", content)
	m.AddContent(c)

	client := sendgrid.NewSendClient(apiKey)
	response, err := client.Send(m)

	if err != nil {
		return err
	}

	if response.StatusCode >= 200 && response.StatusCode <= 299 {
		return nil
	}
	return fmt.Errorf("Send failed with code %d, response: %s", response.StatusCode, response.Body)
}
