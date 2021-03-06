package main

import (
	"Server/proto/common"
	"Server/proto/inquiry"
	"Server/util"
	"database/sql"
	"fmt"
	"log"
	"time"

	"golang.org/x/net/context"
)

func getUserRole(db *sql.DB, uid int64) int64 {
	var role int64
	err := db.QueryRow("SELECT role FROM users WHERE uid = ?", uid).Scan(&role)
	if err != nil {
		log.Printf("getUserRole failed:%d %v", uid, err)
	}
	return role
}

func (s *server) SendChat(ctx context.Context, in *inquiry.ChatRequest) (*common.CommReply, error) {
	util.PubRPCRequest(w, "inquiry", "SendChat")
	role := getUserRole(db, in.Head.Uid)
	var hid, status int64
	var doctor, patient int64
	if role == 1 {
		doctor = in.Head.Uid
		patient = in.Tuid
	} else {
		doctor = in.Tuid
		patient = in.Head.Uid
	}
	err := db.QueryRow("SELECT id, status FROM inquiry_history WHERE doctor = ? AND patient = ? ORDER BY id DESC LIMIT 1", doctor, patient).Scan(&hid, &status)
	if err != nil {
		log.Printf("SendChat get inquiry info failed:%d %d %v", doctor,
			patient, err)
		return &common.CommReply{Head: &common.Head{Retcode: 1}}, nil
	}
	if status != 1 && role == 0 {
		log.Printf("illegal status for patient to send chat:%d %d",
			hid, status)
		return &common.CommReply{Head: &common.Head{Retcode: 1}}, nil
	}

	res, err := db.Exec("INSERT INTO chat(uid, tuid, type, content, hid, ctime) VALUES (?, ?, ?, ?, ?, NOW())",
		in.Head.Uid, in.Tuid, in.Type, in.Content, hid)
	if err != nil {
		log.Printf("SendChat insert failed:%d %d %v", in.Head.Uid,
			in.Tuid, err)
		return &common.CommReply{Head: &common.Head{Retcode: 1}}, nil
	}
	id, err := res.LastInsertId()
	if err != nil {
		log.Printf("SendChat get insert id failed:%d %d %v", in.Head.Uid,
			in.Tuid, err)
		return &common.CommReply{Head: &common.Head{Retcode: 1}}, nil
	}
	util.PubRPCSuccRsp(w, "inquiry", "SendChat")
	return &common.CommReply{Head: &common.Head{Retcode: 0},
		Id: id}, nil
}

func getNewChat(db *sql.DB, uid, tuid, num int64) []*inquiry.ChatInfo {
	rows, err := db.Query("SELECT id, uid, tuid, type, content, ctime FROM chat WHERE ((uid = ? AND tuid = ?) OR (uid = ? AND tuid = ?)) ORDER BY id DESC LIMIT ?",
		uid, tuid, tuid, uid, num)
	if err != nil {
		log.Printf("getNewChat failed:%d %d %v", uid, tuid, err)
		return nil
	}
	defer rows.Close()
	var infos []*inquiry.ChatInfo
	var minseq int64
	for rows.Next() {
		var info inquiry.ChatInfo
		err = rows.Scan(&info.Id, &info.Uid, &info.Tuid, &info.Type, &info.Content, &info.Ctime)
		if err != nil {
			log.Printf("getUserChat scan failed:%d %d %v", uid, tuid, err)
			continue
		}
		info.Seq = info.Id
		minseq = info.Id
		infos = append(infos, &info)
	}
	_, err = db.Exec("UPDATE chat SET ack = 1, acktime = NOW() WHERE uid = ? AND tuid = ? AND id >= ? AND ack = 0",
		tuid, uid, minseq)
	if err != nil {
		log.Printf("getUserChat update ack failed:%v", err)
	}
	return infos
}

func getUserChat(db *sql.DB, uid, tuid, seq, num int64) []*inquiry.ChatInfo {
	if seq == -1 {
		return getNewChat(db, uid, tuid, num)
	}
	rows, err := db.Query("SELECT id, uid, tuid, type, content, ctime FROM chat WHERE ((uid = ? AND tuid = ?) OR (uid = ? AND tuid = ?)) AND id > ? ORDER BY id ASC LIMIT ?",
		uid, tuid, tuid, uid, seq, num)
	if err != nil {
		log.Printf("getUserChat query failed:%d %d %v", uid, tuid, err)
		return nil
	}
	var infos []*inquiry.ChatInfo
	var maxseq int64
	defer rows.Close()
	for rows.Next() {
		var info inquiry.ChatInfo
		err = rows.Scan(&info.Id, &info.Uid, &info.Tuid, &info.Type, &info.Content, &info.Ctime)
		if err != nil {
			log.Printf("getUserChat scan failed:%d %d %v", uid, tuid, err)
			continue
		}
		info.Seq = info.Id
		maxseq = info.Seq
		infos = append(infos, &info)
	}
	_, err = db.Exec("UPDATE chat SET ack = 1, acktime = NOW() WHERE uid = ? AND tuid = ? AND id <= ? AND ack = 0",
		tuid, uid, maxseq)
	if err != nil {
		log.Printf("getUserChat update ack failed:%v", err)
	}
	return infos
}

