package main

import (
	"flag"
	"fmt"
	"github.com/nsqio/go-nsq"
	mgo "gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

const updateDuration = 1 * time.Second

var fatalErr error

func fatal(e error) {
	fmt.Println(e)
	flag.PrintDefaults()
	fatalErr = e
}

func main() {
	var countsLock sync.Mutex
	var counts map[string]int

	defer func() {
		if fatalErr != nil {
			os.Exit(1)
		}
	}()

	log.Println("NSQに接続します...")
	q, err := nsq.NewConsumer("votes", "counter", nsq.NewConfig())
	if err != nil {
		fatal(err)
		return
	}

	// メッセージでvotesに関するものが来るたびにcountsを足していく
	q.AddHandler(nsq.HandlerFunc(func(m *nsq.Message) error {
		countsLock.Lock()
		defer countsLock.Unlock()
		if counts == nil {
			counts = make(map[string]int)
		}
		vote := string(m.Body)
		counts[vote]++
		return nil
	}))

	if err := q.ConnectToNSQLookupd("localhost:4161"); err != nil {
		fatal(err)
		return
	}

	log.Println("データベースに接続します...")
	db, err := mgo.Dial("localhost")
	if err != nil {
		fatal(err)
		return
	}

	// main関数の最後に実行される
	defer func() {
		log.Println("データベース接続を閉じます...")
		db.Close()
	}()
	pollData := db.DB("ballots").C("polls")

	log.Println("NSQ上での投票を待機します...")
	var updater *time.Timer

	// updateDuration後にfuncを実行
	updater = time.AfterFunc(updateDuration, func() {
		countsLock.Lock()
		defer countsLock.Unlock()
		if len(counts) == 0 {
			log.Println("新しい投票はありません。データベースの更新をスキップします")
		} else {
			log.Println("データベースを更新します....")
			log.Println(counts)
			ok := true
			for option, count := range counts {
				sel := bson.M{"options": bson.M{"$in": []string{option}}}
				up := bson.M{"$inc": bson.M{"results." + option: count}}
				if _, err := pollData.UpdateAll(sel, up); err != nil {
					log.Println("更新に失敗しました:", err)
					ok = false
					continue
				}
				counts[option] = 0
			}
			if ok {
				log.Println("データベースの更新が完了しました")
				counts = nil
			}
		}
		updater.Reset(updateDuration)
	})

	termChan := make(chan os.Signal, 1)
	signal.Notify(termChan, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
	for {
		select {
		case <-termChan:
			updater.Stop()
			q.Stop()
		case <-q.StopChan:
			// 完了しました
			return
		}
	}
}
