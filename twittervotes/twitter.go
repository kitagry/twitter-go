package main

import (
	"encoding/json"
	"github.com/garyburd/go-oauth/oauth"
	"github.com/joeshaw/envdecode"
	"github.com/joho/godotenv"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

var conn net.Conn

// envファイルを読み込む
func Env_load() {
	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file")
	}
}

// Twitterとの接続をつなぎ直す
func dial(netw, addr string) (net.Conn, error) {
	if conn != nil {
		conn.Close()
		conn = nil
	}
	netc, err := net.DialTimeout(netw, addr, 5*time.Second)
	if err != nil {
		return nil, err
	}
	conn = netc
	return netc, nil
}

var reader io.ReadCloser

// Twitterとの接続を閉じる
func closeConn() {
	if conn != nil {
		conn.Close()
	}
	if reader != nil {
		reader.Close()
	}
}

var (
	authClient *oauth.Client
	creds      *oauth.Credentials
)

func setupTwitterAuth() {
	Env_load()

	var ts struct {
		ConsumerKey    string `env:"SP_TWITTER_KEY,required"`
		ConsumerSecret string `env:"SP_TWITTER_SECRET,required"`
		AccessToken    string `env:"SP_TWITTER_ACCESSTOKEN,required"`
		AccessSecret   string `env:"SP_TWITTER_ACCESSSECRET,required"`
	}

	if err := envdecode.Decode(&ts); err != nil {
		log.Fatalln(err)
	}

	creds = &oauth.Credentials{
		Token:  ts.AccessToken,
		Secret: ts.AccessSecret,
	}

	authClient = &oauth.Client{
		Credentials: oauth.Credentials{
			Token:  ts.ConsumerKey,
			Secret: ts.ConsumerSecret,
		},
	}
}

var (
	authSetupOnce sync.Once
	httpClient    *http.Client
)

func makeReqest(req *http.Request, params url.Values) (*http.Response, error) {
	authSetupOnce.Do(func() {
		setupTwitterAuth()
		httpClient = &http.Client{
			Transport: &http.Transport{
				Dial: dial,
			},
		}
	})

	formEnc := params.Encode()
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Content-Length", strconv.Itoa(len(formEnc)))
	req.Header.Set("Authorization", authClient.AuthorizationHeader(creds, "POST", req.URL, params))
	return httpClient.Do(req)
}

// Tweetする内容
type tweet struct {
	Text string
}

func readFromTwitter(votes chan<- string) {
	// 選択肢の読み込み
	options, err := loadOptions()
	if err != nil {
		log.Println("選択肢の読み込みに失敗しました:", err)
		return
	}

	// URLオブジェクトの作成
	u, err := url.Parse("https://stream.twitter.com/1.1/statuses/filter.json")
	if err != nil {
		log.Println("URLの解析に失敗しました:", err)
		return
	}

	// URLに選択肢のクエリを追加する
	query := make(url.Values)
	query.Set("track", strings.Join(options, ","))

	// url.ValuesオブジェクトがエンコードされたものをPOSTする必要がある
	// あとでAPIの仕様を読もう
	req, err := http.NewRequest("POST", u.String(),
		strings.NewReader(query.Encode()))
	if err != nil {
		log.Println("検索のリクエストの作成に失敗しました:", err)
		return
	}

	// 上で作成したリクエストをクエリとともに送信する
	resp, err := makeReqest(req, query)
	if err != nil {
		log.Println("検索のリクエストに失敗しました:", err)
		return
	}

	// 返信内容をjsonでデコードする。
	reader = resp.Body
	decoder := json.NewDecoder(reader)
	for {
		var tweet tweet
		if err := decoder.Decode(&tweet); err != nil {
			break
		}
		// 1ツイートで複数の選択肢に投票することができる。
		for _, option := range options {
			if strings.Contains(strings.ToLower(tweet.Text), strings.ToLower(option)) {
				log.Println("投票:", option)
				// votesは chan<- string型で送信専用になっている
				votes <- option
			}
		}
	}
}

// stopchanによって終了が知らされる
// votesによって投票を行う(readFromTwitterを呼び出す)
func startTwitterStream(stopchan <-chan struct{},
	votes chan<- string) <-chan struct{} {
	stoppedchan := make(chan struct{}, 1)
	go func() {
		defer func() {
			// これによって、goroutineの終了を伝える
			stoppedchan <- struct{}{}
		}()

		for {
			select {
			case <-stopchan:
				log.Println("Twitterへの問い合わせを終了します...")
				return
			default:
				log.Println("Twitterに問い合わせします...")
				readFromTwitter(votes)
				log.Println(" (待機中)")
				time.Sleep(10 * time.Second)
			}
		}
	}()
	// このチャネルを使ってgoroutineの終了を伝える
	return stoppedchan
}