func getInquiryInfo(db *sql.DB, uid, tuid int64) (hid, status int64) {
	err := db.QueryRow("SELECT hid, status FROM relations WHERE (doctor = ? AND patient = ?) OR (doctor = ? AND patient = ?)", uid, tuid, tuid, uid).
		Scan(&hid, &status)
	if err != nil {
		log.Printf("getInquiryStatus query failed:%d %d %v", uid, tuid, err)
	}
	return
}

func getInquiryPtime(db *sql.DB, hid int64) int64 {
	var ptime int64
	err := db.QueryRow("SELECT UNIX_TIMESTAMP(ptime) FROM inquiry_history WHERE id = ?", hid).
		Scan(&ptime)
	if err != nil {
		log.Printf("getInquiryPtime failed:%d %v", hid, err)
	}
	return ptime
}

func getRefundFlag(db *sql.DB, hid, uid, tuid int64) int64 {
	role := getUserRole(db, uid)
	if role == doctorRole {
		return 0
	}
	ctime := getLastCtime(db, hid, tuid)
	if ctime == 0 {
		ptime := getInquiryPtime(db, hid)
		if ptime+8*3600 < time.Now().Unix() {
			return 1
		}
	} else {
		if ctime+8*3600 < time.Now().Unix() {
			return 1
		}
	}
	return 0
}

func (s *server) GetChat(ctx context.Context, in *common.CommRequest) (*inquiry.ChatReply, error) {
	log.Printf("GetChat request:%+v", in)
	util.PubRPCRequest(w, "inquiry", "GetChat")
	infos := getUserChat(db, in.Head.Uid, in.Id, in.Seq, in.Num)
	hid, status := getInquiryInfo(db, in.Head.Uid, in.Id)
	var rflag int64
	if status == inquiryStatus {
		rflag = getRefundFlag(db, hid, in.Head.Uid, in.Id)
	}
	util.PubRPCSuccRsp(w, "inquiry", "GetChat")
	return &inquiry.ChatReply{Head: &common.Head{Retcode: 0},
		Infos: infos, Status: status, Rflag: rflag}, nil
}

type chatInfo struct {
	cid     int64
	ctype   int64
	content string
	ctime   string
	ack     int64
}

func getLastChat(db *sql.DB, doctor, patient int64) (*chatInfo, error) {
	var info chatInfo
	err := db.QueryRow("SELECT id, type, content, ctime, ack FROM chat WHERE uid = ? AND tuid = ? ORDER BY id DESC LIMIT 1", patient, doctor).
		Scan(&info.cid, &info.ctype, &info.content, &info.ctime,
			&info.ack)
	if err != nil {
		log.Printf("getLastChat failed:%d %d %v", doctor, patient, err)
		return nil, err
	}
	return &info, nil
}

func getUserChatSession(db *sql.DB, doctor, seq, num int64) []*inquiry.ChatSessionInfo {
	query := fmt.Sprintf("SELECT r.id, r.patient, r.status, u.headurl, u.nickname FROM relations r, users u WHERE r.patient = u.uid AND r.doctor = %d AND r.flag = 1 AND r.deleted = 0 AND u.deleted = 0", doctor)
	if seq != 0 {
		query += fmt.Sprintf(" AND r.id < %d", seq)
	}
	query += fmt.Sprintf(" ORDER BY r.id DESC LIMIT %d", num)
	log.Printf("getUserChatSession query:%s", query)
	rows, err := db.Query(query)
	if err != nil {
		log.Printf("getUserChatSession query failed:%v", err)
		return nil
	}

	var infos []*inquiry.ChatSessionInfo
	defer rows.Close()
	for rows.Next() {
		var info inquiry.ChatSessionInfo
		err = rows.Scan(&info.Id, &info.Uid, &info.Status, &info.Headurl,
			&info.Nickname)
		if err != nil {
			log.Printf("getUserChatSession scan failed:%v", err)
			continue
		}
		cinfo, err := getLastChat(db, doctor, info.Uid)
		if err != nil {
			log.Printf("getUserChatSession getLastChat failed:%v", err)
		} else {
			info.Cid = cinfo.cid
			info.Type = cinfo.ctype
			info.Content = cinfo.content
			info.Ctime = cinfo.ctime
			if cinfo.ack == 0 {
				info.Reddot = 1
			}
		}

		infos = append(infos, &info)
	}
	return infos
}

func (s *server) GetChatSession(ctx context.Context, in *common.CommRequest) (*inquiry.ChatSessionReply, error) {
	util.PubRPCRequest(w, "inquiry", "GetChatSession")
	infos := getUserChatSession(db, in.Head.Uid, in.Seq, in.Num)
	util.PubRPCSuccRsp(w, "inquiry", "GetChatSession")
	var hasmore int64
	if len(infos) >= int(in.Num) {
		hasmore = 1
	}
	return &inquiry.ChatSessionReply{Head: &common.Head{Retcode: 0},
		Infos: infos, Hasmore: hasmore}, nil
}
