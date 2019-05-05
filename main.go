package main

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	twitch "github.com/gempir/go-twitch-irc"
	_ "github.com/mattn/go-sqlite3"
)

const CREATE_CHEERS_SQL = `
	CREATE TABLE cheers (
		created_at integer,
		amount integer,
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
	BitsCheered  int
}

type ChannelInfo struct {
	ActiveSubs  int
	TotalGifts  int
	TotalCheers int
}

func (c ChannelInfo) Treat() string {
	return treats[c.TotalCheers%len(treats)]
}

type LoyaltyTracker struct {
	db *sql.DB
}

func NewLoyaltyTracker() *LoyaltyTracker {
	home, err := os.UserHomeDir()
	if err != nil {
		log.Fatal(err)
	}
	db, err := sql.Open("sqlite3", home+"/sql.db")
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
	} else {
		if tSub > tNow-60*60*24*30 {
			return fmt.Errorf("user is already subscribed")
		}
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
	ui.BitsCheered = lt.Cheers(user)
	return ui
}

func (lt *LoyaltyTracker) ChannelInfo() (ci ChannelInfo) {
	ci.TotalCheers = lt.TotalCheers()
	ci.TotalGifts = lt.TotalGifts()
	ci.ActiveSubs = lt.ActiveSubs()
	return ci
}

func (lt *LoyaltyTracker) Months(user string) int {
	row := lt.db.QueryRow("SELECT COUNT(*) FROM subs WHERE username = ?", user)
	var count sql.NullInt64
	err := row.Scan(&count)
	if err != nil {
		log.Println(err.Error())
		return 0
	}
	if count.Valid {
		return int(count.Int64)
	}
	return 0
}

func (lt *LoyaltyTracker) Cheers(user string) int {
	row := lt.db.QueryRow("SELECT SUM(amount) FROM cheers WHERE username = ?", user)
	var count sql.NullInt64
	err := row.Scan(&count)
	if err != nil {
		log.Println(err.Error())
		return 0
	}
	if count.Valid {
		return int(count.Int64)
	}
	return 0
}

func (lt *LoyaltyTracker) TotalCheers() int {
	row := lt.db.QueryRow("SELECT SUM(amount) FROM cheers")
	var count sql.NullInt64
	err := row.Scan(&count)
	if err != nil {
		log.Println(err.Error())
		return 0
	}
	if count.Valid {
		return int(count.Int64)
	}
	return 0
}

func (lt *LoyaltyTracker) TotalGifts() int {
	row := lt.db.QueryRow("SELECT COUNT(*) FROM subs WHERE giftee IS NOT NULL")
	var count sql.NullInt64
	err := row.Scan(&count)
	if err != nil {
		log.Println(err.Error())
		return 0
	}
	if count.Valid {
		return int(count.Int64)
	}
	return 0
}

func (lt *LoyaltyTracker) ActiveSubs() int {
	tNow := time.Now().Unix() - 60*60*24*30
	row := lt.db.QueryRow("SELECT COUNT(*) FROM subs WHERE created_at > ?", tNow)
	var count sql.NullInt64
	err := row.Scan(&count)
	if err != nil {
		log.Println(err.Error())
		return 0
	}
	if count.Valid {
		return int(count.Int64)
	}
	return 0
}

func (lt *LoyaltyTracker) LastSub(user string) time.Time {
	row := lt.db.QueryRow("SELECT created_at FROM subs WHERE username = ? ORDER BY created_at DESC", user)
	var tSub sql.NullInt64
	err := row.Scan(&tSub)
	if err != nil {
		log.Println(err.Error())
		return time.Now()
	}
	if tSub.Valid {
		return time.Unix(tSub.Int64, 0)
	}
	return time.Time{}
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
	var count sql.NullInt64
	err := row.Scan(&count)
	if err != nil {
		log.Println(err.Error())
		return 0
	}
	if count.Valid {
		return int(count.Int64)
	}
	return 0
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
	tNow := int(time.Now().Unix())
	_, err := lt.db.Exec("INSERT INTO cheers (created_at, username, amount) VALUES (?,?,?)", tNow, user, amount)
	return err
}

type LoyaltyRepo interface {
	Subscribe(user string) error
	Gift(user string, from string) error
	Cheer(user string, amount int) error
	UserInfo(user string) UserInfo
	ChannelInfo() ChannelInfo
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
		time.Sleep(4 * time.Second)
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
	return fmt.Sprintf("Thank you %s for the sub! You can now use our emotes: SeemsGood VoHiYo 4Head GivePLZ Kappa MingLee TableHere #IfYouWant!", message.User.DisplayName)
}

func (cm *ChatMonitor) AboutMe(message twitch.PrivateMessage) string {
	info := cm.LoyaltyRepo.UserInfo(message.User.Name)
	subbed := time.Since(info.LastSub) < 1*time.Hour*24*30
	parts := make([]string, 0)
	if !subbed {
		parts = append(parts, "are not currently subscribed")
	} else {
		parts = append(parts, fmt.Sprintf("have been subscribed for %d months, most recently %s ago", info.MonthsSubbed, time.Since(info.LastSub).Round(time.Hour)))
	}

	if info.GiftsGiven == 0 {
		parts = append(parts, "have given 0 gift subs to the community")
	} else {
		parts = append(parts, fmt.Sprintf("have given %d gift subs", info.GiftsGiven))
	}

	if info.SubbedFrom != nil {
		parts = append(parts, fmt.Sprintf("last received a gift sub from %s", *info.SubbedFrom))
	}

	if info.BitsCheered == 0 {
		parts = append(parts, "have not cheered")
	} else {
		parts = append(parts, fmt.Sprintf("have cheered %d bits", info.BitsCheered))
	}

	return fmt.Sprintf("%s, you: %s.", message.User.DisplayName, strings.Join(parts, "; "))
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
	return fmt.Sprintf("Thank you %s for the gift sub to %s! They can now use SeemsGood VoHiYo 4Head GivePLZ Kappa MingLee TableHere! You have given %d gift subs to this channel.", message.User.DisplayName, *arg, count)
}

func (cm *ChatMonitor) Stats() string {
	ci := cm.ChannelInfo()
	return fmt.Sprintf("There are currently %d active subscribers! The community has given %d gift subs and cheered %d bits!", ci.ActiveSubs, ci.TotalGifts, ci.TotalCheers)
}

func (cm *ChatMonitor) Cheer(message twitch.PrivateMessage) string {

	arg := GetArgument(0, message)
	if arg == nil {
		cmd := GetCommand(message)
		if strings.HasPrefix(cmd, "cheer") {
			part := strings.TrimPrefix(cmd, "cheer")
			arg = &part
		} else {
			return "To cheer, type !cheer <amount>, or Cheer100"
		}
	}
	amount, err := strconv.Atoi(*arg)
	if err != nil {
		return fmt.Sprintf("%s, you must cheer a number.", message.User.DisplayName)
	}
	if amount < 0 {
		return fmt.Sprintf("%s, stop trying to steal my bits! :(", message.User.DisplayName)
	}
	if amount > 1000000 {
		return fmt.Sprintf("%s, I can't allow you to be so generous! GivePLZ", message.User.DisplayName)
	}
	if err := cm.LoyaltyRepo.Cheer(message.User.Name, amount); err != nil {
		log.Println("err cheering:", err.Error())
		return fmt.Sprintf("%s, your cheer failed because `%s`", message.User.DisplayName, err.Error())
	}
	userInfo := cm.UserInfo(message.User.Name)
	info := cm.ChannelInfo()
	return fmt.Sprintf("%s, thanks for cheering %d bits, for a total of %d! The community has given %d bits, enough for a new %s!", message.User.DisplayName, amount, userInfo.BitsCheered, info.TotalCheers, info.Treat())
}

func (cm *ChatMonitor) NewMessage(message twitch.PrivateMessage) {
	cmd := GetCommand(message)
	switch cmd {
	case "giftsub":
		cm.Say(cm.GiftSub(message))
		return
	case "sub":
		cm.Say(cm.Subscribe(message))
		return
	case "me":
		cm.Say(cm.AboutMe(message))
		return
	case "cheer":
		cm.Say(cm.Cheer(message))
		return
	case "stats":
		cm.Say(cm.Stats())
		return
	}

	if strings.HasPrefix(cmd, "cheer") {
		cm.Say(cm.Cheer(message))
		return
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

var treats = [...]string{"teddy bear", "hot choccy", "blanket", "desk plant", "wii u", "copy of mario maker", "rune scim", "egg salad", "buzzy beetle", "mazarati", "golden kappa", "time machine"}
