package main

import (
	"database/sql"
	"errors"
	"fmt"
	"log"
	"net"
	"strconv"
	"strings"
	"time"

	"../../util"

	common "../../proto/common"
	modify "../../proto/modify"
	_ "github.com/go-sql-driver/mysql"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
)

const (
	feedInterval = 60
)

type server struct{}

var db *sql.DB

func (s *server) ReviewNews(ctx context.Context, in *modify.NewsRequest) (*common.CommReply, error) {
	if in.Reject {
		db.Exec("UPDATE news SET review = 1, deleted = 1, rtime = NOW(), ruid = ? WHERE id = ?", in.Head.Uid, in.Id)
	} else {
		query := "UPDATE news SET review = 1, rtime = NOW(), ruid = " + strconv.Itoa(int(in.Head.Uid))
		if in.Modify && in.Title != "" {
			query += ", title = '" + in.Title + "' "
		}
		query += " WHERE id = " + strconv.Itoa(int(in.Id))
		db.Exec(query)
		if len(in.Tags) > 0 {
			for i := 0; i < len(in.Tags); i++ {
				db.Exec("INSERT INTO news_tags(nid, tid, ruid, ctime) VALUES (?, ?, ?, NOW())", in.Id, in.Tags[i], in.Head.Uid)
			}
		}
	}

	return &common.CommReply{Head: &common.Head{Retcode: 0, Uid: in.Head.Uid}}, nil
}

func (s *server) ReviewVideo(ctx context.Context, in *modify.VideoRequest) (*common.CommReply, error) {
	if in.Reject {
		db.Exec("UPDATE youku_video SET review = 1, deleted = 1, rtime = NOW(), ruid = ? WHERE vid = ?", in.Head.Uid, in.Id)
	} else {
		query := "UPDATE youku_video SET review = 1, rtime = NOW(), ruid = " + strconv.Itoa(int(in.Head.Uid))
		if in.Modify && in.Title != "" {
			query += ", title = '" + in.Title + "' "
		}
		query += " WHERE vid = " + strconv.Itoa(int(in.Id))
		db.Exec(query)
	}

	return &common.CommReply{Head: &common.Head{Retcode: 0, Uid: in.Head.Uid}}, nil
}

func (s *server) AddTemplate(ctx context.Context, in *modify.AddTempRequest) (*common.CommReply, error) {
	res, err := db.Exec("INSERT INTO template(title, content, ruid, ctime, mtime) VALUES (?, ?, ?, NOW(), NOW())",
		in.Info.Title, in.Info.Content, in.Head.Uid)
	if err != nil {
		log.Printf("query failed:%v", err)
		return &common.CommReply{Head: &common.Head{Retcode: 1}}, err
	}

	id, err := res.LastInsertId()
	if err != nil {
		log.Printf("query failed:%v", err)
		return &common.CommReply{Head: &common.Head{Retcode: 1}}, err
	}

	return &common.CommReply{Head: &common.Head{Retcode: 0, Uid: in.Head.Uid}, Id: id}, nil
}

func (s *server) AddWifi(ctx context.Context, in *modify.WifiRequest) (*common.CommReply, error) {
	_, err := db.Exec("INSERT INTO wifi(ssid, password, longitude, latitude, uid, ctime) VALUES (?, ?, ?, ?,?, NOW())",
		in.Info.Ssid, in.Info.Password, in.Info.Longitude, in.Info.Latitude, in.Head.Uid)
	if err != nil {
		log.Printf("query failed:%v", err)
		return &common.CommReply{Head: &common.Head{Retcode: 1}}, err
	}

	return &common.CommReply{Head: &common.Head{Retcode: 0, Uid: in.Head.Uid}}, nil
}

func (s *server) ModTemplate(ctx context.Context, in *modify.ModTempRequest) (*common.CommReply, error) {
	query := "UPDATE template SET "
	if in.Info.Title != "" {
		query += " title = '" + in.Info.Title + "', "
	}
	if in.Info.Content != "" {
		query += " content = '" + in.Info.Content + "', "
	}
	online := 0
	if in.Info.Online {
		online = 1
	}
	query += " mtime = NOW(), ruid = " + strconv.Itoa(int(in.Head.Uid)) + ", online = " + strconv.Itoa(online) + " WHERE id = " + strconv.Itoa(int(in.Info.Id))
	_, err := db.Exec(query)

	if err != nil {
		log.Printf("query failed:%v", err)
		return &common.CommReply{Head: &common.Head{Retcode: 1}}, err
	}

	return &common.CommReply{Head: &common.Head{Retcode: 0, Uid: in.Head.Uid}}, nil
}

