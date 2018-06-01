package main

import (
	nsq "github.com/nsqio/go-nsq"
	mgo "gopkg.in/mgo.v2"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

var db *mgo.Session

// mongoDBに接続
func dialdb() error {
	var err error
	log.Println("MongoDBにダイヤル中: localhost")
	db, err = mgo.Dial("localhost")
	return err
}

// mongoDBとの接続を閉じる
func closedb() {
	db.Close()
	log.Println("データベース接続が閉じられました")
}

// 投票の選択肢を表す構造体
type poll struct {
	Options []string
}

// 選択肢のロード
func loadOptions() ([]string, error) {
	var options []string

	// DBはそのまま、Cはテーブルの名前
	// Find(nil)はフィルタリングを行わないという意味
	// iteratorを使うことで、メモリの使用量が大幅に減る
	iter := db.DB("ballots").C("polls").Find(nil).Iter()
	var p poll

	// iterの次の値をpに閉じ込める
	for iter.Next(&p) {
		// ...をつけることでスライス中のそれぞれの項目を個別の引数として渡せる
		options = append(options, p.Options...)
	}
	iter.Close()
	return options, iter.Err()
}

func publishVotes(votes <-chan string) <-chan struct{} {
	stopchan := make(chan struct{}, 1)
	pub, _ := nsq.NewProducer("localhost:4150", nsq.NewConfig())
	go func() {
		// range chanはchanが閉じられるまで、ループし続ける
		// もしchanが空なら、ブロックする
		for vote := range votes {
			pub.Publish("votes", []byte(vote))
		}
		log.Println("Publisher: 停止中です")
		pub.Stop()
		log.Println("Publisher: 停止しました")
		// goroutineの終了を知らせる
		stopchan <- struct{}{}
	}()
	return stopchan
}

func main() {
	var stoplock sync.Mutex
	stop := false
	stopChan := make(chan struct{}, 1)
	signalChan := make(chan os.Signal, 1)
	go func() {
		// signalChanから入力が来るまで待機
		<-signalChan
		stoplock.Lock()
		stop = true
		stoplock.Unlock()
		log.Println("停止します...")
		stopChan <- struct{}{}
		closeConn()
	}()
	// 狩猟用のコマンドが来たらsignalChanに送るように設定
	signal.Notify(signalChan, syscall.SIGINT, syscall.SIGTERM)

	if err := dialdb(); err != nil {
		log.Fatalln("MongoDBへのダイヤルに失敗しました:", err)
	}
	defer closedb()

	votes := make(chan string)
	publisherStoppedChan := publishVotes(votes)
	twitterStoppedChan := startTwitterStream(stopChan, votes)
	go func() {
		for {
			time.Sleep(1 * time.Minute)
			// Twitterとの接続を閉じる
			closeConn()
			stoplock.Lock()
			if stop {
				stoplock.Unlock()
				break
			}
			stoplock.Unlock()
		}
	}()
	<-twitterStoppedChan
	close(votes)
	<-publisherStoppedChan
}
