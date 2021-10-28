package main

import (
	"encoding/base64"
	"fmt"
	"io"
	"log"
	"math/rand"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net/mail"
	"os"
	"strings"
	"time"

	"github.com/cjdenio/temp-email/pkg/db"
	"github.com/cjdenio/temp-email/pkg/schedule"
	"github.com/cjdenio/temp-email/pkg/slackevents"
	"github.com/cjdenio/temp-email/pkg/util"
	"github.com/emersion/go-smtp"
	"github.com/slack-go/slack"
)

type Session struct {
	FromAddr string
	ToAddr   string
}

func (s *Session) Reset() {
	s.FromAddr = ""
	s.ToAddr = ""
}
func (s *Session) Logout() error { return nil }
func (s *Session) Mail(from string, opts smtp.MailOptions) error {
	s.FromAddr = from
	return nil
}
func (s *Session) Rcpt(to string) error {
	s.ToAddr = to
	return nil
}
func (s *Session) Data(r io.Reader) error {
	msg, err := mail.ReadMessage(r)
	if err != nil {
		log.Println(err)
		return nil
	}

	split := strings.Split(s.ToAddr, "@")

	if len(split) >= 1 {
		var email db.Email
		tx := db.DB.Where("id = ? AND expires_at > NOW()", split[0]).First(&email)
		if tx.Error != nil {
			log.Println(tx.Error)
			return nil
		}

		message := ""
		from := s.FromAddr

		addresses, err := msg.Header.AddressList("From")
		if err == nil && len(addresses) >= 1 {
			from = addresses[0].Address
		}

		content_type, params, _ := mime.ParseMediaType(msg.Header.Get("Content-Type"))

		if strings.Contains(content_type, "multipart") {
			log.Println("it was multipart")

			r := multipart.NewReader(msg.Body, params["boundary"])

			parts := map[string]string{}
			random_part := ""

			for {
				part, err := r.NextPart()
				if err != nil {
					log.Println(err)
					break
				}

				content_type, _, _ := mime.ParseMediaType(part.Header.Get("Content-Type"))

				if part.Header.Get("Content-Transfer-Encoding") == "base64" {
					body, _ := io.ReadAll(part)

					out := []byte{}
					base64.StdEncoding.Decode(out, body)

					parts[content_type] = string(out)
					random_part = string(out)
				} else {
					body, _ := io.ReadAll(part)

					parts[content_type] = string(body)
					random_part = string(body)
				}
			}

			if part, ok := parts["text/plain"]; ok {
				message = part
			} else if part, ok := parts["text/html"]; ok {
				message = part
			} else {
				message = random_part
			}
		} else {
			log.Println("it was not multipart")

			if msg.Header.Get("Content-Transfer-Encoding") == "base64" {
				body, _ := io.ReadAll(msg.Body)

				out := []byte{}
				base64.StdEncoding.Decode(out, body)

				message = string(out)
			} else if msg.Header.Get("Content-Transfer-Encoding") == "quoted-printable" {
				r := quotedprintable.NewReader(msg.Body)
				body, _ := io.ReadAll(r)

				message = string(body)
			} else {
				body, _ := io.ReadAll(msg.Body)

				message = string(body)
			}
		}

		subject := msg.Header.Get("Subject")
		if subject == "" {
			subject = "_no subject_"
		} else {
			subject = "subject: *" + util.ParseMailHeader(subject) + "*"
		}

		log.Printf("Message: %s\n", message)

		_, _, err = slackevents.Client.PostMessage(
			os.Getenv("SLACK_CHANNEL"),
			slack.MsgOptionText(fmt.Sprintf("message from %s:\n%s\n\n```%s```", from, subject, message), false),
			slack.MsgOptionTS(email.Timestamp),
		)
		if err != nil {
			log.Println(err)
		}
	}

	return nil
}

type Backend struct{}

func (b Backend) Login(state *smtp.ConnectionState, username, password string) (smtp.Session, error) {
	return nil, smtp.ErrAuthUnsupported
}

func (b Backend) AnonymousLogin(state *smtp.ConnectionState) (smtp.Session, error) {
	return &Session{}, nil
}

func main() {
	rand.Seed(time.Now().UnixNano())

	db.Connect()

	backend := Backend{}
	server := smtp.NewServer(backend)

	server.Addr = ":3000"
	server.Domain = os.Getenv("DOMAIN")

	// Spin up an SMTP server in a goroutine
	go func() {
		log.Println("Starting up SMTP server...")

		err := server.ListenAndServe()
		if err != nil {
			log.Fatal(err)
		}
	}()

	// Start the scheduler
	schedule.Start()

	// Start listening for Slack events
	slackevents.Start()
}