func (s *server) ReportClick(ctx context.Context, in *modify.ClickRequest) (*common.CommReply, error) {
	log.Printf("ReportClick uid:%d type:%d id:%d", in.Head.Uid, in.Type, in.Id)
	var res sql.Result
	var err error
	if in.Type != 4 {
		res, err = db.Exec("INSERT IGNORE INTO click_record(uid, type, id, ctime) VALUES(?, ?, ?, NOW())", in.Head.Uid, in.Type, in.Id)
	} else {
		res, err = db.Exec("INSERT INTO service_click_record(uid, sid, ctime) VALUES(?, ?, NOW())", in.Head.Uid, in.Id)
	}
	if err != nil {
		log.Printf("query failed:%v", err)
		return &common.CommReply{Head: &common.Head{Retcode: 1}}, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		log.Printf("get last insert id failed:%v", err)
		return &common.CommReply{Head: &common.Head{Retcode: 1}}, err
	}

	if id != 0 {
		switch in.Type {
		case 0:
			_, err = db.Exec("UPDATE youku_video SET play = play + 1 WHERE vid = ?", in.Id)
		case 1:
			_, err = db.Exec("UPDATE news SET click = click + 1 WHERE id = ?", in.Id)
		case 2:
			_, err = db.Exec("UPDATE ads SET display = display + 1 WHERE id = ?", in.Id)
		case 3:
			_, err = db.Exec("UPDATE ads SET click = click + 1 WHERE id = ?", in.Id)
		case 4:
			_, err = db.Exec("INSERT INTO service_click(sid, click, ctime) VALUES (?, 1, CURDATE()) ON DUPLICATE KEY UPDATE click = click + 1", in.Id)
		default:
			log.Printf("illegal type:%d, id:%d uid:%d", in.Type, in.Id, in.Head.Uid)

		}
		if err != nil {
			log.Printf("update click count failed type:%d id:%d:%v", in.Type, in.Id, err)
			return &common.CommReply{Head: &common.Head{Retcode: 1}}, err
		}
	}

	return &common.CommReply{Head: &common.Head{Retcode: 0, Uid: in.Head.Uid}}, nil
}

func (s *server) ReportApmac(ctx context.Context, in *modify.ApmacRequest) (*common.CommReply, error) {
	var aid int
	mac := strings.Replace(strings.ToLower(in.Apmac), ":", "", -1)
	log.Printf("ap mac origin:%s convert:%s\n", in.Apmac, mac)
	err := db.QueryRow("SELECT id FROM ap WHERE mac = ? OR mac = ?", in.Apmac, mac).Scan(&aid)
	if err != nil {
		log.Printf("select aid from ap failed uid:%d mac:%s err:%v\n", in.Head.Uid, in.Apmac, err)
		return &common.CommReply{Head: &common.Head{Retcode: 0, Uid: in.Head.Uid}}, nil
	}
	_, err = db.Exec("UPDATE user SET aid = ?, aptime = NOW() WHERE uid = ?", aid, in.Head.Uid)
	if err != nil {
		log.Printf("update user ap info failed uid:%d aid:%d\n", in.Head.Uid, aid)
	}
	return &common.CommReply{Head: &common.Head{Retcode: 0, Uid: in.Head.Uid}}, nil
}

func (s *server) AddImage(ctx context.Context, in *modify.AddImageRequest) (*common.CommReply, error) {
	for i := 0; i < len(in.Fnames); i++ {
		_, err := db.Exec("INSERT IGNORE INTO image(uid, name, ctime) VALUES(?, ?, NOW())",
			in.Head.Uid, in.Fnames[i])
		if err != nil {
			log.Printf("insert into image failed uid:%d name:%s err:%v\n", in.Head.Uid, in.Fnames[i], err)
		}
	}
	return &common.CommReply{Head: &common.Head{Retcode: 0, Uid: in.Head.Uid}}, nil
}

