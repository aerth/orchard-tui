package main

import (
	"log"
	"log/slog"

	"github.com/aerth/mostly/mllm"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func opendb(dbfile string) {
	if dbfile == "" {
		return
	}
	slog.Info("opening db")
	db, err := gorm.Open(sqlite.Open(dbfile), &gorm.Config{})
	if err != nil {
		log.Fatalln(err)
	}
	global.gdb = db
	global.gdbx = gorm.G[mllm.ChatMessage](global.gdb)

	if err := db.AutoMigrate(&mllm.ChatMessage{}); err != nil {
		closedb()
		log.Fatalln(err)
	}
}

func closedb() {
	if global.gdb == nil {
		return
	}
	sqldb, _ := global.gdb.DB()
	if sqldb != nil {
		_ = sqldb.Close()
		println("closed db")
	}
}
