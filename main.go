package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"log"
	"math/rand"
	"os"
	"strings"
	"time"

	"github.com/DusanKasan/parsemail"
	"github.com/PuerkitoBio/goquery"
	"github.com/cjdenio/temp-email/pkg/db"
	"github.com/cjdenio/temp-email/pkg/schedule"
	"github.com/cjdenio/temp-email/pkg/slackevents"
	"github.com/cjdenio/temp-email/pkg/util"
	"github.com/emersion/go-smtp"
	"github.com/slack-go/slack"
	"gorm.io/gorm"

	md "github.com/JohannesKaufmann/html-to-markdown"
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
	split := strings.Split(s.ToAddr, "@")

	if len(split) < 2 {
		return errors.New("invalid address")
	}

	var address db.Address
	tx := db.DB.Where("id = ? AND expires_at > NOW()", split[0]).First(&address)
	if tx.Error == gorm.ErrRecordNotFound {
		return errors.New("address not found")
	} else if tx.Error != nil {
		log.Println(tx.Error)
		return nil
	}

	rawEmail, err := io.ReadAll(r)
	if err != nil {
		log.Println(err)
	}

	email, err := parsemail.Parse(bytes.NewReader(rawEmail))
	if err != nil {
		log.Println(err)
	}

	savedEmail := &db.Email{
		ID:        util.GenerateEmailAddress(),
		AddressID: address.ID,
		Content:   string(rawEmail),
	}

	db.DB.Create(&savedEmail)

	subject := email.Subject
	if subject == "" {
		subject = "_no subject_"
	} else {
		subject = fmt.Sprintf("subject: *%s*", email.Subject)
	}

	body := ""

	if email.HTMLBody != "" {
		converter := md.NewConverter("", true, &md.Options{
			StrongDelimiter: "*",
			EmDelimiter:     "_",
		})

		converter.AddRules(
			md.Rule{
				Filter: []string{"a"},
				Replacement: func(content string, selec *goquery.Selection, options *md.Options) *string {
					return md.String(fmt.Sprintf("<%s|%s>", selec.AttrOr("href", content), content))
				},
			},
			md.Rule{
				Filter: []string{"h1", "h2", "h3", "h4", "h5", "h6"},
				Replacement: func(content string, selec *goquery.Selection, options *md.Options) *string {
					return md.String("\n\n*" + content + "*\n\n")
				},
			},
			md.Rule{
				Filter: []string{"img"},
				Replacement: func(content string, selec *goquery.Selection, options *md.Options) *string {
					return md.String("")
				},
			},
		)

		body, err = converter.ConvertString(email.HTMLBody)
		if err != nil {
			return errors.New("error parsing message")
		}
	} else {
		body = email.TextBody
	}

	_, _, err = slackevents.Client.PostMessage(
		os.Getenv("SLACK_CHANNEL"),
		slack.MsgOptionDisableLinkUnfurl(),
		slack.MsgOptionDisableMediaUnfurl(),
		slack.MsgOptionTS(address.Timestamp),
		slack.MsgOptionBlocks(
			slack.NewSectionBlock(
				slack.NewTextBlockObject("mrkdwn", fmt.Sprintf("message from %s\n%s", email.From[0].Address, util.SanitizeInput(subject)), false, false),
				nil,
				nil,
			),
			slack.NewDividerBlock(),
			slack.NewSectionBlock(
				slack.NewTextBlockObject("mrkdwn", util.SanitizeInput(body), false, false),
				nil,
				nil,
			),
			slack.NewDividerBlock(),
			slack.NewContextBlock("", slack.NewTextBlockObject("mrkdwn", fmt.Sprintf("Not rendering properly? Click <%s/%s|here> to view this email in your browser.", os.Getenv("APP_DOMAIN"), savedEmail.ID), false, false)),
		),
	)
	if err != nil {
		log.Println(err)
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