func (s *server) FinImage(ctx context.Context, in *modify.ImageRequest) (*common.CommReply, error) {
	_, err := db.Exec("UPDATE image SET filesize = ?, height = ?, width = ?, ftime = NOW(), status = 1 WHERE name = ?",
		in.Info.Size, in.Info.Height, in.Info.Width, in.Info.Name)
	if err != nil {
		log.Printf("update image failed name:%s err:%v\n", in.Info.Name, err)
	}
	return &common.CommReply{Head: &common.Head{Retcode: 0, Uid: in.Head.Uid}}, nil
}

func (s *server) AddBanner(ctx context.Context, in *modify.BannerRequest) (*common.CommReply, error) {
	res, err := db.Exec("INSERT INTO banner(img, dst, priority, title, type, ctime) VALUES(?, ?, ?, ?, ?, NOW())",
		in.Info.Img, in.Info.Dst, in.Info.Priority, in.Info.Title, in.Info.Type)
	if err != nil {
		log.Printf("insert into banner failed img:%s dst:%s err:%v\n", in.Info.Img, in.Info.Dst, err)
		return &common.CommReply{Head: &common.Head{Retcode: 1, Uid: in.Head.Uid}}, nil
	}
	id, err := res.LastInsertId()
	if err != nil {
		log.Printf("AddBanner get LastInsertId failed:%v\n", err)
		return &common.CommReply{Head: &common.Head{Retcode: 1, Uid: in.Head.Uid}}, nil
	}
	return &common.CommReply{Head: &common.Head{Retcode: 0, Uid: in.Head.Uid}, Id: id}, nil
}

func (s *server) ModBanner(ctx context.Context, in *modify.BannerRequest) (*common.CommReply, error) {
	query := fmt.Sprintf("UPDATE banner SET priority = %d, online = %d, deleted = %d ",
		in.Info.Priority, in.Info.Online, in.Info.Deleted)
	if in.Info.Img != "" {
		query += ", img = '" + in.Info.Img + "' "
	}
	if in.Info.Dst != "" {
		query += ", dst = '" + in.Info.Dst + "' "
	}
	if in.Info.Title != "" {
		query += ", title = '" + in.Info.Title + "' "
	}
	query += fmt.Sprintf(" WHERE id = %d", in.Info.Id)
	_, err := db.Exec(query)
	if err != nil {
		log.Printf("insert into banner failed img:%s dst:%s err:%v\n", in.Info.Img, in.Info.Dst, err)
		return &common.CommReply{Head: &common.Head{Retcode: 1, Uid: in.Head.Uid}}, nil
	}
	return &common.CommReply{Head: &common.Head{Retcode: 0, Uid: in.Head.Uid}}, nil
}

func (s *server) AddTags(ctx context.Context, in *modify.AddTagRequest) (*modify.AddTagReply, error) {
	var ids []int32
	for i := 0; i < len(in.Tags); i++ {
		res, err := db.Exec("INSERT INTO tags(content, ctime) VALUES (?, NOW())", in.Tags[i])
		if err != nil {
			log.Printf("add tag failed tag:%s err:%v\n", in.Tags[i], err)
			continue
		}
		id, err := res.LastInsertId()
		if err != nil {
			log.Printf("get tag insert id failed:%v", err)
			continue
		}
		ids = append(ids, int32(id))
	}
	return &modify.AddTagReply{Head: &common.Head{Retcode: 0, Uid: in.Head.Uid}, Ids: ids}, nil
}

func genIDStr(ids []int64) string {
	var str string
	for i := 0; i < len(ids); i++ {
		str += strconv.Itoa(int(ids[i]))
		if i < len(ids)-1 {
			str += ","
		}
	}
	return str
}

func (s *server) DelTags(ctx context.Context, in *modify.DelTagRequest) (*common.CommReply, error) {
	str := genIDStr(in.Ids)
	query := "UPDATE tags SET deleted = 1 WHERE id IN (" + str + ")"
	_, err := db.Exec(query)
	if err != nil {
		log.Printf("DelTags failed:%v", err)
	}
	return &common.CommReply{Head: &common.Head{Retcode: 0, Uid: in.Head.Uid}}, nil
}

func genConfStr(names []string) string {
	var str string
	for i := 0; i < len(names); i++ {
		str += "'" + names[i] + "'"
		if i < len(names)-1 {
			str += ","
		}
	}
	return str
}

func (s *server) DelConf(ctx context.Context, in *modify.DelConfRequest) (*common.CommReply, error) {
	str := genConfStr(in.Names)
	query := "UPDATE kv_config SET deleted = 1 WHERE name IN (" + str + ")"
	_, err := db.Exec(query)
	if err != nil {
		log.Printf("DelTags failed:%v", err)
	}
	return &common.CommReply{Head: &common.Head{Retcode: 0, Uid: in.Head.Uid}}, nil
}

