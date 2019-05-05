package main

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	twitch "github.com/gempir/go-twitch-irc"
	_ "github.com/mattn/go-sqlite3"
)

const CREATE_CHEERS_SQL = `
	CREATE TABLE cheers (
		created_at integer,
		username text
	)
`

const CREATE_SUBS_SQL = `
	CREATE TABLE subs (
		created_at integer,
		username text,
		giftee text,
		tier integer
	);
`

type UserInfo struct {
	LastSub      time.Time
	SubbedFrom   *string
	MonthsSubbed int
	GiftsGiven   int
}

type LoyaltyTracker struct {
	db *sql.DB
}

func NewLoyaltyTracker() *LoyaltyTracker {
	db, err := sql.Open("sqlite3", "./sql.db")
	if err != nil {
		log.Fatal(err)
	}

	{
		_, err := db.Exec(CREATE_CHEERS_SQL)
		if err != nil {
			log.Println("error creating table:", err.Error())
		}
	}

	{
		_, err := db.Exec(CREATE_SUBS_SQL)
		if err != nil {
			log.Println("error creating table:", err.Error())
		}
	}

	lt := new(LoyaltyTracker)
	lt.db = db
	return lt
}
func (lt *LoyaltyTracker) Subscribe(user string) error {
	tx, err := lt.db.Begin()
	if err != nil {
		return err
	}

	tNow := int(time.Now().Unix())
	row := tx.QueryRow("SELECT created_at FROM subs WHERE username = ?", user)
	tSub := 0
	if err := row.Scan(&tSub); err != nil {
		if err != sql.ErrNoRows {
			return err
		}
	}
	if tSub > tNow-60*60*24*30 {
		return fmt.Errorf("user is already subscribed")
	}
	{
		_, err := tx.Exec("INSERT INTO subs ( created_at, username, tier ) VALUES (?,?,?)",
			tNow, user, 1)
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (lt *LoyaltyTracker) UserInfo(user string) (ui UserInfo) {
	ui.MonthsSubbed = lt.Months(user)
	ui.GiftsGiven = lt.GiftSubs(user)
	ui.SubbedFrom = lt.Giftee(user)
	ui.LastSub = lt.LastSub(user)
	return ui
}

func (lt *LoyaltyTracker) Months(user string) int {
	row := lt.db.QueryRow("SELECT COUNT(*) FROM subs WHERE username = ?", user)
	count := 0
	err := row.Scan(&count)
	if err != nil {
		log.Println(err.Error())
		return 0
	}
	return count
}

func (lt *LoyaltyTracker) LastSub(user string) time.Time {
	row := lt.db.QueryRow("SELECT created_at FROM subs WHERE username = ? ORDER BY created_at DESC", user)
	tSub := 0
	err := row.Scan(&tSub)
	if err != nil {
		log.Println(err.Error())
		return time.Now()
	}
	return time.Unix(int64(tSub), 0)
}

func (lt *LoyaltyTracker) Giftee(user string) *string {
	row := lt.db.QueryRow("SELECT giftee FROM subs WHERE username = ? ORDER BY created_at DESC", user)
	var name sql.NullString
	err := row.Scan(&name)
	if err != nil {
		log.Println(err.Error())
		return nil
	}
	if name.Valid {
		return &name.String
	}
	return nil
}

func (lt *LoyaltyTracker) GiftSubs(user string) int {
	row := lt.db.QueryRow("SELECT COUNT(*) FROM subs WHERE giftee = ?", user)
	count := 0
	err := row.Scan(&count)
	if err != nil {
		log.Println(err.Error())
		return 0
	}
	return count
}

func (lt *LoyaltyTracker) Gift(user, from string) error {
	tx, err := lt.db.Begin()
	if err != nil {
		return err
	}

	tNow := int(time.Now().Unix())
	row := tx.QueryRow("SELECT created_at FROM subs WHERE username = ?", user)
	tSub := 0
	if err := row.Scan(&tSub); err != nil {
		if err != sql.ErrNoRows {
			return err
		}
	}
	if tSub > tNow-60*60*24*30 {
		return fmt.Errorf("user is already subscribed")
	}
	{
		_, err := tx.Exec("INSERT INTO subs ( created_at, username, giftee, tier ) VALUES (?,?,?,?)",
			tNow, user, from, 1)
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (lt *LoyaltyTracker) Cheer(user string, amount int) error {
	return nil
}

type LoyaltyRepo interface {
	Subscribe(user string) error
	Gift(user string, from string) error
	Cheer(user string, amount int) error
	UserInfo(user string) UserInfo
}

type ChatMonitor struct {
	LoyaltyRepo
	*twitch.Client
	channel string

	messages chan string
}

func NewChatMonitor(lp LoyaltyRepo) *ChatMonitor {
	cm := &ChatMonitor{LoyaltyRepo: lp}
	cm.messages = make(chan string, 1000)
	return cm
}

func (cm *ChatMonitor) SaySlowly() {
	lastMessage := ""
	for m := range cm.messages {
		if lastMessage == m {
			continue
		}
		lastMessage = m
		cm.Client.Say(cm.channel, m)
		time.Sleep(2 * time.Second)
	}
}

func (cm *ChatMonitor) Say(s string) {
	cm.messages <- s
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
	cm.channel = channel
	cm.Join(channel)
	cm.OnPrivateMessage(cm.NewMessage)
	go cm.SaySlowly()

	return client.Connect()
}

func (cm *ChatMonitor) Subscribe(message twitch.PrivateMessage) string {
	if err := cm.LoyaltyRepo.Subscribe(message.User.Name); err != nil {
		log.Println("err sub:", err.Error())
		return fmt.Sprintf("%s, your sub failed because `%s`", message.User.DisplayName, err.Error())
	}
	return fmt.Sprintf("thank you %s for the sub!", message.User.DisplayName)
}

func (cm *ChatMonitor) AboutMe(message twitch.PrivateMessage) string {
	info := cm.LoyaltyRepo.UserInfo(message.User.Name)
	subbed := time.Since(info.LastSub) < 1*time.Hour*24*30
	parts := make([]string, 0)
	if subbed {
		parts = append(parts, "are not currently subscribed")
	} else {
		parts = append(parts, fmt.Sprintf("have been subscribed for %d months, most recently at %s", info.MonthsSubbed, info.LastSub))
	}

	if info.GiftsGiven == 0 {
		parts = append(parts, "have not given any gift subs")
	} else {
		parts = append(parts, fmt.Sprintf("have given %d gift subs", info.GiftsGiven))
	}

	if info.SubbedFrom != nil {
		parts = append(parts, fmt.Sprintf("last received a gift sub from %s", *info.SubbedFrom))
	}

	return fmt.Sprintf("%s, you: %s!", message.User.DisplayName, strings.Join(parts, "; "))
}

func (cm *ChatMonitor) GiftSub(message twitch.PrivateMessage) string {
	arg := GetArgument(0, message)
	if arg == nil {
		return "To gift sub, type !giftsub <username>"
	}
	if err := cm.LoyaltyRepo.Gift(*arg, message.User.Name); err != nil {
		log.Println("err giftsub:", err.Error())
		return fmt.Sprintf("%s, your giftsub failed because `%s`", message.User.DisplayName, err.Error())
	}
	count := cm.LoyaltyRepo.UserInfo(message.User.Name).GiftsGiven
	return fmt.Sprintf("Thank you %s for the gift sub to %s! You have given %d gift subs.", message.User.DisplayName, *arg, count)
}

func (cm *ChatMonitor) NewMessage(message twitch.PrivateMessage) {
	switch GetCommand(message) {
	case "giftsub":
		cm.Say(cm.GiftSub(message))
	case "sub":
		cm.Say(cm.Subscribe(message))
	case "me":
		cm.Say(cm.AboutMe(message))
	}
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

func GetCommand(message twitch.PrivateMessage) string {
	return strings.TrimPrefix(strings.ToLower(strings.Split(message.Message, " ")[0]), "!")
}

func GetArgument(n int, message twitch.PrivateMessage) *string {
	parts := strings.Split(message.Message, " ")
	if n+1 >= len(parts) {
		return nil
	}
	res := strings.TrimPrefix(strings.ToLower(parts[n+1]), "@")
	return &res
}
