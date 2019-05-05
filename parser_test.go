package main

import (
	"testing"

	twitch "github.com/gempir/go-twitch-irc"
)

func TestCheers(t *testing.T) {
	lp := new(MockLoyaltyRepo)
	parser := NewChatMonitor(lp)
	msg := twitch.PrivateMessage{
		Message: "Cheer10 bday10",
		User: twitch.User{
			DisplayName: "test",
			Name:        "test",
		},
	}

	lp.On("Cheer", "test", 20).Return(nil)
	lp.On("UserInfo", "test").Return(UserInfo{})
	lp.On("ChannelInfo").Return(ChannelInfo{})
	go func() {
		<-parser.messages
	}()
	parser.NewMessage(msg)
	lp.AssertExpectations(t)
}