func (s *server) AddAddress(ctx context.Context, in *modify.AddressRequest) (*common.CommReply, error) {
	log.Printf("AddAddress uid:%d detail:%s", in.Head.Uid, in.Info.Detail)
	res, err := db.Exec("INSERT INTO address(uid, consignee, phone, province, city, district, detail, zip, addr, ctime) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, NOW())",
		in.Head.Uid, in.Info.User, in.Info.Mobile, in.Info.Province, in.Info.City, in.Info.Zone,
		in.Info.Detail, in.Info.Zip, in.Info.Addr)
	if err != nil {
		log.Printf("add address failed uid:%d detail:%s err:%v\n", in.Head.Uid, in.Info.Detail, err)
		return &common.CommReply{Head: &common.Head{Retcode: 1, Uid: in.Head.Uid}}, errors.New("add address failed")
	}
	id, err := res.LastInsertId()
	if err != nil {
		log.Printf("add address get insert id failed:%v", err)
		return &common.CommReply{Head: &common.Head{Retcode: 1, Uid: in.Head.Uid}}, errors.New("add address failed")
	}
	if in.Info.Def {
		_, err = db.Exec("UPDATE user SET address = ? WHERE uid = ?", id, in.Head.Uid)
		if err != nil {
			log.Printf("update user address failed, uid:%d aid:%d", in.Head.Uid, id)
		}
	}
	return &common.CommReply{Head: &common.Head{Retcode: 0, Uid: in.Head.Uid}, Id: id}, nil
}

func (s *server) ModAddress(ctx context.Context, in *modify.AddressRequest) (*common.CommReply, error) {
	log.Printf("ModAddress uid:%d detail:%s", in.Head.Uid, in.Info.Detail)
	_, err := db.Exec("UPDATE address SET consignee = ?, phone = ?, province = ?, city = ?, district = ?, detail = ?, zip = ?, addr = ? WHERE uid = ? AND aid = ?",
		in.Info.User, in.Info.Mobile, in.Info.Province, in.Info.City, in.Info.Zone,
		in.Info.Detail, in.Info.Zip, in.Info.Addr, in.Head.Uid, in.Info.Aid)
	if err != nil {
		log.Printf("modify address failed uid:%d detail:%s err:%v\n", in.Head.Uid, in.Info.Detail, err)
		return &common.CommReply{Head: &common.Head{Retcode: 1, Uid: in.Head.Uid}}, errors.New("add address failed")
	}
	return &common.CommReply{Head: &common.Head{Retcode: 0, Uid: in.Head.Uid}}, nil
}

func (s *server) DelAddress(ctx context.Context, in *modify.AddressRequest) (*common.CommReply, error) {
	log.Printf("DelAddress uid:%d aid:%d", in.Head.Uid, in.Info.Aid)
	_, err := db.Exec("UPDATE address SET deleted = 1 WHERE uid = ? AND aid = ?",
		in.Head.Uid, in.Info.Aid)
	if err != nil {
		log.Printf("del address failed uid:%d aid:%d err:%v\n", in.Head.Uid, in.Info.Aid, err)
		return &common.CommReply{Head: &common.Head{Retcode: 1, Uid: in.Head.Uid}}, errors.New("add address failed")
	}
	return &common.CommReply{Head: &common.Head{Retcode: 0, Uid: in.Head.Uid}}, nil
}

func (s *server) AddConf(ctx context.Context, in *modify.ConfRequest) (*common.CommReply, error) {
	log.Printf("AddConf uid:%d key:%s", in.Head.Uid, in.Info.Key)
	_, err := db.Exec("INSERT INTO kv_config(name, val, ctime) VALUES (?, ?, NOW()) ON DUPLICATE KEY UPDATE val = ?",
		in.Info.Key, in.Info.Val, in.Info.Val)
	if err != nil {
		log.Printf("add config failed uid:%d name:%s err:%v\n", in.Head.Uid, in.Info.Key, err)
		return &common.CommReply{Head: &common.Head{Retcode: 1, Uid: in.Head.Uid}}, errors.New("add conf failed")
	}
	return &common.CommReply{Head: &common.Head{Retcode: 0, Uid: in.Head.Uid}}, nil
}

