package slackevents

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/cjdenio/temp-email/pkg/db"
	"github.com/cjdenio/temp-email/pkg/util"
	"github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"
)

var Client = slack.New(os.Getenv("SLACK_TOKEN"))

func Start() {
	http.HandleFunc("/slack/events", func(w http.ResponseWriter, r *http.Request) {
		body, err := ioutil.ReadAll(r.Body)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		sv, err := slack.NewSecretsVerifier(r.Header, os.Getenv("SLACK_SIGNING_SECRET"))
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if _, err := sv.Write(body); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		if err := sv.Ensure(); err != nil {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		eventsAPIEvent, err := slackevents.ParseEvent(json.RawMessage(body), slackevents.OptionNoVerifyToken())
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		if eventsAPIEvent.Type == slackevents.URLVerification {
			var r *slackevents.ChallengeResponse
			err := json.Unmarshal([]byte(body), &r)
			if err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "text")
			w.Write([]byte(r.Challenge))
		}
		if eventsAPIEvent.Type == slackevents.CallbackEvent {
			innerEvent := eventsAPIEvent.InnerEvent
			switch ev := innerEvent.Data.(type) {
			case *slackevents.MessageEvent:
				if ev.SubType == "" && ev.Channel == os.Getenv("SLACK_CHANNEL") && ev.ThreadTimeStamp == "" && strings.Contains(ev.Text, "gib email") {
					address := util.GenerateEmailAddress()

					err = Client.AddReaction("thumb", slack.ItemRef{
						Channel:   ev.Channel,
						Timestamp: ev.TimeStamp,
					})
					if err != nil {
						fmt.Println(err)
					}

					Client.PostMessage(ev.Channel, slack.MsgOptionText(fmt.Sprintf("wahoo! your temporary 24-hour email address is %s@%s\n\ni'll post emails in this thread :arrow_down:", address, os.Getenv("DOMAIN")), false), slack.MsgOptionTS(ev.TimeStamp))

					email := db.Email{
						ID:        address,
						CreatedAt: time.Now(),
						ExpiresAt: time.Now().Add(24 * time.Hour),
						Timestamp: ev.TimeStamp,
						User:      ev.User,
					}

					db.DB.Create(&email)
				}
			}
		}
	})

	log.Println("Starting up HTTP server...")

	// Spin up the Slack app's HTTP server in the main goroutine
	err := http.ListenAndServe(":3001", nil)
	if err != nil {
		log.Fatal(err)
	}
}
