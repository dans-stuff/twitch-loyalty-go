package main

import (
	"log"

	_ "github.com/mattn/go-sqlite3"
)

func main() {
	lt := NewLoyaltyTracker()
	cm := NewChatMonitor(lt)
	err := cm.Monitor()
	if err != nil {
		log.Println("error monitoring:", err.Error())
	}
}
