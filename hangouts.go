package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"cloud.google.com/go/pubsub"
	"github.com/prometheus/alertmanager/template"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/chat/v1"
	"google.golang.org/api/option"
)

// HangoutsConfig specific configuration for Hangouts
// This stores the connection properties and the
// alertGroups to User mapping in Hangouts
type HangoutsConfig struct {
	// Path to credentials file
	CredentialsFile string `yaml:"credentialsFile,omitempty"`
	// Google Cloud Project name
	Project string `yaml:"project,omitempty"`
	// Pub/Sub subscription name configured for Hangouts Chat
	PsSubscription string `yaml:"psSubscripton,omitempty"`

	// Persistent config about who to "annoy" about Prometheus alerts
	PromAlertSubscribers map[string]map[string]HangoutsUser `yaml:"promAlertSubscribers,omitempty"`
}

var (
	ctx         context.Context
	cursorTimer = time.Time{}
	sms         *chat.SpacesMessagesService
)

func initHangouts() {
	log.Infoln("Initializing Hangouts backend")

	// This seems like a hack, but some of the oauth libraries expect an environment variable
	// if you use the JSON file, as opposed to being able to specify the path
	// as part of client creation.
	os.Setenv("GOOGLE_APPLICATION_CREDENTIALS", botanistConfig.Hangouts.CredentialsFile)
	ctx = context.Background()

	client, err := pubsub.NewClient(ctx, botanistConfig.Hangouts.Project, option.WithCredentialsFile(botanistConfig.Hangouts.CredentialsFile))

	if err != nil {
		log.Fatalf("error creating newclient: %v.\n", err)
	}

	sub := client.Subscription(botanistConfig.Hangouts.PsSubscription)

	httpClient, err := google.DefaultClient(oauth2.NoContext, "https://www.googleapis.com/auth/chat.bot")
	if err != nil {
		log.Fatalf("Error creating httpClient: %v.\n", err)
	}

	chatService, err := chat.New(httpClient)
	if err != nil {
		log.Fatalf("Error creating chatService: %v.\n", err)
	}

	sms = chat.NewSpacesMessagesService(chatService)

	cctx, cancel := context.WithCancel(ctx)
	defer cancel()

	ok, err := sub.Exists(ctx)
	if err != nil {
		log.Fatalf("Error checking if subscription exists. Err: %v.", err)
	}
	if !ok {
		log.Fatalln("Checked if subscription exists. It doesn't.")
	}

	var incomingMessage *chat.DeprecatedEvent

	err = sub.Receive(cctx, func(ctx context.Context, msg *pubsub.Message) {
		log.Debugf("Received Message %s.\n", string(msg.Data))
		msg.Ack()

		err := json.Unmarshal(msg.Data, &incomingMessage)
		if err != nil {
			log.Fatalf("Unable to decode Chat Message JSON: %v.\n", err)
		}

		responseMessage := reactToMessage(incomingMessage)
		if responseMessage == nil {
			return
		}

		log.Debugf("My Space: %#v.\n", incomingMessage.Space)
		response, err := sms.Create(incomingMessage.Space.Name, responseMessage).Do()
		if err != nil {
			log.Warnf("There was an error sending a response back to Hangouts Chat: %v.\n", err)
		}
		log.Debugf("Hangouts Response: %+v.\n", response)
	})
	if err != nil {
		log.Warnf("Error when receiving pubsub message: %v.\n", err)
	}

}

func reactToMessage(message *chat.DeprecatedEvent) *chat.Message {
	switch message.Type {
	case "ADDED_TO_SPACE":
		responseMessage := &chat.Message{Text: "Thanks for adding me"}
		if message.Message != nil {
			responseMessage.Thread = message.Message.Thread
			responseMessage.Space = message.Message.Space
		}
		return responseMessage
	case "MESSAGE":
		sender := HangoutsUser{
			&Userinfo{
				MessagePath:  message.Space.Name,
				Username:     message.User.Name,
				FriendlyName: message.User.DisplayName,
			},
		}
		genericMsg := genericMessage{
			Sender:      sender,
			ContentText: strings.TrimSpace(message.Message.ArgumentText),
			Thread:      message.Message.Thread.Name,
			MessagePath: message.Message.Space.Name,
		}
		response, _ := handleRequest(&genericMsg)
		hangoutsResponse, _ := genericToHangoutsMessage(response)
		return hangoutsResponse
	case "CARD_CLICKED":
		return handleClick(message)
	case "REMOVED_FROM_SPACE":
		// We should clean up the User's subscriptions here
		return nil
	}
	log.Warnf("Message type %s not implemented!", message.Type)
	return nil
}

func handleClick(message *chat.DeprecatedEvent) *chat.Message {
	if isOutdatedClick(message.EventTime) {
		return nil
	}

	log.Infof("User %s instructed me to execute %s", message.User.DisplayName, message.Action.ActionMethodName)
	var alertMgrAddress string
	commonLabels := make(template.KV)
	for _, param := range message.Action.Parameters {
		switch param.Key {
		case "labels":
			alertMgrAddress = param.Value
		case "alertMgrAddress":
			err := json.Unmarshal([]byte(param.Value), &commonLabels)
			if err != nil {
				log.Fatalf("Issues unmarshaling commonLabels: %s", err)
			}
		}
	}
	silenceWithLabels(commonLabels, message.User.DisplayName, alertMgrAddress)

	response := message.Message
	response.ActionResponse = &chat.ActionResponse{Type: "UPDATE_MESSAGE"}
	response.Cards[0].Header.Title = "SILENCED!"
	_, err := sms.Update(message.Message.Name, response).UpdateMask("cards").Do()
	if err != nil {
		return &chat.Message{Text: fmt.Sprintf("There was an error silencing this alert: \n %s", err)}
	}

	updateCursorTime(message.EventTime)
	log.Debugf("Sent message update: %#v", response)
	return &chat.Message{Text: fmt.Sprintf("%s silenced an alarm for an hour", message.User.DisplayName)}
}

