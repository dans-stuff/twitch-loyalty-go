package main

import (
	"database/sql"
	"fmt"
	"log"
	"os"

	twitch "github.com/gempir/go-twitch-irc"
	_ "github.com/mattn/go-sqlite3"
)

const CREATE_SQL = `
	CREATE TABLE users (
		username text, channel text,
		last_sub integer, total_subs integer,
		gift_count integer, sub_count integer,
		cheer_count integer
	)
`

type LoyaltyTracker struct {
	db *sql.DB
}

func NewLoyaltyTracker() *LoyaltyTracker {
	db, err := sql.Open("sqlite3", "./sql.db")
	if err != nil {
		log.Fatal(err)
	}

	{
		_, err := db.Exec(CREATE_SQL)
		if err != nil {
			log.Println("error creating table:", err.Error())
		}
	}

	lt := new(LoyaltyTracker)
	lt.db = db
	return lt
}

type LoyaltyRepo interface {
}

type ChatMonitor struct {
	LoyaltyRepo
	*twitch.Client
}

func NewChatMonitor(lp LoyaltyRepo) *ChatMonitor {
	cm := &ChatMonitor{LoyaltyRepo: lp}
	return cm
}

func (cm *ChatMonitor) Monitor() error {
	token := os.Getenv("USER_OAUTH_TOKEN")
	name := os.Getenv("USER_NAME")

	if len(token) == 0 {
		return fmt.Errorf("error, USER_OAUTH_TOKEN variable empty")
	}

	if len(name) == 0 {
		return fmt.Errorf("error, USER_NAME variable empty")
	}

	client := twitch.NewClient(name, token)
	cm.Client = client
	client.OnConnect(func() { log.Println("connected!") })

	channel := os.Getenv("USER_CHANNEL")
	if len(channel) == 0 {
		return fmt.Errorf("error, USER_CHANNEL variable empty")
	}
	cm.Join(channel)
	cm.OnPrivateMessage(cm.NewMessage)

	return client.Connect()
}

func (cm *ChatMonitor) NewMessage(message twitch.PrivateMessage) {
	fmt.Println(message.User.Name, ":", message.Message)
}

func main() {
	lt := NewLoyaltyTracker()
	cm := NewChatMonitor(lt)
	err := cm.Monitor()
	if err != nil {
		log.Println("error monitoring:", err.Error())
	}
}
