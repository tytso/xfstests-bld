package util

import (
	"io/ioutil"
	"log"

	"github.com/sendgrid/sendgrid-go"
	"github.com/sendgrid/sendgrid-go/helpers/mail"
)

// SendEmail sends the contents in filePath to receiver.
// return true on success and false otherwise.
func SendEmail(subject string, receiver string, filePath string) bool {
	if receiver == "" {
		log.Printf("No destination for report to be sent to")
		return false
	}

	config := GetConfig(GceConfigFile)
	apiKey := config.Get("SENDGRID_API_KEY")
	if apiKey == "" {
		log.Printf("No sendgrid api key found")
		return false
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

	file, err := ioutil.ReadFile(filePath)
	if err != nil {
		log.Printf("Unable to read file %s", filePath)
		return false
	}
	c := mail.NewContent("text/plain", string(file))
	m.AddContent(c)

	client := sendgrid.NewSendClient(apiKey)
	response, err := client.Send(m)

	if err != nil {
		log.Printf("Send email failed with error: %s", err)
		return false
	}

	if response.StatusCode >= 200 && response.StatusCode <= 299 {
		log.Printf("Send email succeeded with code %d", response.StatusCode)
		return true
	}
	log.Printf("Send email failed with code %d", response.StatusCode)
	log.Printf("response: %s", response.Body)
	return false
}
