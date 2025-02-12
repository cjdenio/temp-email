package slackevents

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/DusanKasan/parsemail"
	"github.com/cjdenio/temp-email/pkg/db"
	"github.com/cjdenio/temp-email/pkg/util"
	"github.com/gin-gonic/gin"
	"github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"
	"gorm.io/gorm"
)

var Client *slack.Client

func topLevelMessage(ev *slackevents.MessageEvent) bool {
	return ev.Channel == os.Getenv("SLACK_CHANNEL") && ev.ThreadTimeStamp == ""
}

func Start() {
	Client = slack.New(os.Getenv("SLACK_TOKEN"))

	r := gin.Default()

	r.POST("/slack/events", func(c *gin.Context) {
		body, err := ioutil.ReadAll(c.Request.Body)
		if err != nil {
			c.Writer.WriteHeader(http.StatusBadRequest)
			return
		}
		sv, err := slack.NewSecretsVerifier(c.Request.Header, os.Getenv("SLACK_SIGNING_SECRET"))
		if err != nil {
			c.Writer.WriteHeader(http.StatusBadRequest)
			return
		}
		if _, err := sv.Write(body); err != nil {
			c.Writer.WriteHeader(http.StatusInternalServerError)
			return
		}
		if err := sv.Ensure(); err != nil {
			c.Writer.WriteHeader(http.StatusUnauthorized)
			return
		}
		eventsAPIEvent, err := slackevents.ParseEvent(json.RawMessage(body), slackevents.OptionNoVerifyToken())
		if err != nil {
			c.Writer.WriteHeader(http.StatusInternalServerError)
			return
		}

		if eventsAPIEvent.Type == slackevents.URLVerification {
			var r *slackevents.ChallengeResponse
			err := json.Unmarshal([]byte(body), &r)
			if err != nil {
				c.Writer.WriteHeader(http.StatusInternalServerError)
				return
			}
			c.Writer.Header().Set("Content-Type", "text")
			c.Writer.Write([]byte(r.Challenge))
		}
		if eventsAPIEvent.Type == slackevents.CallbackEvent {
			innerEvent := eventsAPIEvent.InnerEvent
			switch ev := innerEvent.Data.(type) {
			case *slackevents.MessageEvent:
				if ev.SubType == "" && topLevelMessage(ev) && strings.Contains(strings.ToLower(ev.Text), "gib email") {
					address := util.GenerateEmailAddress()

					err = Client.AddReaction("thumb", slack.ItemRef{
						Channel:   ev.Channel,
						Timestamp: ev.TimeStamp,
					})
					if err != nil {
						fmt.Println(err)
					}

					Client.PostMessage(
						ev.Channel,
						slack.MsgOptionText(fmt.Sprintf(`wahoo! your temporary 24-hour email address is %s@%s
						
to stop receiving emails, delete your 'gib email' message.

i'll post emails in this thread :arrow_down:`, address, os.Getenv("DOMAIN")), false),
						slack.MsgOptionTS(ev.TimeStamp),
					)

					email := db.Address{
						ID:        address,
						CreatedAt: time.Now(),
						ExpiresAt: time.Now().Add(24 * time.Hour),
						Timestamp: ev.TimeStamp,
						User:      ev.User,
					}

					db.DB.Create(&email)
				} else if ev.SubType == "" && topLevelMessage(ev) && strings.HasPrefix(strings.ToLower(ev.Text), "gib ") {
					Client.PostMessage(ev.Channel, slack.MsgOptionText(fmt.Sprintf("unfortunately i am unable to _\"gib %s\"_. maybe try _\"gib email\"_?", strings.TrimPrefix(strings.ToLower(ev.Text), "gib ")), false), slack.MsgOptionTS(ev.TimeStamp))
				} else if (ev.SubType == "message_deleted" || (ev.SubType == "message_changed" && ev.Message.SubType == "tombstone")) && topLevelMessage(ev) {
					var address db.Address
					tx := db.DB.Where("timestamp = ? AND expires_at > NOW()", ev.PreviousMessage.TimeStamp).First(&address)

					if tx.Error == nil {
						address.ExpiresAt = time.Now()
						address.ExpiredMessageSent = true
						tx = db.DB.Save(&address)
						if tx.Error == nil {
							Client.PostMessage(
								os.Getenv("SLACK_CHANNEL"),
								slack.MsgOptionText(":x: since you deleted your message, this address has been deactivated.", false),
								slack.MsgOptionTS(address.Timestamp),
							)
						}

					}
				}
			}
		}
	})

	r.POST("/slack/interactivity", func(c *gin.Context) {
		body, err := ioutil.ReadAll(c.Request.Body)
		if err != nil {
			c.Writer.WriteHeader(http.StatusBadRequest)
			return
		}
		sv, err := slack.NewSecretsVerifier(c.Request.Header, os.Getenv("SLACK_SIGNING_SECRET"))
		if err != nil {
			c.Writer.WriteHeader(http.StatusBadRequest)
			return
		}
		if _, err := sv.Write(body); err != nil {
			c.Writer.WriteHeader(http.StatusInternalServerError)
			return
		}
		if err := sv.Ensure(); err != nil {
			c.Writer.WriteHeader(http.StatusUnauthorized)
			return
		}

		form, err := url.ParseQuery(string(body))
		if err != nil {
			fmt.Println(err)
		}

		var payload slack.InteractionCallback

		err = json.Unmarshal([]byte(form.Get("payload")), &payload)
		if err != nil {
			fmt.Printf("Could not parse action response JSON: %v", err)
		}

		if payload.ActionCallback.BlockActions[0].ActionID == "reactivate" {
			id := payload.ActionCallback.BlockActions[0].Value
			var address db.Address
			tx := db.DB.Where("id = ? AND expires_at < NOW()", id).First(&address)
			if tx.Error != nil {
				return
			}

			if payload.User.ID != address.User {
				Client.PostEphemeral(os.Getenv("SLACK_CHANNEL"), payload.User.ID, slack.MsgOptionTS(address.Timestamp), slack.MsgOptionText("whatcha tryin' to pull here :face_with_raised_eyebrow:", false))
				return
			}

			address.ExpiresAt = time.Now().Add(24 * time.Hour)
			address.ExpiredMessageSent = false

			db.DB.Save(&address)

			Client.PostMessage(os.Getenv("SLACK_CHANNEL"), slack.MsgOptionTS(address.Timestamp), slack.MsgOptionText("This address will be available for another 24 hours!", false))
			Client.RemoveReaction("clock1", slack.ItemRef{
				Channel:   os.Getenv("SLACK_CHANNEL"),
				Timestamp: address.Timestamp,
			})
		}
	})

	r.GET("/:email", func(c *gin.Context) {
		var rawEmail db.Email
		tx := db.DB.Where("id = ?", c.Param("email")).First(&rawEmail)
		if tx.Error == gorm.ErrRecordNotFound {
			c.String(404, "404 email not found :(")
			return
		} else if tx.Error != nil {
			c.String(500, "aaaaaaaaaaaaaaaaaaaa something went wrong")
			return
		}

		email, err := parsemail.Parse(strings.NewReader(rawEmail.Content))
		if err != nil {
			c.String(500, "aaaaaaaaaaaaaaaaaaaa something went wrong")
			return
		}

		if email.HTMLBody != "" {
			c.Header("Content-Type", "text/html; charset=utf-8")

			c.String(200, email.HTMLBody)
		} else if email.TextBody != "" {

			c.Header("Content-Type", "text/plain; charset=utf-8")

			c.String(200, email.TextBody)
		} else {
			c.Header("Content-Type", "text/plain")

			c.String(200, "Something went wrong: this message has no content :(")
		}
	})

	log.Println("Starting up HTTP server...")

	r.Run(":3001")
}
