package main

import (
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	_ "github.com/mattn/go-sqlite3"
	"github.com/tucnak/telebot"
)

const (
	SELL = "SELL"
	BUY  = "BUY"
)

var (
	SETBUYERROR  = errors.New("請輸入要購買的數字")
	SETSELLERROR = errors.New("請輸入要賣出的數字")
	db           *sql.DB
	token        = flag.String("t", "", "input telegram bot token")
)

type Commander interface {
	Execute(text string) (string, error)
}

func SetBuyCommand(m telebot.Message) (msg string, err error) {
	cmd := "INSERT INTO e3gold(uid,type,price,date) VALUES (?,?,?,?)"
	tx, err := db.Begin()
	if err != nil {
		return
	}
	stmt, err := tx.Prepare(cmd)
	if err != nil {
		return
	}
	date := time.Now().Format("2006/01/02 15:04:05")

	price, err := strconv.Atoi(m.Text)
	if err != nil {
		log.Println(err)
		err = SETBUYERROR
		return
	}
	_, err = stmt.Exec(m.Sender.ID, BUY, price, date)
	if err != nil {
		tx.Rollback()
		stmt.Close()
		return
	}
	err = tx.Commit()
	if err != nil {
		tx.Rollback()
		stmt.Close()
		return
	}
	msg = "設定的買價為 "
	msg = msg + strconv.Itoa(price) + " 成功"
	defer stmt.Close()
	return
}

func SetSellCommand(m telebot.Message) (msg string, err error) {
	cmd := "INSERT INTO e3gold(uid,type,price,date) VALUES (?,?,?,?)"
	tx, err := db.Begin()
	if err != nil {
		return
	}
	stmt, err := tx.Prepare(cmd)
	if err != nil {
		return
	}
	date := time.Now().Format("2006/01/02 15:04:05")

	price, err := strconv.Atoi(m.Text)
	if err != nil {
		log.Println(err)
		err = SETSELLERROR
		return
	}
	_, err = stmt.Exec(m.Sender.ID, SELL, price, date)
	if err != nil {
		tx.Rollback()
		stmt.Close()
		return
	}
	err = tx.Commit()
	if err != nil {
		tx.Rollback()
		stmt.Close()
		return
	}
	msg = "設定的賣價為 "
	msg = msg + strconv.Itoa(price) + " 成功"
	defer stmt.Close()
	return
}

func HelpCommand(m telebot.Message) (msg string, err error) {
	msg = "\n 這是玉山銀行金價查詢機器人，您可以照著指令設定 \n /setbuy - 設定買入價格(達到此價格會訊息通知)\n /setsell - 設定賣出價格(達到此價格會訊息通知)\n /now - 查詢現有金價"
	return
}

func NowCommand(m telebot.Message) (msg string, err error) {
	r, err := queryE3()
	if err != nil {
		return
	}
	msg = fmt.Sprintf("玉山買進:%d \n 玉山賣出:%d", r["e3buy"], r["e3sell"])
	return
}

func init() {
	d, err := sql.Open("sqlite3", "./e3gold.sqlite3")
	if err != nil {
		return
	}

	sqlStmt := `
	create table if not exists e3gold (uid,type,price,date,UNIQUE (uid,type,price) ON CONFLICT REPLACE)
	`
	_, err = d.Exec(sqlStmt)
	if err != nil {
		panic(err)
	}
	db = d
}
func main() {
	flag.Parse()
	bot, err := telebot.NewBot(*token)
	if err != nil {
		log.Println(err)
		return
	}
	log.Println("Bot Start")
	go autoQuery(bot)

	messages := make(chan telebot.Message)
	bot.Listen(messages, 60*time.Second)
	queue := make(map[int]func(telebot.Message) (msg string, err error))
	for message := range messages {
		log.Println(message.Sender.ID, message.Sender.Username, message.Text)
		if v, ok := queue[message.Sender.ID]; ok {
			msg, err := v(message)
			if err != nil {
				bot.SendMessage(message.Chat, err.Error(), nil)
			} else {
				bot.SendMessage(message.Chat, msg, nil)
				delete(queue, message.Sender.ID)
			}
		} else {
			switch message.Text {
			case "/help":
				s, _ := HelpCommand(message)
				bot.SendMessage(message.Chat, s, nil)
				break
			case "/now":
				s, _ := NowCommand(message)
				bot.SendMessage(message.Chat, s, nil)
				break
			case "/setsell":
				queue[message.Sender.ID] = SetSellCommand
				bot.SendMessage(message.Chat, "請輸入想要賣出的數字", nil)
				break
			case "/setbuy":
				queue[message.Sender.ID] = SetBuyCommand
				bot.SendMessage(message.Chat, "請輸入想要買入的數字", nil)
				break
			}
		}

	}
}

type e3gold struct {
	uid   int
	t     string
	price int
}

func autoQuery(bot *telebot.Bot) {
	t := time.NewTicker(5 * time.Minute)
	for {
		select {
		case <-t.C:
			r, err := queryE3()
			if err != nil {
				log.Println(err)
				return
			}
			sql := "SELECT uid,type,price FROM `e3gold` WHERE (price >=? AND type = ?) OR (price <=? AND type = ?)"
			stmt, err := db.Prepare(sql)
			if err != nil {
				log.Println(err)
				return
			}

			a := make([]e3gold, 5)
			rows, err := stmt.Query(r["e3sell"], "BUY", r["e3buy"], "SELL")
			if err != nil {
				log.Println(err)
				return
			}
			for rows.Next() {
				e3 := e3gold{}
				err = rows.Scan(&e3.uid, &e3.t, &e3.price)
				if err != nil {
					log.Println(err)
					return
				}
				user := telebot.User{ID: e3.uid}
				if e3.t == "BUY" {
					bot.SendMessage(user, "您設定的買進價為:"+strconv.Itoa(e3.price)+" \n 玉山賣出價:"+strconv.Itoa(r["e3sell"]), nil)
				}
				if e3.t == "SELL" {
					bot.SendMessage(user, "您設定的賣出為:"+strconv.Itoa(e3.price)+"\n 玉山買進價:"+strconv.Itoa(r["e3buy"]), nil)
				}
				a = append(a, e3)

			}
			rows.Close()
			for _, e3 := range a {
				remove(e3.uid, e3.price, e3.t)
			}
			break
		}
	}

}
func queryE3() (r map[string]int, err error) {
	doc, err := goquery.NewDocument("http://www.esunbank.com.tw/info/goldpassbook.aspx")
	if err != nil {
		return
	}
	e3buy, err := strconv.Atoi(strings.Replace(doc.Find(".datatable").Eq(1).Find(".default-color7").Eq(1).Text(), ",", "", 1))
	e3sell, err := strconv.Atoi(strings.Replace(doc.Find(".datatable").Eq(1).Find(".default-color7").Eq(2).Text(), ",", "", 1))
	if err != nil {
		return
	}
	r = make(map[string]int)
	r["e3buy"] = e3buy
	r["e3sell"] = e3sell
	return
}
func remove(uid, price int, t string) (err error) {
	sql := "DELETE FROM `e3gold` where uid = ? AND price = ? AND type = ?"

	stmt, err := db.Prepare(sql)
	if err != nil {
		stmt.Close()
		return
	}

	_, err = stmt.Exec(uid, price, t)
	if err != nil {
		stmt.Close()
		return
	}
	defer stmt.Close()
	return

}