func isOutdatedClick(eventTime string) bool {
	messageTimestamp, err := time.Parse(time.RFC3339Nano, eventTime)
	if err != nil {
		log.Warnln("Could not parse time when receiving message")
	}
	if !cursorTimer.IsZero() && !messageTimestamp.After(cursorTimer) {
		log.Infof("Old message received - ignoring %s", messageTimestamp)
		return true
	}
	log.Debugf("Time %s vs %s Zero: %t After: %t", messageTimestamp, cursorTimer, !cursorTimer.IsZero(), messageTimestamp.After(cursorTimer))
	return false
}

func updateCursorTime(eventTime string) {
	var err error
	cursorTimer, err = time.Parse(time.RFC3339Nano, eventTime)
	if err != nil {
		log.Warnln("Could not parse time when saving time")
	}
}

func genericToHangoutsMessage(msg *genericMessage) (*chat.Message, error) {
	if len(msg.Buttons) == 0 {
		// When there are no buttons, assume it is a regular text message
		return &chat.Message{Text: msg.ContentText}, nil
	}

	var sections []*chat.Section

	for _, button := range msg.Buttons {
		onClickEvent := &chat.OnClick{}
		if button.OnClickLink != "" {
			onClickEvent = &chat.OnClick{
				OpenLink: &chat.OpenLink{Url: button.OnClickLink},
			}
		} else {
			parameters := []*chat.ActionParameter{}
			for key, value := range button.CallbackInfos {
				param := &chat.ActionParameter{Key: key, Value: value}
				parameters = append(parameters, param)
			}
			onClickEvent = &chat.OnClick{Action: &chat.FormAction{
				ActionMethodName: button.CallbackFunction,
				Parameters:       parameters,
			}}
		}

		section := &chat.Section{
			Widgets: []*chat.WidgetMarkup{&chat.WidgetMarkup{KeyValue: &chat.KeyValue{
				TopLabel:    button.HeaderText,
				Content:     button.ContentText,
				BottomLabel: button.FooterText,
				Button: &chat.Button{
					TextButton: &chat.TextButton{
						Text:    button.ButtonText,
						OnClick: onClickEvent,
					}},
			}}},
		}
		sections = append(sections, section)
	}

	hangoutsMessage := &chat.Message{
		Cards: []*chat.Card{&chat.Card{
			Header: &chat.CardHeader{
				Title:    msg.HeaderText,
				Subtitle: msg.FooterText,
				// ImageURL needs to be public so that Chat can get it
				ImageUrl: msg.HeaderPictureURL,
			},
			Sections: sections,
		}}}
	return hangoutsMessage, nil
}

// We use a map[User]struct{} here to have a unique list of users
// that belong to the named group and the special group "all"
func getHangoutsUsersForAlertGroup(group string) map[User]struct{} {
	if len(botanistConfig.Hangouts.PromAlertSubscribers) == 0 {
		// no alertGroups defined yet
		return make(map[User]struct{})
	}
	alertGroup := botanistConfig.Hangouts.PromAlertSubscribers[group]
	allGroup := botanistConfig.Hangouts.PromAlertSubscribers["all"]
	if len(alertGroup) == 0 {
		// no receivers defined for our set alertGroup
		if len(allGroup) == 0 {
			// no receivers defined for all alerts
			return make(map[User]struct{})
		}
		// init empty map so that we can handle it easier below
		alertGroup = make(map[string]HangoutsUser)
	}
	userList := make(map[User]struct{})

	for _, user := range alertGroup {
		userList[user] = struct{}{}
	}
	for _, user := range allGroup {
		userList[user] = struct{}{}
	}

	return userList
}

func (hoUser HangoutsUser) sendMessage(msg *genericMessage) error {
	hangoutsMessage, err := genericToHangoutsMessage(msg)
	if err != nil {
		return err
	}

	_, err = sms.Create(hoUser.MessagePath, hangoutsMessage).Do()
	return err
}

func (hoUser HangoutsUser) addToAlertGroup(group string) error {
	if len(botanistConfig.Hangouts.PromAlertSubscribers) == 0 {
		botanistConfig.Hangouts.PromAlertSubscribers = make(map[string]map[string]HangoutsUser)
	}
	if len(botanistConfig.Hangouts.PromAlertSubscribers[group]) > 0 {
		botanistConfig.Hangouts.PromAlertSubscribers[group][hoUser.MessagePath] = hoUser
	} else {
		botanistConfig.Hangouts.PromAlertSubscribers[group] = map[string]HangoutsUser{hoUser.MessagePath: hoUser}
	}
	return persistConfigChanges()
}

func (hoUser HangoutsUser) delFromAlertGroup(group string) error {
	if len(botanistConfig.Hangouts.PromAlertSubscribers) == 0 ||
		len(botanistConfig.Hangouts.PromAlertSubscribers[group]) == 0 {
		return nil
	}
	delete(botanistConfig.Hangouts.PromAlertSubscribers[group], hoUser.MessagePath)
	log.Debugf("Current config \n %#v", botanistConfig.Hangouts)
	return persistConfigChanges()
}

func (hoUser HangoutsUser) getUserinfo() *Userinfo {
	return hoUser.Userinfo
}
