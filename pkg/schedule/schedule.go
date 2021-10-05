package schedule

import (
	"fmt"
	"time"

	"github.com/cjdenio/temp-email/pkg/db"
	"github.com/cjdenio/temp-email/pkg/slackevents"
	"github.com/go-co-op/gocron"
	"github.com/slack-go/slack"
)

func Start() {
	scheduler := gocron.NewScheduler(time.UTC)

	scheduler.Every(30).Minutes().Tag("expiry notification").Do(func() {
		fmt.Println("Checking for expired addresses...")

		var emails []db.Email
		tx := db.DB.Where("expires_at < NOW() AND NOT expired_message_sent").Find(&emails)
		if tx.Error != nil {
			fmt.Println(tx.Error)
		}

		fmt.Println(len(emails))

		for _, e := range emails {
			slackevents.Client.PostMessage("C02GK2TVAVB", slack.MsgOptionText(":x: :clock1: it's been 24 hours, so this address will no longer receive mail.", false), slack.MsgOptionTS(e.Timestamp))
			slackevents.Client.AddReaction("clock1", slack.ItemRef{
				Channel:   "C02GK2TVAVB",
				Timestamp: e.Timestamp,
			})

			e.ExpiredMessageSent = true
			db.DB.Save(&e)
		}
	})

	scheduler.StartAsync()
}
