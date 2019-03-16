package main

// User interface
// All messaging plattforms need to implement this
type User interface {
	sendMessage(msg *genericMessage) error
	addToAlertGroup(group string) error
	delFromAlertGroup(group string) error
	getUserinfo() *Userinfo
}

// Userinfo that can be re-used by User implementations
type Userinfo struct {
	// Path used inside of the messaging protocols to reach this user
	MessagePath string
	// username for internal usage
	Username string
	// username used to speak to the user
	FriendlyName string
}

// HangoutsUser implements User for Hangouts Chat
type HangoutsUser struct {
	*Userinfo
}

type genericMessage struct {
	HeaderText, ContentText, FooterText string
	HeaderPictureURL                    string
	// Path to the group chat / DM this message belongs to
	MessagePath string
	// Thread in the chat
	Thread  string
	Sender  User
	Buttons []*genericButton
}

type genericButton struct {
	// Text that should be displayed "next" to the button
	HeaderText, ContentText, FooterText string
	// Text that should be displayed on the button
	ButtonText string
	// Picture that should be shown on the button (instead of text)
	PictureURL string
	// URL that should be opened on click (instead of callback)
	OnClickLink string
	// function we want to call when the button is clicked
	CallbackFunction string
	// Additional infos we want to pass to the callback as key-value
	CallbackInfos map[string]string
}
