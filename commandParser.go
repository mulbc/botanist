package main

import (
	"fmt"
	"strings"

	"github.com/sbstjn/allot"
)

var commandDescription map[string]func(allot.MatchInterface, User) (*genericMessage, error)
var commandList map[allot.Command]func(allot.MatchInterface, User) (*genericMessage, error)
var fortunes []string

func init() {
	commandDescription = map[string]func(allot.MatchInterface, User) (*genericMessage, error){
		"echo (.*)":             handleEcho,
		"welcome <user:string>": handleWelcome,
		"annoy me about <alertgroup:string> alerts":     handleAddToAlertGroup,
		"don't bug me about <alertgroup:string> alerts": handleDelFromAlertGroup,
	}
	commandList = make(map[allot.Command]func(allot.MatchInterface, User) (*genericMessage, error))
	for comm, handler := range commandDescription {
		newCommand := allot.New(comm)
		commandList[newCommand] = handler
	}
}

func handleRequest(incomingMessage *genericMessage) (*genericMessage, error) {
	log.Debugf("Incoming Message: %v#", incomingMessage)
	request := strings.TrimSpace(incomingMessage.ContentText)
	for cmd, handler := range commandList {
		match, err := cmd.Match(request)

		if err == nil {
			message, err := handler(match, incomingMessage.Sender)
			message.Thread = incomingMessage.Thread
			message.MessagePath = incomingMessage.MessagePath
			if err != nil {
				return message, err
			}
			return message, nil
		}
	}
	messageText := "Your message did not match any command.\nPossible case-sensitive commands are:\n"
	for key := range commandDescription {
		messageText += fmt.Sprintf(" %s\n", key)
	}
	return &genericMessage{ContentText: messageText, Thread: incomingMessage.Thread, MessagePath: incomingMessage.MessagePath}, nil
}

func handleEcho(match allot.MatchInterface, User User) (*genericMessage, error) {
	echo, err := match.Match(0)
	if err != nil {
		return &genericMessage{ContentText: "I had issues when parsing your words"}, err
	}
	return &genericMessage{ContentText: fmt.Sprintf("What you said: \"%s\"", echo)}, nil
}

func handleWelcome(match allot.MatchInterface, User User) (*genericMessage, error) {
	echo, err := match.String("user")
	if err != nil {
		return &genericMessage{ContentText: "I had issues identifying who to welcome"}, err
	}
	return &genericMessage{ContentText: fmt.Sprintf("Welcome %s - nice to meet you here :)", echo)}, nil
}

func handleAddToAlertGroup(match allot.MatchInterface, User User) (*genericMessage, error) {
	alertGroup, err := match.String("alertgroup")
	err = User.addToAlertGroup(alertGroup)
	return &genericMessage{ContentText: fmt.Sprintf("User %s added to alert group %s", User.getUserinfo().FriendlyName, alertGroup)}, err
}

func handleDelFromAlertGroup(match allot.MatchInterface, User User) (*genericMessage, error) {
	alertGroup, err := match.String("alertgroup")
	err = User.delFromAlertGroup(alertGroup)
	return &genericMessage{ContentText: fmt.Sprintf("User %s removed from alert group %s", User.getUserinfo().FriendlyName, alertGroup)}, err
}
