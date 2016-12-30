package main

import (
	"log"
	"os"
	"time"

	juhe "../juhe"

	util "../util"
	_ "github.com/go-sql-driver/mysql"
)

func main() {
	db, err := util.InitDB(false)
	if err != nil {
		log.Printf("InitDB failed:%v", err)
		os.Exit(1)
	}

	ts := time.Now().Unix()
	var i int64
	for {
		i++
		for j := 0; j < 2; j++ {
			jokes := juhe.GetJoke(ts, i, 20, int64(j))
			if len(jokes) == 0 {
				break
			}
			for j := 0; j < len(jokes); j++ {
				ns := jokes[j]
				_, err := db.Exec("INSERT IGNORE INTO joke(content, dst, md5, type, ctime, dtime) VALUES (?, ?, ?, ?, ?, NOW())",
					ns.Content, ns.URL, ns.Md5, j, ns.Date)
				if err != nil {
					log.Printf("insert failed:%v", err)
				}
			}
		}
		if i%1000 == 0 {
			time.Sleep(1)
		}
	}
}
