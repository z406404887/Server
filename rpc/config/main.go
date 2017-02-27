package main

import (
	"fmt"
	"log"
	"net"

	"database/sql"

	"Server/proto/common"
	"Server/proto/config"
	"Server/util"

	_ "github.com/go-sql-driver/mysql"
	nsq "github.com/nsqio/go-nsq"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	redis "gopkg.in/redis.v5"
)

const (
	menuType = 0
	tabType  = 1
)

type server struct{}

var db *sql.DB
var kv *redis.Client
var w *nsq.Producer

func getPortalMenu(db *sql.DB, stype int64, flag bool) []*config.PortalMenuInfo {
	query := fmt.Sprintf("SELECT icon, text, name, routername, url FROM portal_menu WHERE type = %d AND deleted = 0 ", stype)
	if !flag {
		query += " AND dbg = 0 "
	}
	query += " ORDER BY priority DESC"
	rows, err := db.Query(query)
	var infos []*config.PortalMenuInfo
	if err != nil {
		log.Printf("getPortalMenu query failed:%v", err)
		return infos
	}

	defer rows.Close()
	for rows.Next() {
		var info config.PortalMenuInfo
		err := rows.Scan(&info.Icon, &info.Text, &info.Name, &info.Routername,
			&info.Url)
		if err != nil {
			log.Printf("getPortalMenu scan failed:%v", err)
			continue
		}
		infos = append(infos, &info)
	}
	return infos
}

func (s *server) GetPortalMenu(ctx context.Context, in *common.CommRequest) (*config.PortalMenuReply, error) {
	util.PubRPCRequest(w, "config", "GetPortalMenu")
	flag := util.IsWhiteUser(db, in.Head.Uid, util.PortalMenuDbgType)
	menulist := getPortalMenu(db, menuType, flag)
	tablist := getPortalMenu(db, tabType, flag)
	util.PubRPCSuccRsp(w, "config", "GetPortalMenu")
	return &config.PortalMenuReply{
		Head: &common.Head{Retcode: 0, Uid: in.Head.Uid}, Menulist: menulist,
		Tablist: tablist}, nil
}

func getBanners(db *sql.DB, flag bool) []*config.MediaInfo {
	var infos []*config.MediaInfo
	query := "SELECT img, dst, id FROM banner WHERE deleted = 0 AND type = 0"
	if flag {
		query += " AND (online = 1 OR dbg = 1) "
	} else {
		query += " AND online = 1 "
	}
	query += " ORDER BY priority DESC LIMIT 20"
	rows, err := db.Query(query)
	if err != nil {
		log.Printf("select banner info failed:%v", err)
		return infos
	}
	for rows.Next() {
		var info config.MediaInfo
		err := rows.Scan(&info.Img, &info.Dst, &info.Id)
		if err != nil {
			log.Printf("scan failed:%v", err)
			continue
		}

		infos = append(infos, &info)

	}
	return infos
}

func getUrbanServices(db *sql.DB, term int64) []*config.MediaInfo {
	var infos []*config.MediaInfo
	rows, err := db.Query("SELECT title, img, dst, id FROM urban_service WHERE type = ?", term)
	if err != nil {
		log.Printf("getUrbanServices query failed:%v", err)
		return infos
	}
	defer rows.Close()
	for rows.Next() {
		var info config.MediaInfo
		err := rows.Scan(&info.Title, &info.Img, &info.Dst, &info.Id)
		if err != nil {
			log.Printf("getUrbanServices scan failed:%v", err)
			continue
		}
		infos = append(infos, &info)
	}
	return infos
}

func getRecommends(db *sql.DB) []*config.MediaInfo {
	var infos []*config.MediaInfo
	rows, err := db.Query("SELECT img, dst, id FROM recommend WHERE deleted = 0 ORDER BY priority DESC")
	if err != nil {
		log.Printf("getRecommends query failed:%v", err)
		return infos
	}
	defer rows.Close()
	for rows.Next() {
		var info config.MediaInfo
		err := rows.Scan(&info.Img, &info.Dst, &info.Id)
		if err != nil {
			log.Printf("getRecommends scan failed:%v", err)
			continue
		}
		infos = append(infos, &info)
	}
	return infos
}

func getCategoryTitleIcon(category int) (string, string) {
	switch category {
	default:
		return "智慧政务", "http://file.yunxingzh.com/ico_government_xxxh.png"
	case 2:
		return "交通出行", "http://file.yunxingzh.com/ico_traffic_xxxh.png"
	case 3:
		return "医疗服务", "http://file.yunxingzh.com/ico_medical_xxxh.png"
	case 4:
		return "网上充值", "http://file.yunxingzh.com/ico_recharge.png"
	}
}

func getServices(db *sql.DB) []*config.ServiceCategory {
	var infos []*config.ServiceCategory
	rows, err := db.Query("SELECT title, dst, category, sid, icon FROM service WHERE category != 0 AND deleted = 0 AND dst != '' ORDER BY category")
	if err != nil {
		log.Printf("query failed:%v", err)
		return infos
	}
	defer rows.Close()

	category := 0
	var srvs []*config.ServiceInfo
	for rows.Next() {
		var info config.ServiceInfo
		var cate int
		err := rows.Scan(&info.Title, &info.Dst, &cate, &info.Sid, &info.Icon)
		if err != nil {
			continue
		}

		if cate != category {
			if len(srvs) > 0 {
				var cateinfo config.ServiceCategory
				cateinfo.Title, cateinfo.Icon = getCategoryTitleIcon(category)
				cateinfo.Items = srvs[:]
				infos = append(infos, &cateinfo)
				srvs = srvs[len(srvs):]
			}
			category = cate
		}
		srvs = append(srvs, &info)
	}

	if len(srvs) > 0 {
		var cateinfo config.ServiceCategory
		cateinfo.Title, cateinfo.Icon = getCategoryTitleIcon(category)
		cateinfo.Items = srvs[:]
		infos = append(infos, &cateinfo)
	}

	return infos
}

