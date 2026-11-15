//go:build cgo

package main

import (
	"log"
	"log/slog"
	"time"

	"github.com/aerth/mostly/mllm"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func opendb(dbfile string) {
	if dbfile == "" {
		return
	}
	db, err := gorm.Open(sqlite.Open(dbfile), &gorm.Config{})
	if err != nil {
		slog.Error("opening db failed, will proceed with no persistence", "err", err)
		time.Sleep(time.Second * 2)
		return
	}
	slog.Info("opened db...", "thread", mllm.DefaultThread.String())
	global.gdb = db
	global.gdbx = gorm.G[mllm.ChatMessage](global.gdb)

	if err := db.AutoMigrate(&mllm.ChatMessage{}); err != nil {
		slog.Error("db migrations failed", "err", err)
		closedb()
		log.Fatalln("update to latest version and try again")
	}
}

func closedb() {
	if global.gdb == nil {
		return
	}
	sqldb, _ := global.gdb.DB()
	if sqldb != nil {
		if err := sqldb.Close(); err != nil {
			slog.Error("closing db", "err", err)
		} else {
			slog.Info("closed db")
			println("closed db")
		}
	}
}
