package main

import (
	"fmt"
	"io"
	"log"
	"math/rand"
	"mime"
	"mime/multipart"
	"net/mail"
	"strings"
	"time"

	"github.com/cjdenio/temp-email/pkg/db"
	"github.com/cjdenio/temp-email/pkg/slackevents"
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
	msg, _ := mail.ReadMessage(r)

	split := strings.Split(s.ToAddr, "@")

	if len(split) >= 1 {
		var email db.Email
		tx := db.DB.Where("id = ? AND expires_at > NOW()", split[0]).First(&email)
		if tx.Error == nil {
			message := ""

			content_type, params, err := mime.ParseMediaType(msg.Header.Get("Content-Type"))
			if err != nil {
				return nil
			}

			if strings.Contains(content_type, "multipart") {
				r := multipart.NewReader(msg.Body, params["boundary"])
				for {
					part, err := r.NextPart()
					if err != nil {
						return nil
					}

					if strings.Contains(part.Header.Get("Content-Type"), "text/plain") {
						body, _ := io.ReadAll(part)

						message = string(body)
						break
					}
				}
			} else {
				body, _ := io.ReadAll(msg.Body)
				message = string(body)
			}

			subject := msg.Header.Get("Subject")
			if subject == "" {
				subject = "_no subject_"
			} else {
				subject = "subject: *" + subject + "*"
			}

			slackevents.Client.PostMessage(
				"C02GK2TVAVB",
				slack.MsgOptionText(fmt.Sprintf("message from %s:\n%s\n\n```%s```", s.FromAddr, subject, message), false),
				slack.MsgOptionTS(email.Timestamp),
			)
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
	server.Domain = "calebdenio.me"

	// Spin up an SMTP server in a goroutine
	go func() {
		log.Println("Starting up SMTP server...")

		err := server.ListenAndServe()
		if err != nil {
			log.Fatal(err)
		}
	}()

	// Start listening for Slack events
	slackevents.Start()
}