func (s *server) GetDiscovery(ctx context.Context, in *common.CommRequest) (*config.DiscoveryReply, error) {
	util.PubRPCRequest(w, "config", "GetDiscovery")
	banners := getBanners(db, false)
	urbanservices := getUrbanServices(db, in.Head.Term)
	recommends := getRecommends(db)
	services := getServices(db)
	util.PubRPCSuccRsp(w, "config", "GetDiscovery")
	return &config.DiscoveryReply{
		Head: &common.Head{Retcode: 0, Uid: in.Head.Uid}, Banners: banners,
		Urbanservices: urbanservices, Recommends: recommends,
		Services: services}, nil
}

func fetchPortalMenu(db *sql.DB, stype int64) []*config.PortalMenuInfo {
	var infos []*config.PortalMenuInfo
	rows, err := db.Query("SELECT id, icon, text, name, routername, url, priority, dbg, deleted FROM portal_menu WHERE type = ? ORDER BY priority DESC", stype)
	if err != nil {
		log.Printf("fetchPortalMenu query failed:%v", err)
		return infos
	}

	defer rows.Close()
	for rows.Next() {
		var info config.PortalMenuInfo
		err := rows.Scan(&info.Id, &info.Icon, &info.Text, &info.Name, &info.Routername,
			&info.Url, &info.Priority, &info.Dbg, &info.Deleted)
		if err != nil {
			log.Printf("fetchPortalMenu scan failed:%v", err)
			continue
		}
		infos = append(infos, &info)
	}
	return infos
}

func (s *server) FetchPortalMenu(ctx context.Context, in *common.CommRequest) (*config.MenuReply, error) {
	util.PubRPCRequest(w, "config", "FetchPortalMenu")
	infos := fetchPortalMenu(db, in.Type)
	util.PubRPCSuccRsp(w, "config", "FetchPortalMenu")
	return &config.MenuReply{
		Head: &common.Head{Retcode: 0, Uid: in.Head.Uid}, Infos: infos}, nil
}

func (s *server) AddPortalMenu(ctx context.Context, in *config.MenuRequest) (*common.CommReply, error) {
	util.PubRPCRequest(w, "config", "AddPortalMenu")
	res, err := db.Exec("INSERT INTO portal_menu(type, icon, text, name, routername, url, priority, dbg, deleted, ctime) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, NOW())",
		in.Info.Type, in.Info.Icon, in.Info.Text, in.Info.Name, in.Info.Routername,
		in.Info.Url, in.Info.Priority, in.Info.Dbg, in.Info.Deleted)
	if err != nil {
		log.Printf("AddPortalMenu query failed:%v", err)
		return &common.CommReply{
			Head: &common.Head{Retcode: 1, Uid: in.Head.Uid}}, nil
	}
	id, err := res.LastInsertId()
	if err != nil {
		log.Printf("AddPortalMenu get id failed:%v", err)
		return &common.CommReply{
			Head: &common.Head{Retcode: 1, Uid: in.Head.Uid}}, nil
	}
	util.PubRPCSuccRsp(w, "config", "AddPortalMenu")
	return &common.CommReply{
		Head: &common.Head{Retcode: 0, Uid: in.Head.Uid}, Id: id}, nil
}

func (s *server) ModPortalMenu(ctx context.Context, in *config.MenuRequest) (*common.CommReply, error) {
	util.PubRPCRequest(w, "config", "ModPortalMenu")
	query := fmt.Sprintf("UPDATE portal_menu SET mtime = NOW(), dbg = %d, deleted = %d ",
		in.Info.Dbg, in.Info.Deleted)
	if in.Info.Icon != "" {
		query += ", icon = '" + in.Info.Icon + "' "
	}
	if in.Info.Text != "" {
		query += ", text = '" + in.Info.Text + "' "
	}
	if in.Info.Name != "" {
		query += ", name = '" + in.Info.Name + "' "
	}
	if in.Info.Url != "" {
		query += ", url = '" + in.Info.Url + "' "
	}
	if in.Info.Priority != 0 {
		query += fmt.Sprintf(", priority = %d", in.Info.Priority)
	}
	if in.Info.Routername != "" {
		query += ", routername = '" + in.Info.Routername + "' "
	}
	query += fmt.Sprintf(" WHERE id = %d", in.Info.Id)
	_, err := db.Exec(query)
	if err != nil {
		log.Printf("ModPortalMenu query failed:%v", err)
		return &common.CommReply{
			Head: &common.Head{Retcode: 1, Uid: in.Head.Uid}}, nil
	}
	util.PubRPCSuccRsp(w, "config", "ModPortalMenu")
	return &common.CommReply{
		Head: &common.Head{Retcode: 0, Uid: in.Head.Uid}}, nil
}

func main() {
	lis, err := net.Listen("tcp", util.ConfigServerPort)
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	w = util.NewNsqProducer()

	db, err = util.InitDB(false)
	if err != nil {
		log.Fatalf("failed to init db connection: %v", err)
	}
	db.SetMaxIdleConns(util.MaxIdleConns)
	kv = util.InitRedis()
	go util.ReportHandler(kv, util.ConfigServerName, util.ConfigServerPort)
	//cli := util.InitEtcdCli()
	//go util.ReportEtcd(cli, util.ConfigServerName, util.ConfigServerPort)

	s := grpc.NewServer()
	config.RegisterConfigServer(s, &server{})
	if err := s.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}
