package main

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"time"

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
			if err := tx.Rollback(); err != nil {
				return err
			}
			return err
		}
	} else {
		if tSub > tNow-60*60*24*30 {
			if err := tx.Rollback(); err != nil {
				return err
			}
			return fmt.Errorf("user is already subscribed")
		}
	}

	{
		_, err := tx.Exec("INSERT INTO subs ( created_at, username, tier ) VALUES (?,?,?)",
			tNow, user, 1)
		if err != nil {
			if err := tx.Rollback(); err != nil {
				return err
			}
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
			if err := tx.Rollback(); err != nil {
				return err
			}
			return err
		}
	}
	if tSub > tNow-60*60*24*30 {
		if err := tx.Rollback(); err != nil {
			return err
		}
		return fmt.Errorf("user is already subscribed")
	}
	{
		_, err := tx.Exec("INSERT INTO subs ( created_at, username, giftee, tier ) VALUES (?,?,?,?)",
			tNow, user, from, 1)
		if err != nil {
			if err := tx.Rollback(); err != nil {
				return err
			}
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
