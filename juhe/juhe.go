package juhe

import (
	"bytes"
	"io"
	"log"

	"html/template"

	simplejson "github.com/bitly/go-simplejson"

	aliyun "../aliyun"
	util "../util"
	"github.com/PuerkitoBio/goquery"
)

const (
	baseurl = "http://v.juhe.cn/toutiao/index"
	appkey  = "ab30c7f450f8322c1e1be4efe2e3d084"
)

//News information for news
type News struct {
	Title, Date, Author, URL, Md5 string
	Pics                          [3]string
	Stype                         int
}

//Page page info
type Page struct {
	Title   string
	Content template.HTML
}

func getTypeStr(stype int) string {
	switch stype {
	case 1:
		return "shehui"
	case 2:
		return "guonei"
	case 3:
		return "guoji"
	case 4:
		return "yule"
	case 5:
		return "tiyu"
	case 6:
		return "junshi"
	case 7:
		return "keji"
	case 8:
		return "caijing"
	case 9:
		return "shishang"
	default:
		return "top"
	}

}

func initTemplate() *template.Template {
	tpl, err := template.ParseFiles("templates/news.html")
	if err != nil {
		panic("parse template failed")
	}
	return tpl
}

//GetNews fetch news
func GetNews(stype int) []News {
	tpl := initTemplate()
	news := make([]News, 50)
	typeStr := getTypeStr(stype)
	url := baseurl + "?type=" + typeStr + "&key=" + appkey

	rspbody, err := util.HTTPRequest(url, "")
	if err != nil {
		log.Printf("HTTPRequest failed:%v", err)
		return nil
	}

	js, _ := simplejson.NewJson([]byte(`{}`))
	err = js.UnmarshalJSON([]byte(rspbody))
	if err != nil {
		log.Printf("parse rspbody failed:%v", err)
		return nil
	}

	errcode, err := js.Get("error_code").Int()
	if err != nil {
		log.Printf("get error code failed:%v", err)
		return nil
	}

	if errcode != 0 {
		log.Printf("get error code failed:%v", err)
		return nil
	}

	arr, err := js.Get("result").Get("data").Array()
	if err != nil {
		log.Printf("get data failed:%v", err)
		return nil
	}

	i := 0
	for ; i < len(arr); i++ {
		info := js.Get("result").Get("data").GetIndex(i)
		var ns News
		ns.Title, _ = info.Get("title").String()
		ns.Stype = stype
		ns.Md5 = util.GetMD5Hash(ns.Title)
		ns.Date, _ = info.Get("date").String()
		ns.URL, _ = info.Get("url").String()
		d, err := goquery.NewDocument(ns.URL)
		if err != nil {
			log.Printf("fetch url failed:%v", err)
			continue
		}

		pics, err := GetImages(d, ns.URL)
		if err != nil {
			log.Printf("fetch images from url failed:%v", err)
			ns.Pics[0], _ = info.Get("thumbnail_pic_s").String()
		} else {
			for i := 0; i < len(pics) && i < 3; i++ {
				ns.Pics[i] = pics[i]
			}
		}
		title := d.Find("title").Text()
		content, err := d.Find("article").Html()
		if err != nil {
			log.Printf("get article failed %s:%v", ns.URL, err)
			continue
		}
		var buf bytes.Buffer
		w := io.Writer(&buf)
		err = tpl.Execute(w, &Page{Title: title, Content: template.HTML(content)})
		filename := util.GenSalt()
		if flag := aliyun.UploadOssFile(filename, buf.String()); !flag {
			log.Printf("UploadOssFile failed %s:%v", filename, err)
			continue
		}
		ns.URL = aliyun.GenOssURL(filename)
		ns.Author, _ = info.Get("author_name").String()
		news[i] = ns
		log.Printf("title:%s", ns.Title)
	}

	return news[:i]
}

//GetImages extract images from url
func GetImages(d *goquery.Document, url string) ([]string, error) {
	var images []string
	sel := d.Find("a")
	sel.Each(func(i int, n *goquery.Selection) {
		if val, ok := n.Attr("class"); ok {
			if val == "img-wrap" {
				if href, ok := n.Attr("href"); ok {
					images = append(images, href)
				}
			}
		}
	})

	return images, nil
}