func (s *server) AddAdBan(ctx context.Context, in *modify.AddBanRequest) (*common.CommReply, error) {
	log.Printf("AddAdBan uid:%d term:%s version", in.Head.Uid, in.Info.Term, in.Info.Version)
	res, err := db.Exec("INSERT INTO ad_ban(term, version, ctime) VALUES (?, ?, NOW()) ON DUPLICATE KEY UPDATE deleted = 0",
		in.Info.Term, in.Info.Version)
	if err != nil {
		log.Printf("add adban failed uid:%d term:%d version:%d err:%v\n",
			in.Head.Uid, in.Info.Term, in.Info.Version, err)
		return &common.CommReply{Head: &common.Head{Retcode: 1, Uid: in.Head.Uid}}, errors.New("add adban failed")
	}
	id, err := res.LastInsertId()
	if err != nil {
		log.Printf("add adban get insert id failed uid:%d term:%d version:%d err:%v\n",
			in.Head.Uid, in.Info.Term, in.Info.Version, err)
		return &common.CommReply{Head: &common.Head{Retcode: 1, Uid: in.Head.Uid}}, errors.New("add adban failed")
	}
	return &common.CommReply{Head: &common.Head{Retcode: 0, Uid: in.Head.Uid}, Id: id}, nil
}

func (s *server) DelAdBan(ctx context.Context, in *modify.DelBanRequest) (*common.CommReply, error) {
	log.Printf("DelAdBan uid:%d", in.Head.Uid)
	idStr := genIDStr(in.Ids)
	query := fmt.Sprintf("UPDATE ad_ban SET deleted = 1 WHERE id IN (%s)", idStr)
	log.Printf("query :%s", query)
	_, err := db.Exec(query)
	if err != nil {
		log.Printf("DelAdBan query failed:%v", err)
	}
	return &common.CommReply{Head: &common.Head{Retcode: 0, Uid: in.Head.Uid}}, nil
}

func (s *server) AddWhiteList(ctx context.Context, in *modify.WhiteRequest) (*common.CommReply, error) {
	for _, v := range in.Ids {
		_, err := db.Exec("INSERT INTO white_list(type, uid, ctime) VALUES (?, ?, NOW()) ON DUPLICATE KEY UPDATE deleted = 0", in.Type, v)
		if err != nil {
			log.Printf("AddWhiteList insert failed uid:%d %v", v, err)
			continue
		}
	}
	return &common.CommReply{Head: &common.Head{Retcode: 0, Uid: in.Head.Uid}}, nil
}

func (s *server) DelWhiteList(ctx context.Context, in *modify.WhiteRequest) (*common.CommReply, error) {
	idStr := genIDStr(in.Ids)
	query := fmt.Sprintf("UPDATE white_list SET deleted = 1 WHERE type = 0 AND uid IN (%s)", idStr)
	log.Printf("DelWhiteList query:%s", query)
	_, err := db.Exec(query)
	if err != nil {
		log.Printf("DelWhiteList query failed:%v", err)
	}
	return &common.CommReply{Head: &common.Head{Retcode: 0, Uid: in.Head.Uid}}, nil
}

func (s *server) AddFeedback(ctx context.Context, in *modify.FeedRequest) (*common.CommReply, error) {
	var last int64
	db.QueryRow("SELECT UNIX_TIMESTAMP(ctime) FROM feedback WHERE uid = ? ORDER BY id DESC LIMIT 1").
		Scan(&last)
	if time.Now().Unix() > last+feedInterval {
		db.Exec("INSERT INTO feedback(uid, content, ctime) VALUES(?, ?, NOW())", in.Head.Uid,
			in.Content)
	} else {
		log.Printf("frequency exceed limit uid:%d", in.Head.Uid)
	}
	return &common.CommReply{Head: &common.Head{Retcode: 0, Uid: in.Head.Uid}}, nil
}

func main() {
	lis, err := net.Listen("tcp", util.ModifyServerPort)
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	db, err = util.InitDB(false)
	if err != nil {
		log.Fatalf("failed to init db connection:%v", err)
	}
	db.SetMaxIdleConns(util.MaxIdleConns)

	kv := util.InitRedis()
	go util.ReportHandler(kv, util.ModifyServerName, util.ModifyServerPort)

	s := grpc.NewServer()
	modify.RegisterModifyServer(s, &server{})
	if err := s.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}
