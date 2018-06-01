このアプリは「Go言語によるWebアプリケーション開発」の本のアプリを作成したものです。

## 使い方

### TwitterAPIの登録

https://apps.twitter.com/
上のサイトからAppを登録し、KEYを取得する。

### KEYを環境変数に登録する

```bash
export SP_TWITTER_KEY=<% 取得したキー %>
export SP_TWITTER_SECRET=<% 取得したキー %>
export SP_TWITTER_ACCESSTOKEN=<% 取得したキー %>
export SP_TWITTER_ACCESSSECRET=<% 取得したキー %>
```

### プログラムを起動する

```
// terminal1
$ mongod --dbpath ./db
// terminal2
$ nsqd --lookupd-tcp-address=127.0.0.1:4160

// terminal3
$ nsqlookupd

// terminal4
$ cd twittervotes
$ go build -o twittervotes
$ ./twittervotes

// terminal5
$ cd counter
$ go build -o counter
$ ./counter

// terminal6
$ cd api
$ go build -o api
$ ./api

// terminal7
$ cd web
$ go build -o web
$ ./web
```
