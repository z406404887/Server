package httpserver

import (
	"bytes"
	"errors"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"strconv"
	"strings"
	"time"

	"Server/aliyun"
	"Server/pay"
	"Server/proto/common"
	"Server/proto/config"
	"Server/proto/fetch"
	"Server/proto/punch"
	"Server/proto/userinfo"

	"Server/proto/hot"

	"Server/proto/modify"
	"Server/proto/verify"
	"Server/util"

	simplejson "github.com/bitly/go-simplejson"
	pingpp "github.com/pingplusplus/pingpp-go/pingpp"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
)

const (
	wxHost     = "http://wx.yunxingzh.com/"
	maxZipcode = 820000
	portalDst  = "http://120.25.133.234/"
)

type portalDir struct {
	Dir    string
	Expire int64
}

var pdir = portalDir{
	Dir:    "dist/",
	Expire: time.Now().Unix(),
}

func login(w http.ResponseWriter, r *http.Request) (apperr *util.AppError) {
	var req Request
	req.Init(r)
	username := req.GetParamString("username")
	password := req.GetParamString("password")
	model := req.GetParamString("model")
	udid := req.GetParamString("udid")

	uuid := util.GenUUID()
	resp, rpcerr := callRPC(util.VerifyServerType, 0, "Login",
		&verify.LoginRequest{Head: &common.Head{Sid: uuid},
			Username: username, Password: password, Model: model, Udid: udid})
	checkRPCErr(rpcerr, "Login")
	res := resp.Interface().(*verify.LoginReply)
	checkRPCCode(res.Head.Retcode, "Login")

	body := genResponseBody(res, true)
	w.Write(body)
	reportSuccResp(r.RequestURI)
	return nil
}

func getCode(phone string, ctype int64) (bool, error) {
	uuid := util.GenUUID()
	resp, rpcerr := callRPC(util.VerifyServerType, 0, "GetPhoneCode",
		&verify.CodeRequest{Head: &common.Head{Sid: uuid},
			Phone: phone, Ctype: ctype})
	checkRPCErr(rpcerr, "GetPhoneCode")
	res := resp.Interface().(*verify.VerifyReply)

	return res.Result, nil
}

func getPhoneCode(w http.ResponseWriter, r *http.Request) (apperr *util.AppError) {
	var req Request
	req.Init(r)
	phone := req.GetParamString("phone")
	ctype := req.GetParamIntDef("type", 0)

	if !util.IsIllegalPhone(phone) {
		log.Printf("getPhoneCode illegal phone:%s", phone)
		return &util.AppError{errIllegalPhone, "请输入正确的手机号"}
	}

	flag, err := getCode(phone, ctype)
	if err != nil || !flag {
		return &util.AppError{errCode, "获取验证码失败"}
	}
	w.Write([]byte(`{"errno":0}`))
	reportSuccResp(r.RequestURI)
	return nil
}

func getCheckCode(w http.ResponseWriter, r *http.Request) (apperr *util.AppError) {
	var req Request
	req.Init(r)
	phone := req.GetParamString("phone")
	acname := req.GetParamStringDef("wlanacname", "")
	term := req.GetParamInt("term")

	if !util.IsIllegalPhone(phone) {
		log.Printf("getCheckCode illegal phone:%s", phone)
		return &util.AppError{errIllegalPhone, "请输入正确的手机号"}
	}

	uuid := util.GenUUID()
	resp, rpcerr := callRPC(util.VerifyServerType, 0, "GetCheckCode",
		&verify.PortalLoginRequest{Head: &common.Head{Sid: uuid, Term: term},
			Info: &verify.PortalInfo{Phone: phone, Acname: acname}})
	checkRPCErr(rpcerr, "GetPhoneCode")
	res := resp.Interface().(*common.CommReply)
	checkRPCCode(res.Head.Retcode, "GetPhoneCode")

	req.WriteRsp(w, []byte(`{"errno":0}`))
	reportSuccResp(r.RequestURI)
	return nil
}

func logout(w http.ResponseWriter, r *http.Request) (apperr *util.AppError) {
	var req Request
	req.Init(r)
	uid := req.GetParamInt("uid")
	token := req.GetParamString("token")

	uuid := util.GenUUID()
	resp, rpcerr := callRPC(util.VerifyServerType, uid, "Logout",
		&verify.LogoutRequest{Head: &common.Head{Sid: uuid, Uid: uid},
			Token: token})
	checkRPCErr(rpcerr, "Logout")
	res := resp.Interface().(*common.CommReply)
	checkRPCCode(res.Head.Retcode, "Logout")

	w.Write([]byte(`{"errno":0}`))
	reportSuccResp(r.RequestURI)
	return nil
}

func reportWifi(w http.ResponseWriter, r *http.Request) (apperr *util.AppError) {
	var req Request
	req.InitCheckApp(r)
	uid := req.GetParamInt("uid")
	ssid := req.GetParamString("ssid")
	password := req.GetParamString("password")
	longitude := req.GetParamFloat("longitude")
	latitude := req.GetParamFloat("latitude")

	uuid := util.GenUUID()
	resp, rpcerr := callRPC(util.ModifyServerType, uid, "AddWifi",
		&modify.WifiRequest{Head: &common.Head{Sid: uuid, Uid: uid},
			Info: &common.WifiInfo{Ssid: ssid, Password: password, Longitude: longitude,
				Latitude: latitude}})
	checkRPCErr(rpcerr, "AddWifi")
	res := resp.Interface().(*common.CommReply)
	checkRPCCode(res.Head.Retcode, "AddWifi")

	w.Write([]byte(`{"errno":0}`))
	reportSuccResp(r.RequestURI)
	return nil
}

func connectWifi(w http.ResponseWriter, r *http.Request) (apperr *util.AppError) {
	var req Request
	req.InitCheckApp(r)
	uid := req.GetParamInt("uid")
	acname := req.GetParamString("wlanacname")
	acip := req.GetParamString("wlanacip")
	userip := req.GetParamString("wlanuserip")
	usermac := req.GetParamString("wlanusermac")
	apmac := req.GetParamString("apmac")

	uuid := util.GenUUID()
	resp, rpcerr := callRPC(util.VerifyServerType, uid, "WifiAccess",
		&verify.AccessRequest{Head: &common.Head{Sid: uuid, Uid: uid},
			Info: &verify.PortalInfo{Userip: userip, Usermac: usermac, Acname: acname,
				Acip: acip, Apmac: apmac}})
	checkRPCErr(rpcerr, "WifiAccess")
	res := resp.Interface().(*common.CommReply)
	checkRPCCode(res.Head.Retcode, "WifiAccess")

	w.Write([]byte(`{"errno":0}`))
	reportSuccResp(r.RequestURI)
	return nil
}

func addAddress(w http.ResponseWriter, r *http.Request) (apperr *util.AppError) {
	var req Request
	req.InitCheckApp(r)
	uid := req.GetParamInt("uid")
	province := req.GetParamInt("province")
	city := req.GetParamInt("city")
	zone := req.GetParamInt("zone")
	if province >= maxZipcode || city >= maxZipcode || zone >= maxZipcode {
		return &util.AppError{errInvalidParam, "illegal zipcode"}
	}
	zip := req.GetParamInt("zip")
	detail := req.GetParamString("detail")
	mobile := req.GetParamString("mobile")
	user := req.GetParamString("user")
	addr := req.GetParamString("addr")
	def := req.GetParamBoolDef("def", false)

	uuid := util.GenUUID()
	resp, rpcerr := callRPC(util.ModifyServerType, uid, "AddAddress",
		&modify.AddressRequest{Head: &common.Head{Sid: uuid, Uid: uid},
			Info: &common.AddressInfo{Province: province, City: city,
				Zone: zone, Zip: zip, Addr: addr, Detail: detail,
				Def: def, User: user, Mobile: mobile}})
	checkRPCErr(rpcerr, "AddAddress")
	res := resp.Interface().(*common.CommReply)
	checkRPCCode(res.Head.Retcode, "AddAddress")

	body := genResponseBody(res, false)

	w.Write(body)
	reportSuccResp(r.RequestURI)
	return nil
}

func addShare(w http.ResponseWriter, r *http.Request) (apperr *util.AppError) {
	var req Request
	req.InitCheckApp(r)
	uid := req.GetParamInt("uid")
	bid := req.GetParamInt("bid")
	title := req.GetParamString("title")
	text := req.GetParamString("text")
	images, err := req.Post.Get("data").Get("images").Array()
	if err != nil {
		return &util.AppError{errInvalidParam, err.Error()}
	}
	var imgs []string
	for i := 0; i < len(images); i++ {
		img := images[i].(string)
		imgs = append(imgs, img)
	}

	uuid := util.GenUUID()
	resp, rpcerr := callRPC(util.ModifyServerType, uid, "AddShare",
		&modify.ShareRequest{Head: &common.Head{Sid: uuid, Uid: uid},
			Bid: bid, Title: title, Text: text, Images: imgs})
	checkRPCErr(rpcerr, "AddShare")
	res := resp.Interface().(*common.CommReply)
	checkRPCCode(res.Head.Retcode, "AddShare")

	w.Write([]byte(`{"errno":0}`))
	reportSuccResp(r.RequestURI)
	return nil
}

func setWinStatus(w http.ResponseWriter, r *http.Request) (apperr *util.AppError) {
	var req Request
	req.InitCheckApp(r)
	uid := req.GetParamInt("uid")
	bid := req.GetParamInt("bid")
	status := req.GetParamInt("status")
	aid := req.GetParamIntDef("aid", 0)
	account := req.GetParamStringDef("account", "")

	uuid := util.GenUUID()
	resp, rpcerr := callRPC(util.ModifyServerType, uid, "SetWinStatus",
		&modify.WinStatusRequest{Head: &common.Head{Sid: uuid, Uid: uid},
			Bid: bid, Status: status, Aid: aid, Account: account})
	checkRPCErr(rpcerr, "SetWinStatus")
	res := resp.Interface().(*common.CommReply)
	checkRPCCode(res.Head.Retcode, "SetWinStatus")

	w.Write([]byte(`{"errno":0}`))
	reportSuccResp(r.RequestURI)
	return nil
}

func addFeedback(w http.ResponseWriter, r *http.Request) (apperr *util.AppError) {
	var req Request
	req.InitCheckApp(r)
	uid := req.GetParamInt("uid")
	content := req.GetParamString("content")
	contact := req.GetParamStringDef("contact", "")

	uuid := util.GenUUID()
	resp, rpcerr := callRPC(util.ModifyServerType, uid, "AddFeedback",
		&modify.FeedRequest{Head: &common.Head{Sid: uuid, Uid: uid},
			Content: content, Contact: contact})
	checkRPCErr(rpcerr, "AddFeedback")
	res := resp.Interface().(*common.CommReply)
	checkRPCCode(res.Head.Retcode, "AddFeedback")

	w.Write([]byte(`{"errno":0}`))
	reportSuccResp(r.RequestURI)
	return nil
}

func purchaseSales(w http.ResponseWriter, r *http.Request) (apperr *util.AppError) {
	var req Request
	req.InitCheckApp(r)
	uid := req.GetParamInt("uid")
	bid := req.GetParamInt("bid")

	uuid := util.GenUUID()
	resp, rpcerr := callRPC(util.ModifyServerType, uid, "PurchaseSales",
		&common.CommRequest{Head: &common.Head{Sid: uuid, Uid: uid},
			Id: bid})
	checkRPCErr(rpcerr, "PurchaseSales")
	res := resp.Interface().(*modify.PurchaseReply)
	checkRPCCode(res.Head.Retcode, "PurchaseSales")

	js, err := simplejson.NewJson([]byte(`{"errno":0}`))
	if err != nil {
		return &util.AppError{errInner, err.Error()}
	}
	js.Set("data", res.Info)
	body, err := js.MarshalJSON()
	if err != nil {
		return &util.AppError{errInner, "marshal json failed"}
	}

	w.Write(body)
	reportSuccResp(r.RequestURI)
	return nil
}

func modAddress(w http.ResponseWriter, r *http.Request) (apperr *util.AppError) {
	var req Request
	req.InitCheckApp(r)
	uid := req.GetParamInt("uid")
	aid := req.GetParamInt("aid")
	province := req.GetParamInt("province")
	city := req.GetParamInt("city")
	zone := req.GetParamInt("zone")
	if province >= maxZipcode || city >= maxZipcode || zone >= maxZipcode {
		return &util.AppError{errInvalidParam, "illegal zipcode"}
	}
	zip := req.GetParamInt("zip")
	detail := req.GetParamString("detail")
	mobile := req.GetParamString("mobile")
	user := req.GetParamString("user")
	addr := req.GetParamString("addr")
	def := req.GetParamBoolDef("def", false)

	uuid := util.GenUUID()
	resp, rpcerr := callRPC(util.ModifyServerType, uid, "ModAddress",
		&modify.AddressRequest{Head: &common.Head{Sid: uuid, Uid: uid},
			Info: &common.AddressInfo{Aid: aid, Province: province,
				City: city, Zone: zone, Zip: zip, Addr: addr,
				Detail: detail, Def: def, User: user, Mobile: mobile}})
	checkRPCErr(rpcerr, "ModAddress")
	res := resp.Interface().(*common.CommReply)
	checkRPCCode(res.Head.Retcode, "ModAddress")

	w.Write([]byte(`{"errno":0}`))
	reportSuccResp(r.RequestURI)
	return nil
}

func delAddress(w http.ResponseWriter, r *http.Request) (apperr *util.AppError) {
	var req Request
	req.InitCheckApp(r)
	uid := req.GetParamInt("uid")
	aid := req.GetParamInt("aid")

	uuid := util.GenUUID()
	resp, rpcerr := callRPC(util.ModifyServerType, uid, "DelAddress",
		&modify.AddressRequest{Head: &common.Head{Sid: uuid, Uid: uid},
			Info: &common.AddressInfo{Aid: aid}})
	checkRPCErr(rpcerr, "DelAddress")
	res := resp.Interface().(*common.CommReply)
	checkRPCCode(res.Head.Retcode, "DelAddress")

	w.Write([]byte(`{"errno":0}`))
	reportSuccResp(r.RequestURI)
	return nil
}

func applyImageUpload(w http.ResponseWriter, r *http.Request) (apperr *util.AppError) {
	var req Request
	req.InitCheckApp(r)
	uid := req.GetParamInt("uid")
	format := req.GetParamString("format")

	fname := util.GenUUID() + "." + format
	var names = []string{fname}
	err := addImages(uid, names)
	if err != nil {
		return &util.AppError{errInner, err.Error()}
	}

	js, err := simplejson.NewJson([]byte(`{"errno":0}`))
	if err != nil {
		return &util.AppError{errInner, err.Error()}
	}
	data, err := simplejson.NewJson([]byte(`{}`))
	if err != nil {
		return &util.AppError{errInner, err.Error()}
	}
	aliyun.FillCallbackInfo(data)
	data.Set("name", fname)
	js.Set("data", data)

	body, err := js.MarshalJSON()
	if err != nil {
		return &util.AppError{errInner, "marshal json failed"}
	}
	w.Write(body)
	reportSuccResp(r.RequestURI)
	return nil
}

func pingppPay(w http.ResponseWriter, r *http.Request) (apperr *util.AppError) {
	var req Request
	req.InitCheckApp(r)
	uid := req.GetParamInt("uid")
	amount := req.GetParamInt("amount")
	channel := req.GetParamString("channel")
	log.Printf("pingppPay uid:%d amount:%d channel:%s", uid, amount, channel)

	res := pay.GetPingPPCharge(int(amount), channel)
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(res))
	reportSuccResp(r.RequestURI)
	return nil
}

func reportApmac(w http.ResponseWriter, r *http.Request) (apperr *util.AppError) {
	var req Request
	req.InitCheckApp(r)
	uid := req.GetParamInt("uid")
	apmac := req.GetParamString("apmac")
	log.Printf("report_apmac uid:%d apmac:%s\n", uid, apmac)

	uuid := util.GenUUID()
	resp, rpcerr := callRPC(util.ModifyServerType, uid, "ReportApmac",
		&modify.ApmacRequest{Head: &common.Head{Sid: uuid, Uid: uid},
			Apmac: apmac})
	checkRPCErr(rpcerr, "ReportApmac")
	res := resp.Interface().(*common.CommReply)
	checkRPCCode(res.Head.Retcode, "ReportApmac")

	w.Write([]byte(`{"errno":0}`))
	reportSuccResp(r.RequestURI)
	return nil
}

func uploadCallback(w http.ResponseWriter, r *http.Request) (apperr *util.AppError) {
	r.ParseForm()
	reportRequest(r.RequestURI)
	fname := r.Form["filename"]
	if len(fname) < 1 {
		log.Printf("parse filename failed\n")
		w.Write([]byte(`{"Status":"OK"}`))
		return nil
	}
	size := r.Form["size"]
	fsize, _ := strconv.ParseInt(size[0], 10, 64)
	height := r.Form["height"]
	fheight, _ := strconv.ParseInt(height[0], 10, 64)
	width := r.Form["width"]
	fwidth, _ := strconv.ParseInt(width[0], 10, 64)
	log.Printf("upload_callback fname:%s size:%d height:%d width:%d\n", fname, fsize,
		fheight, fwidth)

	uuid := util.GenUUID()
	resp, rpcerr := callRPC(util.ModifyServerType, 0, "FinImage",
		&modify.ImageRequest{Head: &common.Head{Sid: uuid},
			Info: &modify.ImageInfo{Name: fname[0], Size: fsize,
				Height: fheight, Width: fwidth}})
	checkRPCErr(rpcerr, "FinImage")
	res := resp.Interface().(*common.CommReply)
	checkRPCCode(res.Head.Retcode, "FinImage")

	w.Write([]byte(`{"Status":"OK"}`))
	reportSuccResp(r.RequestURI)
	return nil
}

func reportClick(w http.ResponseWriter, r *http.Request) (apperr *util.AppError) {
	var req Request
	req.InitCheckApp(r)
	uid := req.GetParamInt("uid")
	id := req.GetParamIntDef("id", 0)
	ctype := req.GetParamInt("type")
	name := req.GetParamStringDef("name", "")
	log.Printf("reportClick uid:%d type:%d id:%d name:%s", uid, ctype, id, name)

	uuid := util.GenUUID()
	resp, rpcerr := callRPC(util.ModifyServerType, uid, "ReportClick",
		&modify.ClickRequest{Head: &common.Head{Sid: uuid, Uid: uid},
			Id: id, Type: ctype, Name: name})
	checkRPCErr(rpcerr, "ReportClick")
	res := resp.Interface().(*common.CommReply)
	checkRPCCode(res.Head.Retcode, "ReportClick")

	w.Write([]byte(`{"errno":0}`))
	reportSuccResp(r.RequestURI)
	return nil
}

func fetchWifi(w http.ResponseWriter, r *http.Request) (apperr *util.AppError) {
	var req Request
	req.InitCheckApp(r)
	uid := req.GetParamInt("uid")
	longitude := req.GetParamFloat("longitude")
	latitude := req.GetParamFloat("latitude")

	uuid := util.GenUUID()
	resp, rpcerr := callRPC(util.FetchServerType, uid, "FetchWifi",
		&fetch.WifiRequest{Head: &common.Head{Sid: uuid, Uid: uid},
			Longitude: longitude, Latitude: latitude})
	checkRPCErr(rpcerr, "FetchWifi")
	res := resp.Interface().(*common.CommReply)
	checkRPCCode(res.Head.Retcode, "FetchWifi")

	body := genResponseBody(res, false)
	w.Write(body)
	reportSuccResp(r.RequestURI)
	return nil
}

func checkUpdate(w http.ResponseWriter, r *http.Request) (apperr *util.AppError) {
	var req Request
	req.InitCheckApp(r)
	uid := req.GetParamInt("uid")
	term := req.GetParamInt("term")
	version := req.GetParamInt("version")
	channel := req.GetParamString("channel")

	uuid := util.GenUUID()
	resp, rpcerr := callRPC(util.FetchServerType, uid, "FetchLatestVersion",
		&fetch.VersionRequest{
			Head:    &common.Head{Sid: uuid, Uid: uid, Term: term, Version: version},
			Channel: channel})
	checkRPCErr(rpcerr, "FetchLatestVersion")
	res := resp.Interface().(*fetch.VersionReply)
	checkRPCCode(res.Head.Retcode, "FetchLatestVersion")

	body := genResponseBody(res, false)
	w.Write(body)
	reportSuccResp(r.RequestURI)
	return nil
}

func checkLogin(w http.ResponseWriter, r *http.Request) (apperr *util.AppError) {
	var req Request
	req.Init(r)
	usermac := req.GetParamString("wlanusermac")
	acname := req.GetParamString("wlanacname")
	log.Printf("checkLogin usermac:%s", usermac)

	uuid := util.GenUUID()
	resp, rpcerr := callRPC(util.VerifyServerType, 0, "CheckLogin",
		&verify.AccessRequest{
			Head: &common.Head{Sid: uuid},
			Info: &verify.PortalInfo{Usermac: usermac, Acname: acname}})
	checkRPCErr(rpcerr, "FetchLatestVersion")
	res := resp.Interface().(*verify.CheckReply)
	checkRPCCode(res.Head.Retcode, "FetchLatestVersion")

	body := genResponseBody(res, false)
	req.WriteRsp(w, body)
	reportSuccResp(r.RequestURI)
	return nil
}

func getFrontInfo(w http.ResponseWriter, r *http.Request) (apperr *util.AppError) {
	var req Request
	req.InitCheckApp(r)
	uid := req.GetParamInt("uid")

	uuid := util.GenUUID()
	resp, rpcerr := callRPC(util.HotServerType, uid, "GetFrontInfo",
		&common.CommRequest{Head: &common.Head{Sid: uuid, Uid: uid}})
	checkRPCErr(rpcerr, "GetFrontInfo")
	res := resp.Interface().(*hot.FrontReply)
	checkRPCCode(res.Head.Retcode, "GetFrontInfo")

	body := genResponseBody(res, false)
	w.Write(body)
	reportSuccResp(r.RequestURI)
	return nil
}

func getFlashAd(w http.ResponseWriter, r *http.Request) (apperr *util.AppError) {
	var req Request
	req.InitCheckApp(r)
	uid := req.GetParamInt("uid")
	version := req.GetParamInt("version")
	term := req.GetParamInt("term")

	uuid := util.GenUUID()
	resp, rpcerr := callRPC(util.FetchServerType, uid, "FetchFlashAd",
		&fetch.AdRequest{Head: &common.Head{Sid: uuid, Uid: uid},
			Term: term, Version: version})
	checkRPCErr(rpcerr, "GetFlashAd")
	res := resp.Interface().(*fetch.AdReply)
	checkRPCCode(res.Head.Retcode, "GetFlashAd")

	js, err := simplejson.NewJson([]byte(`{"errno":0}`))
	if err != nil {
		return &util.AppError{errInner, "invalid param"}
	}
	if res.Info != nil && res.Info.Img != "" {
		js.Set("data", res.Info)
	}

	body, err := js.MarshalJSON()
	if err != nil {
		return &util.AppError{errInner, "marshal json failed"}
	}
	w.Write(body)
	reportSuccResp(r.RequestURI)
	return nil
}

func getOpening(w http.ResponseWriter, r *http.Request) (apperr *util.AppError) {
	var req Request
	req.InitCheckApp(r)
	uid := req.GetParamInt("uid")

	uuid := util.GenUUID()
	resp, rpcerr := callRPC(util.HotServerType, uid, "GetOpening",
		&common.CommRequest{Head: &common.Head{Sid: uuid, Uid: uid}})
	checkRPCErr(rpcerr, "GetOpening")
	res := resp.Interface().(*hot.OpeningReply)
	checkRPCCode(res.Head.Retcode, "GetOpening")

	body := genResponseBody(res, false)
	w.Write(body)
	reportSuccResp(r.RequestURI)
	return nil
}

func getOpened(w http.ResponseWriter, r *http.Request) (apperr *util.AppError) {
	var req Request
	req.InitCheckApp(r)
	uid := req.GetParamInt("uid")
	seq := req.GetParamInt("seq")
	num := req.GetParamInt("num")

	uuid := util.GenUUID()
	resp, rpcerr := callRPC(util.HotServerType, uid, "GetOpened",
		&common.CommRequest{Head: &common.Head{Sid: uuid, Uid: uid},
			Seq: seq, Num: num})
	checkRPCErr(rpcerr, "GetOpened")
	res := resp.Interface().(*hot.OpenedReply)
	checkRPCCode(res.Head.Retcode, "GetOpened")

	js, err := simplejson.NewJson([]byte(`{"errno":0}`))
	if err != nil {
		return &util.AppError{errInner, "invalid param"}
	}
	js.SetPath([]string{"data", "opened"}, res.Opened)
	if len(res.Opened) >= int(num) {
		js.SetPath([]string{"data", "hasmore"}, 1)
	}

	body, err := js.MarshalJSON()
	if err != nil {
		return &util.AppError{errInner, "marshal json failed"}
	}
	w.Write(body)
	reportSuccResp(r.RequestURI)
	return nil
}

func getRunning(w http.ResponseWriter, r *http.Request) (apperr *util.AppError) {
	var req Request
	req.InitCheckApp(r)
	uid := req.GetParamInt("uid")
	seq := req.GetParamInt("seq")
	num := req.GetParamInt("num")

	uuid := util.GenUUID()
	resp, rpcerr := callRPC(util.HotServerType, uid, "GetRunning",
		&common.CommRequest{Head: &common.Head{Sid: uuid, Uid: uid},
			Seq: seq, Num: num})
	checkRPCErr(rpcerr, "GetRunning")
	res := resp.Interface().(*hot.RunningReply)
	checkRPCCode(res.Head.Retcode, "GetRunning")

	js, err := simplejson.NewJson([]byte(`{"errno":0}`))
	if err != nil {
		return &util.AppError{errInner, "invalid param"}
	}
	js.SetPath([]string{"data", "running"}, res.Running)
	if len(res.Running) >= int(num) {
		js.SetPath([]string{"data", "hasmore"}, 1)
	}

	body, err := js.MarshalJSON()
	if err != nil {
		return &util.AppError{errInner, "marshal json failed"}
	}
	w.Write(body)
	reportSuccResp(r.RequestURI)
	return nil
}

func getMarquee(w http.ResponseWriter, r *http.Request) (apperr *util.AppError) {
	var req Request
	req.InitCheckApp(r)
	uid := req.GetParamInt("uid")

	uuid := util.GenUUID()
	resp, rpcerr := callRPC(util.HotServerType, uid, "GetMarquee",
		&common.CommRequest{Head: &common.Head{Sid: uuid, Uid: uid}})
	checkRPCErr(rpcerr, "GetMarquee")
	res := resp.Interface().(*hot.MarqueeReply)
	checkRPCCode(res.Head.Retcode, "GetMarquee")

	body := genResponseBody(res, false)
	w.Write(body)
	reportSuccResp(r.RequestURI)
	return nil
}

func getHotList(w http.ResponseWriter, r *http.Request) (apperr *util.AppError) {
	var req Request
	req.InitCheckApp(r)
	uid := req.GetParamInt("uid")

	uuid := util.GenUUID()
	resp, rpcerr := callRPC(util.HotServerType, uid, "GetHotList",
		&common.CommRequest{Head: &common.Head{Sid: uuid, Uid: uid}})
	checkRPCErr(rpcerr, "GetHotList")
	res := resp.Interface().(*hot.HotListReply)
	checkRPCCode(res.Head.Retcode, "GetHotList")

	body := genResponseBody(res, false)
	w.Write(body)
	reportSuccResp(r.RequestURI)
	return nil
}

func getWifiPass(w http.ResponseWriter, r *http.Request) (apperr *util.AppError) {
	var req Request
	req.InitCheckApp(r)
	uid := req.GetParamInt("uid")
	longitude := req.GetParamFloat("longitude")
	latitude := req.GetParamFloat("latitude")
	ssids, err := req.Post.Get("data").Get("ssids").Array()
	if err != nil {
		return &util.AppError{errInner, err.Error()}
	}
	var ids []string
	if len(ssids) == 0 {
		return &util.AppError{errInvalidParam, "illegal param:empty ssids"}
	}
	for i := 0; i < len(ssids); i++ {
		ssid := ssids[i].(string)
		ids = append(ids, ssid)
	}

	uuid := util.GenUUID()
	resp, rpcerr := callRPC(util.FetchServerType, uid, "FetchWifiPass",
		&fetch.WifiPassRequest{
			Head:      &common.Head{Sid: uuid, Uid: uid},
			Longitude: longitude,
			Latitude:  latitude,
			Ssids:     ids})
	checkRPCErr(rpcerr, "FetchWifiPass")
	res := resp.Interface().(*fetch.WifiPassReply)
	checkRPCCode(res.Head.Retcode, "FetchWifiPass")

	body := genResponseBody(res, false)
	w.Write(body)
	reportSuccResp(r.RequestURI)
	return nil
}

func getShare(w http.ResponseWriter, r *http.Request) (apperr *util.AppError) {
	var req Request
	req.InitCheckApp(r)
	uid := req.GetParamInt("uid")
	gid := req.GetParamIntDef("gid", 0)
	seq := req.GetParamInt("seq")
	num := req.GetParamIntDef("num", util.MaxListSize)
	path := r.URL.Path
	log.Printf("path:%s", path)
	var stype int64
	if path == "/get_share_gid" {
		stype = util.GidShareType
	} else if path == "/get_share_list" {
		stype = util.ListShareType
	} else if path == "/get_share_uid" {
		stype = util.UidShareType
	}

	uuid := util.GenUUID()
	resp, rpcerr := callRPC(util.FetchServerType, uid, "FetchShare",
		&fetch.ShareRequest{
			Head: &common.Head{Sid: uuid, Uid: uid},
			Type: stype, Seq: seq, Num: num, Id: gid})
	checkRPCErr(rpcerr, "FetchShare")
	res := resp.Interface().(*fetch.ShareReply)
	checkRPCCode(res.Head.Retcode, "FetchShare")

	body := genResponseBody(res, false)
	w.Write(body)
	reportSuccResp(r.RequestURI)
	return nil
}

func getShareDetail(w http.ResponseWriter, r *http.Request) (apperr *util.AppError) {
	var req Request
	req.InitCheckApp(r)
	uid := req.GetParamInt("uid")
	sid := req.GetParamInt("sid")

	uuid := util.GenUUID()
	resp, rpcerr := callRPC(util.FetchServerType, uid, "FetchShareDetail",
		&common.CommRequest{
			Head: &common.Head{Sid: uuid, Uid: uid},
			Id:   sid})
	checkRPCErr(rpcerr, "FetchShareDetail")
	res := resp.Interface().(*fetch.ShareDetailReply)
	checkRPCCode(res.Head.Retcode, "FetchShareDetail")

	body := genResponseBody(res, false)
	w.Write(body)
	reportSuccResp(r.RequestURI)
	return nil
}

func getDetail(w http.ResponseWriter, r *http.Request) (apperr *util.AppError) {
	var req Request
	req.InitCheckApp(r)
	uid := req.GetParamInt("uid")
	gid := req.GetParamIntDef("gid", 0)
	bid := req.GetParamIntDef("bid", 0)
	if gid == 0 && bid == 0 {
		return &util.AppError{errInvalidParam, "invalid param"}
	}

	uuid := util.GenUUID()
	resp, rpcerr := callRPC(util.HotServerType, uid, "GetDetail",
		&hot.DetailRequest{
			Head: &common.Head{Sid: uuid, Uid: uid},
			Bid:  bid, Gid: gid})
	checkRPCErr(rpcerr, "GetDetail")
	res := resp.Interface().(*hot.DetailReply)
	checkRPCCode(res.Head.Retcode, "GetDetail")

	body := genResponseBody(res, false)
	w.Write(body)
	reportSuccResp(r.RequestURI)
	return nil
}

func getImageToken(w http.ResponseWriter, r *http.Request) (apperr *util.AppError) {
	var req Request
	req.InitCheckApp(r)
	uid := req.GetParamInt("uid")

	uuid := util.GenUUID()
	resp, rpcerr := callRPC(util.FetchServerType, uid, "FetchStsCredentials",
		&common.CommRequest{
			Head: &common.Head{Sid: uuid, Uid: uid}})
	checkRPCErr(rpcerr, "FetchStsCredentials")
	res := resp.Interface().(*fetch.StsReply)
	checkRPCCode(res.Head.Retcode, "FetchStsCredentials")

	js, err := simplejson.NewJson([]byte(`{"errno":0}`))
	if err != nil {
		return &util.AppError{errInner, "invalid param"}
	}
	js.Set("data", res.Credential)

	body, err := js.MarshalJSON()
	if err != nil {
		return &util.AppError{errInner, "marshal json failed"}
	}
	w.Write(body)
	reportSuccResp(r.RequestURI)
	return nil
}

func getWeatherNews(w http.ResponseWriter, r *http.Request) (apperr *util.AppError) {
	var req Request
	req.InitCheckApp(r)
	uid := req.GetParamInt("uid")
	response, err := getRspFromSSDB(hotWeatherKey)
	if err == nil {
		log.Printf("getRspFromSSDB succ key:%s\n", hotWeatherKey)
		rspGzip(w, []byte(response))
		reportSuccResp(r.RequestURI)
		return nil
	}

	uuid := util.GenUUID()
	resp, rpcerr := callRPC(util.HotServerType, uid, "GetWeatherNews",
		&common.CommRequest{Head: &common.Head{Sid: uuid, Uid: uid}})
	checkRPCErr(rpcerr, "GetWeatherNews")
	res := resp.Interface().(*hot.WeatherNewsReply)
	checkRPCCode(res.Head.Retcode, "GetWeatherNews")

	js, err := simplejson.NewJson([]byte(`{"errno":0}`))
	if err != nil {
		return &util.AppError{errInner, "invalid param"}
	}
	js.SetPath([]string{"data", "news"}, res.News)
	js.SetPath([]string{"data", "weather"}, res.Weather)
	js.SetPath([]string{"data", "notice"}, res.Notice)

	body, err := js.MarshalJSON()
	if err != nil {
		return &util.AppError{errInner, "marshal json failed"}
	}
	rspGzip(w, body)
	data := js.Get("data")
	setSSDBCache(hotWeatherKey, data)
	reportSuccResp(r.RequestURI)
	return nil
}

func getLiveInfo(w http.ResponseWriter, r *http.Request) (apperr *util.AppError) {
	var req Request
	req.InitCheckApp(r)
	uid := req.GetParamInt("uid")
	seq := req.GetParamInt("seq")

	uuid := util.GenUUID()
	resp, rpcerr := callRPC(util.HotServerType, uid, "GetLive",
		&common.CommRequest{Head: &common.Head{Sid: uuid, Uid: uid}, Seq: seq})
	checkRPCErr(rpcerr, "GetLive")
	res := resp.Interface().(*hot.LiveReply)
	checkRPCCode(res.Head.Retcode, "GetLive")

	js, err := simplejson.NewJson([]byte(`{"errno":0}`))
	if err != nil {
		return &util.AppError{errInner, "invalid param"}
	}
	js.SetPath([]string{"data", "list"}, res.List)
	if len(res.List) >= util.MaxListSize {
		js.SetPath([]string{"data", "hasmore"}, 1)
	}

	body, err := js.MarshalJSON()
	if err != nil {
		return &util.AppError{errInner, "marshal json failed"}
	}
	rspGzip(w, body)
	reportSuccResp(r.RequestURI)
	return nil
}

func getJokes(w http.ResponseWriter, r *http.Request) (apperr *util.AppError) {
	var req Request
	req.InitCheckApp(r)
	uid := req.GetParamInt("uid")
	seq := req.GetParamInt("seq")

	uuid := util.GenUUID()
	resp, rpcerr := callRPC(util.HotServerType, uid, "GetJoke",
		&common.CommRequest{Head: &common.Head{Sid: uuid, Uid: uid}, Seq: seq})
	checkRPCErr(rpcerr, "GetJoke")
	res := resp.Interface().(*hot.JokeReply)
	checkRPCCode(res.Head.Retcode, "GetJoke")

	js, err := simplejson.NewJson([]byte(`{"errno":0}`))
	if err != nil {
		return &util.AppError{errInner, "invalid param"}
	}
	js.SetPath([]string{"data", "infos"}, res.Infos)
	if len(res.Infos) >= util.MaxListSize {
		js.SetPath([]string{"data", "hasmore"}, 1)
	}

	body, err := js.MarshalJSON()
	if err != nil {
		return &util.AppError{errInner, "marshal json failed"}
	}
	rspGzip(w, body)
	reportSuccResp(r.RequestURI)
	return nil
}

func getZipcode(w http.ResponseWriter, r *http.Request) (apperr *util.AppError) {
	var req Request
	req.InitCheckApp(r)
	uid := req.GetParamInt("uid")
	ziptype := req.GetParamInt("type")
	code := req.GetParamInt("code")

	uuid := util.GenUUID()
	resp, rpcerr := callRPC(util.FetchServerType, uid, "FetchZipcode",
		&fetch.ZipcodeRequest{Head: &common.Head{Sid: uuid, Uid: uid},
			Type: ziptype, Code: code})
	checkRPCErr(rpcerr, "FetchZipcode")
	res := resp.Interface().(*fetch.ZipcodeReply)
	checkRPCCode(res.Head.Retcode, "FetchZipcode")

	body := genResponseBody(res, false)
	w.Write(body)
	reportSuccResp(r.RequestURI)
	return nil
}

func getActivity(w http.ResponseWriter, r *http.Request) (apperr *util.AppError) {
	var req Request
	req.InitCheckApp(r)
	uid := req.GetParamInt("uid")

	uuid := util.GenUUID()
	resp, rpcerr := callRPC(util.FetchServerType, uid, "FetchActivity",
		&common.CommRequest{Head: &common.Head{Sid: uuid, Uid: uid}})
	checkRPCErr(rpcerr, "FetchActivity")
	res := resp.Interface().(*fetch.ActivityReply)
	checkRPCCode(res.Head.Retcode, "FetchActivity")

	js, err := simplejson.NewJson([]byte(`{"errno":0}`))
	if err != nil {
		return &util.AppError{errInner, "invalid param"}
	}
	js.Set("data", res.Activity)

	body, err := js.MarshalJSON()
	if err != nil {
		return &util.AppError{errInner, "marshal json failed"}
	}
	w.Write(body)
	reportSuccResp(r.RequestURI)
	return nil
}

func getGoodsIntro(w http.ResponseWriter, r *http.Request) (apperr *util.AppError) {
	var req Request
	req.InitCheckApp(r)
	uid := req.GetParamInt("uid")
	gid := req.GetParamInt("gid")

	uuid := util.GenUUID()
	resp, rpcerr := callRPC(util.FetchServerType, uid, "FetchGoodsIntro",
		&common.CommRequest{Head: &common.Head{Sid: uuid, Uid: uid},
			Id: gid})
	checkRPCErr(rpcerr, "FetchGoodsIntro")
	res := resp.Interface().(*fetch.GoodsIntroReply)
	checkRPCCode(res.Head.Retcode, "FetchGoodsIntro")

	js, err := simplejson.NewJson([]byte(`{"errno":0}`))
	if err != nil {
		return &util.AppError{errInner, "invalid param"}
	}
	js.Set("data", res.Info)

	body, err := js.MarshalJSON()
	if err != nil {
		return &util.AppError{errInner, "marshal json failed"}
	}
	w.Write(body)
	reportSuccResp(r.RequestURI)
	return nil
}

func getBetHistory(w http.ResponseWriter, r *http.Request) (apperr *util.AppError) {
	var req Request
	req.InitCheckApp(r)
	uid := req.GetParamInt("uid")
	gid := req.GetParamInt("gid")
	seq := req.GetParamInt("seq")
	num := req.GetParamInt("num")

	uuid := util.GenUUID()
	resp, rpcerr := callRPC(util.FetchServerType, uid, "FetchBetHistory",
		&common.CommRequest{Head: &common.Head{Sid: uuid, Uid: uid},
			Seq: seq, Num: num, Id: gid})
	checkRPCErr(rpcerr, "FetchBetHistory")
	res := resp.Interface().(*fetch.BetHistoryReply)
	checkRPCCode(res.Head.Retcode, "FetchBetHistory")

	body := genResponseBody(res, false)
	w.Write(body)
	reportSuccResp(r.RequestURI)
	return nil
}

func getPurchaseRecord(w http.ResponseWriter, r *http.Request) (apperr *util.AppError) {
	var req Request
	req.InitCheckApp(r)
	uid := req.GetParamInt("uid")
	bid := req.GetParamInt("bid")
	seq := req.GetParamInt("seq")
	num := req.GetParamInt("num")

	uuid := util.GenUUID()
	resp, rpcerr := callRPC(util.FetchServerType, uid, "FetchPurchaseRecord",
		&common.CommRequest{Head: &common.Head{Sid: uuid, Uid: uid},
			Seq: seq, Num: num, Id: bid})
	checkRPCErr(rpcerr, "FetchPurchaseRecord")
	res := resp.Interface().(*fetch.PurchaseRecordReply)
	checkRPCCode(res.Head.Retcode, "FetchPurchaseRecord")

	body := genResponseBody(res, false)
	w.Write(body)
	reportSuccResp(r.RequestURI)
	return nil
}

func getUserBet(w http.ResponseWriter, r *http.Request) (apperr *util.AppError) {
	var req Request
	req.InitCheckApp(r)
	uid := req.GetParamInt("uid")
	seq := req.GetParamInt("seq")
	num := req.GetParamInt("num")
	path := r.URL.Path
	var stype int64
	if path == "/get_user_award" {
		stype = util.UserAwardType
	} else {
		stype = util.UserBetType
	}

	uuid := util.GenUUID()
	resp, rpcerr := callRPC(util.FetchServerType, uid, "FetchUserBet",
		&common.CommRequest{Head: &common.Head{Sid: uuid, Uid: uid},
			Seq: seq, Num: num, Type: stype})
	checkRPCErr(rpcerr, "FetchUserBet")
	res := resp.Interface().(*fetch.UserBetReply)
	checkRPCCode(res.Head.Retcode, "FetchUserBet")

	js, err := simplejson.NewJson([]byte(`{"errno":0}`))
	if err != nil {
		return &util.AppError{errInner, "invalid param"}
	}
	js.SetPath([]string{"data", "infos"}, res.Infos)
	if len(res.Infos) >= util.MaxListSize {
		js.SetPath([]string{"data", "hasmore"}, 1)
	}

	body, err := js.MarshalJSON()
	if err != nil {
		return &util.AppError{errInner, "marshal json failed"}
	}
	w.Write(body)
	reportSuccResp(r.RequestURI)
	return nil
}

func getKvConf(w http.ResponseWriter, r *http.Request) (apperr *util.AppError) {
	var req Request
	req.InitCheckApp(r)
	uid := req.GetParamInt("uid")
	key := req.GetParamString("key")

	uuid := util.GenUUID()
	resp, rpcerr := callRPC(util.FetchServerType, uid, "FetchKvConf",
		&fetch.KvRequest{Head: &common.Head{Sid: uuid, Uid: uid},
			Key: key})
	checkRPCErr(rpcerr, "FetchKvConf")
	res := resp.Interface().(*fetch.KvReply)
	checkRPCCode(res.Head.Retcode, "FetchKvConf")

	body := genResponseBody(res, false)
	w.Write(body)
	reportSuccResp(r.RequestURI)
	return nil
}

func getMenu(w http.ResponseWriter, r *http.Request) (apperr *util.AppError) {
	var req Request
	req.InitCheckApp(r)
	uid := req.GetParamInt("uid")
	term := req.GetParamInt("term")
	version := req.GetParamInt("version")

	uuid := util.GenUUID()
	resp, rpcerr := callRPC(util.FetchServerType, uid, "FetchMenu",
		&common.CommRequest{Head: &common.Head{Sid: uuid, Uid: uid,
			Term: term, Version: version}})
	checkRPCErr(rpcerr, "FetchMenu")
	res := resp.Interface().(*fetch.MenuReply)
	checkRPCCode(res.Head.Retcode, "FetchMenu")

	body := genResponseBody(res, false)
	w.Write(body)
	reportSuccResp(r.RequestURI)
	return nil
}

func getAddress(w http.ResponseWriter, r *http.Request) (apperr *util.AppError) {
	var req Request
	req.InitCheckApp(r)
	uid := req.GetParamInt("uid")

	uuid := util.GenUUID()
	resp, rpcerr := callRPC(util.FetchServerType, uid, "FetchAddress",
		&common.CommRequest{Head: &common.Head{Sid: uuid, Uid: uid}})
	checkRPCErr(rpcerr, "FetchAddress")
	res := resp.Interface().(*fetch.AddressReply)
	checkRPCCode(res.Head.Retcode, "FetchAddress")

	body := genResponseBody(res, false)
	w.Write(body)
	reportSuccResp(r.RequestURI)
	return nil
}

func getWinStatus(w http.ResponseWriter, r *http.Request) (apperr *util.AppError) {
	var req Request
	req.InitCheckApp(r)
	uid := req.GetParamInt("uid")
	bid := req.GetParamInt("bid")

	uuid := util.GenUUID()
	resp, rpcerr := callRPC(util.FetchServerType, uid, "FetchWinStatus",
		&common.CommRequest{Head: &common.Head{Sid: uuid, Uid: uid},
			Id: bid})
	checkRPCErr(rpcerr, "FetchWinStatus")
	res := resp.Interface().(*fetch.WinStatusReply)
	checkRPCCode(res.Head.Retcode, "FetchWinStatus")

	body := genResponseBody(res, false)
	w.Write(body)
	reportSuccResp(r.RequestURI)
	return nil
}

func genSsdbKey(ctype int64, newFlag bool) string {
	switch ctype {
	default:
		if newFlag {
			return hotNewsKey
		}
		return hotNewsCompKey
	case hotVideoType:
		return hotVideoKey
	case hotDgType:
		return hotDgNewsKey
	case hotAmuseType:
		return hotAmuseKey
	case hotJokeType:
		return hotJokeKey
	}
}

func getRspFromSSDB(key string) (string, error) {
	val, err := util.GetSSDBVal(key)
	if err != nil {
		log.Printf("getRspFromSSDB GetSSDBVal key:%s failed:%v", key, err)
		return "", err
	}
	js, err := simplejson.NewJson([]byte(val))
	if err != nil {
		log.Printf("getRspFromSSDB parse json failed:%v", err)
		return "", err
	}
	expire, err := js.Get("expire").Int64()
	if err != nil {
		log.Printf("getRspFromSSDB get expire failed:%v", err)
		return "", err
	}
	if time.Now().Unix() > expire {
		log.Printf("getRspFromSSDB data expire :%d", expire)
		return "", errors.New("ssdb data expired")
	}
	rsp, err := simplejson.NewJson([]byte(`{"errno":0}`))
	if err != nil {
		log.Printf("getRspFromSSDB NewJson failed:%v", err)
		return "", err
	}
	data := js.Get("data")
	rsp.Set("data", data)

	body, err := rsp.MarshalJSON()
	if err != nil {
		log.Printf("getRspFromSSDB MarshalJson failed:%v", err)
		return "", err
	}

	return string(body), nil
}

func setSSDBCache(key string, data *simplejson.Json) {
	expire := time.Now().Unix() + expireInterval
	js, err := simplejson.NewJson([]byte(`{}`))
	if err != nil {
		log.Printf("setSSDBCache key:%s NewJson failed:%v\n", key, err)
		return
	}
	js.Set("expire", expire)
	js.Set("data", data)
	body, err := js.MarshalJSON()
	if err != nil {
		log.Printf("setSSDBCache MarshalJson failed:%v", err)
		return
	}
	util.SetSSDBVal(key, string(body))
	return
}

func getHot(w http.ResponseWriter, r *http.Request) (apperr *util.AppError) {
	var req Request
	req.InitCheckApp(r)
	uid := req.GetParamInt("uid")
	ctype := req.GetParamInt("type")
	term := req.GetParamInt("term")
	version := req.GetParamInt("version")
	seq := req.GetParamInt("seq")
	log.Printf("uid:%d ctype:%d seq:%d term:%d version:%d\n", uid, ctype, seq, term, version)
	if seq == 0 {
		flag := util.CheckTermVersion(term, version)
		key := genSsdbKey(ctype, flag)
		log.Printf("key:%s", key)
		resp, err := getRspFromSSDB(key)
		if err == nil {
			log.Printf("getRspFromSSDB succ key:%s\n", key)
			rspGzip(w, []byte(resp))
			reportSuccResp(r.RequestURI)
			return nil
		}
		log.Printf("getRspFromSSDB failed key:%s err:%v\n", key, err)
	}

	uuid := util.GenUUID()
	resp, rpcerr := callRPC(util.HotServerType, uid, "GetHots",
		&common.CommRequest{Head: &common.Head{Sid: uuid, Uid: uid, Term: term, Version: version},
			Type: ctype, Seq: seq})
	checkRPCErr(rpcerr, "GetHots")
	res := resp.Interface().(*hot.HotsReply)
	checkRPCCode(res.Head.Retcode, "GetHots")

	js, err := simplejson.NewJson([]byte(`{"errno":0}`))
	if err != nil {
		return &util.AppError{errInner, "invalid param"}
	}
	js.SetPath([]string{"data", "infos"}, res.Infos)
	if len(res.Infos) >= util.MaxListSize ||
		(seq == 0 && ctype == 0 && len(res.Infos) >= util.MaxListSize/2) {
		js.SetPath([]string{"data", "hasmore"}, 1)
	}

	body, err := js.MarshalJSON()
	if err != nil {
		return &util.AppError{errInner, "marshal json failed"}
	}
	rspGzip(w, body)
	if seq == 0 {
		flag := util.CheckTermVersion(term, version)
		key := genSsdbKey(ctype, flag)
		data := js.Get("data")
		setSSDBCache(key, data)
	}
	reportSuccResp(r.RequestURI)
	return nil
}

func autoLogin(w http.ResponseWriter, r *http.Request) (apperr *util.AppError) {
	var req Request
	req.Init(r)
	uid := req.GetParamInt("uid")
	token := req.GetParamString("token")
	privdata := req.GetParamString("privdata")
	log.Printf("autoLogin uid:%d token:%s privdata:%s", uid, token, privdata)

	uuid := util.GenUUID()
	resp, rpcerr := callRPC(util.VerifyServerType, uid, "AutoLogin",
		&verify.AutoRequest{Head: &common.Head{Uid: uid, Sid: uuid},
			Token: token, Privdata: privdata})
	checkRPCErr(rpcerr, "AutoLogin")
	res := resp.Interface().(*verify.RegisterReply)
	checkRPCCode(res.Head.Retcode, "GetHots")

	body := genResponseBody(res, false)
	w.Write(body)
	reportSuccResp(r.RequestURI)
	return nil
}

func portalLogin(w http.ResponseWriter, r *http.Request) (apperr *util.AppError) {
	var req Request
	req.Init(r)
	phone := req.GetParamString("phone")
	code := req.GetParamString("code")
	acname := req.GetParamString("wlanacname")
	acip := req.GetParamString("wlanacip")
	userip := req.GetParamString("wlanuserip")
	usermac := req.GetParamString("wlanusermac")
	log.Printf("portalLogin phone:%s code:%s acname:%s acip:%s userip:%s usermac:%s",
		phone, code, acname, acip, userip, usermac)

	uuid := util.GenUUID()
	resp, rpcerr := callRPC(util.VerifyServerType, 0, "PortalLogin",
		&verify.PortalLoginRequest{Head: &common.Head{Sid: uuid},
			Info: &verify.PortalInfo{
				Acname: acname, Acip: acip, Usermac: usermac, Userip: userip,
				Phone: phone, Code: code}})
	checkRPCErr(rpcerr, "PortalLogin")
	res := resp.Interface().(*verify.PortalLoginReply)
	checkRPCCode(res.Head.Retcode, "PortalLogin")

	body := genResponseBody(res, true)
	req.WriteRsp(w, body)
	reportSuccResp(r.RequestURI)
	return nil
}

func oneClickLogin(w http.ResponseWriter, r *http.Request) (apperr *util.AppError) {
	var req Request
	req.Init(r)
	acname := req.GetParamString("wlanacname")
	acip := req.GetParamString("wlanacip")
	userip := req.GetParamString("wlanuserip")
	usermac := req.GetParamString("wlanusermac")
	log.Printf("oneClickLogin acname:%s acip:%s userip:%s usermac:%s",
		acname, acip, userip, usermac)

	uuid := util.GenUUID()
	resp, rpcerr := callRPC(util.VerifyServerType, 0, "OneClickLogin",
		&verify.AccessRequest{Head: &common.Head{Sid: uuid},
			Info: &verify.PortalInfo{
				Acname: acname, Acip: acip, Usermac: usermac, Userip: userip}})
	checkRPCErr(rpcerr, "OneClickLogin")
	res := resp.Interface().(*verify.PortalLoginReply)
	checkRPCCode(res.Head.Retcode, "OneClickLogin")

	body := genResponseBody(res, true)
	req.WriteRsp(w, body)
	reportSuccResp(r.RequestURI)
	return nil
}

func getService(w http.ResponseWriter, r *http.Request) (apperr *util.AppError) {
	var req Request
	req.InitCheckApp(r)
	uid := req.GetParamInt("uid")
	term := req.GetParamInt("term")
	if term != util.WxTerm {
		response, err := getRspFromSSDB(hotServiceKey)
		if err == nil {
			log.Printf("getRspFromSSDB succ key:%s\n", hotServiceKey)
			rspGzip(w, []byte(response))
			reportSuccResp(r.RequestURI)
			return nil
		}
	}

	uuid := util.GenUUID()
	resp, rpcerr := callRPC(util.HotServerType, uid, "GetServices",
		&common.CommRequest{Head: &common.Head{Uid: uid, Sid: uuid, Term: term}})
	checkRPCErr(rpcerr, "GetServices")
	res := resp.Interface().(*hot.ServiceReply)
	checkRPCCode(res.Head.Retcode, "GetServices")

	js, err := simplejson.NewJson([]byte(`{"errno":0}`))
	if err != nil {
		return &util.AppError{errInner, "init json failed"}
	}
	js.SetPath([]string{"data", "services"}, res.Services)
	body, err := js.MarshalJSON()
	if err != nil {
		return &util.AppError{errInner, "marshal json failed"}
	}
	rspGzip(w, body)
	if term != util.WxTerm {
		data := js.Get("data")
		setSSDBCache(hotServiceKey, data)
	}
	reportSuccResp(r.RequestURI)
	return nil
}

func getDiscovery(w http.ResponseWriter, r *http.Request) (apperr *util.AppError) {
	var req Request
	req.InitCheckApp(r)
	uid := req.GetParamInt("uid")
	term := req.GetParamIntDef("term", 0)
	response, err := getRspFromSSDB(configDiscoveryKey)
	if err == nil {
		log.Printf("getRspFromSSDB succ key:%s\n", configDiscoveryKey)
		rspGzip(w, []byte(response))
		reportSuccResp(r.RequestURI)
		return nil
	}

	uuid := util.GenUUID()
	resp, rpcerr := callRPC(util.ConfigServerType, uid, "GetDiscovery",
		&common.CommRequest{Head: &common.Head{Uid: uid, Sid: uuid, Term: term}})
	checkRPCErr(rpcerr, "GetDiscovery")
	res := resp.Interface().(*config.DiscoveryReply)
	checkRPCCode(res.Head.Retcode, "GetDiscovery")

	js, err := simplejson.NewJson([]byte(`{"errno":0}`))
	if err != nil {
		return &util.AppError{errInner, "init json failed"}
	}
	js.SetPath([]string{"data", "services"}, res.Services)
	js.SetPath([]string{"data", "banners"}, res.Banners)
	js.SetPath([]string{"data", "recommends"}, res.Recommends)
	js.SetPath([]string{"data", "urbanservices"}, res.Urbanservices)
	body, err := js.MarshalJSON()
	if err != nil {
		return &util.AppError{errInner, "marshal json failed"}
	}
	rspGzip(w, body)
	data := js.Get("data")
	setSSDBCache(configDiscoveryKey, data)
	reportSuccResp(r.RequestURI)
	return nil
}

func punchAp(w http.ResponseWriter, r *http.Request) (apperr *util.AppError) {
	var req Request
	req.InitCheckApp(r)
	uid := req.GetParamInt("uid")
	apmac := req.GetParamString("apmac")
	uuid := util.GenUUID()
	resp, rpcerr := callRPC(util.PunchServerType, uid, "Punch",
		&punch.PunchRequest{Head: &common.Head{Uid: uid, Sid: uuid}, Apmac: apmac})
	checkRPCErr(rpcerr, "Punch")
	res := resp.Interface().(*common.CommReply)
	checkRPCCode(res.Head.Retcode, "Punch")

	w.Write([]byte(`{"errno":0}`))
	reportSuccResp(r.RequestURI)
	return nil
}

func correctAp(w http.ResponseWriter, r *http.Request) (apperr *util.AppError) {
	var req Request
	req.InitCheckApp(r)
	uid := req.GetParamInt("uid")
	aid := req.GetParamInt("aid")
	etype := req.GetParamInt("type")
	uuid := util.GenUUID()
	resp, rpcerr := callRPC(util.PunchServerType, uid, "Correct",
		&punch.ApRequest{Head: &common.Head{Uid: uid, Sid: uuid}, Aid: aid,
			Etype: etype})
	checkRPCErr(rpcerr, "Correct")
	res := resp.Interface().(*common.CommReply)
	checkRPCCode(res.Head.Retcode, "Correct")

	w.Write([]byte(`{"errno":0}`))
	reportSuccResp(r.RequestURI)
	return nil
}

func getMyPunch(w http.ResponseWriter, r *http.Request) (apperr *util.AppError) {
	var req Request
	req.InitCheckApp(r)
	uid := req.GetParamInt("uid")

	uuid := util.GenUUID()
	resp, rpcerr := callRPC(util.PunchServerType, uid, "GetPunch",
		&common.CommRequest{
			Head: &common.Head{Sid: uuid, Uid: uid}})
	checkRPCErr(rpcerr, "GetPunch")
	res := resp.Interface().(*punch.PunchReply)
	checkRPCCode(res.Head.Retcode, "GetPunch")

	body := genResponseBody(res, false)
	w.Write(body)
	reportSuccResp(r.RequestURI)
	return nil
}

func getUserinfo(w http.ResponseWriter, r *http.Request) (apperr *util.AppError) {
	var req Request
	req.InitCheckApp(r)
	uid := req.GetParamInt("uid")

	uuid := util.GenUUID()
	resp, rpcerr := callRPC(util.FetchServerType, uid, "FetchUserInfo",
		&common.CommRequest{Head: &common.Head{Sid: uuid, Uid: uid}})
	checkRPCErr(rpcerr, "FetchUserInfo")
	res := resp.Interface().(*fetch.UserInfoReply)
	checkRPCCode(res.Head.Retcode, "FetchUserInfo")

	body := genResponseBody(res, false)
	w.Write(body)
	reportSuccResp(r.RequestURI)
	return nil
}

func getUserInfo(w http.ResponseWriter, r *http.Request) (apperr *util.AppError) {
	var req Request
	req.InitCheckApp(r)
	uid := req.GetParamInt("uid")
	tuid := req.GetParamInt("tuid")

	uuid := util.GenUUID()
	resp, rpcerr := callRPC(util.UserinfoServerType, uid, "GetInfo",
		&common.CommRequest{
			Head: &common.Head{Sid: uuid, Uid: tuid}})
	checkRPCErr(rpcerr, "GetInfo")
	res := resp.Interface().(*userinfo.InfoReply)
	checkRPCCode(res.Head.Retcode, "GetInfo")

	body := genResponseBody(res, false)
	w.Write(body)
	reportSuccResp(r.RequestURI)
	return nil
}

func getRandNick(w http.ResponseWriter, r *http.Request) (apperr *util.AppError) {
	var req Request
	req.InitCheckApp(r)
	uid := req.GetParamInt("uid")

	uuid := util.GenUUID()
	resp, rpcerr := callRPC(util.UserinfoServerType, uid, "GenRandNick",
		&common.CommRequest{
			Head: &common.Head{Sid: uuid, Uid: uid}})
	checkRPCErr(rpcerr, "GenRandNick")
	res := resp.Interface().(*userinfo.NickReply)
	checkRPCCode(res.Head.Retcode, "GenRandNick")

	body := genResponseBody(res, false)
	w.Write(body)
	reportSuccResp(r.RequestURI)
	return nil
}

func getDefHead(w http.ResponseWriter, r *http.Request) (apperr *util.AppError) {
	var req Request
	req.InitCheckApp(r)
	uid := req.GetParamInt("uid")

	uuid := util.GenUUID()
	resp, rpcerr := callRPC(util.UserinfoServerType, uid, "GetDefHead",
		&common.CommRequest{
			Head: &common.Head{Sid: uuid, Uid: uid}})
	checkRPCErr(rpcerr, "GetDefHead")
	res := resp.Interface().(*userinfo.HeadReply)
	checkRPCCode(res.Head.Retcode, "GetDefHead")

	body := genResponseBody(res, false)
	w.Write(body)
	reportSuccResp(r.RequestURI)
	return nil
}

func getPortalMenu(w http.ResponseWriter, r *http.Request) (apperr *util.AppError) {
	var req Request
	req.InitCheckApp(r)
	uid := req.GetParamInt("uid")

	uuid := util.GenUUID()
	resp, rpcerr := callRPC(util.ConfigServerType, uid, "GetPortalMenu",
		&common.CommRequest{
			Head: &common.Head{Sid: uuid, Uid: uid}})
	checkRPCErr(rpcerr, "GetPortalMenu")
	res := resp.Interface().(*config.PortalMenuReply)
	checkRPCCode(res.Head.Retcode, "GetPortalMenu")

	body := genResponseBody(res, false)
	w.Write(body)
	reportSuccResp(r.RequestURI)
	return nil
}

func modUserInfo(w http.ResponseWriter, r *http.Request) (apperr *util.AppError) {
	var req Request
	req.InitCheckApp(r)
	uid := req.GetParamInt("uid")
	nickname := req.GetParamStringDef("nickname", "")
	headurl := req.GetParamStringDef("headurl", "")

	if headurl == "" && nickname == "" {
		w.Write([]byte(`{"errno":0}`))
		return nil
	}

	uuid := util.GenUUID()
	resp, rpcerr := callRPC(util.UserinfoServerType, uid, "ModInfo",
		&userinfo.InfoRequest{
			Head:    &common.Head{Sid: uuid, Uid: uid},
			Headurl: headurl, Nickname: nickname})
	checkRPCErr(rpcerr, "ModInfo")
	res := resp.Interface().(*common.CommReply)
	checkRPCCode(res.Head.Retcode, "ModInfo")

	w.Write([]byte(`{"errno":0}`))
	reportSuccResp(r.RequestURI)
	return nil
}

func getPunchStat(w http.ResponseWriter, r *http.Request) (apperr *util.AppError) {
	var req Request
	req.InitCheckApp(r)
	uid := req.GetParamInt("uid")
	apmac := req.GetParamString("apmac")

	uuid := util.GenUUID()
	resp, rpcerr := callRPC(util.PunchServerType, uid, "GetStat",
		&punch.PunchRequest{
			Head: &common.Head{Sid: uuid, Uid: uid}, Apmac: apmac})
	checkRPCErr(rpcerr, "GetStat")
	res := resp.Interface().(*punch.PunchStatReply)
	checkRPCCode(res.Head.Retcode, "GetStat")

	body := genResponseBody(res, false)
	w.Write(body)
	reportSuccResp(r.RequestURI)
	return nil
}

func submitXcxCode(w http.ResponseWriter, r *http.Request) (apperr *util.AppError) {
	var req Request
	req.Init(r)
	code := req.GetParamString("code")

	uuid := util.GenUUID()
	resp, rpcerr := callRPC(util.PunchServerType, 0, "SubmitCode",
		&punch.CodeRequest{
			Head: &common.Head{Sid: uuid}, Code: code})
	checkRPCErr(rpcerr, "SubmitCode")
	res := resp.Interface().(*punch.LoginReply)
	checkRPCCode(res.Head.Retcode, "SubmitCode")

	body := genResponseBody(res, false)
	w.Write(body)
	reportSuccResp(r.RequestURI)
	return nil
}

func xcxLogin(w http.ResponseWriter, r *http.Request) (apperr *util.AppError) {
	var req Request
	req.Init(r)
	sid := req.GetParamString("sid")
	rawData := req.GetParamString("rawData")
	signature := req.GetParamString("signature")
	encryptedData := req.GetParamString("encryptedData")
	iv := req.GetParamString("iv")

	uuid := util.GenUUID()
	resp, rpcerr := callRPC(util.PunchServerType, 0, "Login",
		&punch.LoginRequest{
			Head: &common.Head{Sid: uuid}, Sid: sid,
			Rawdata: rawData, Signature: signature,
			Encrypteddata: encryptedData, Iv: iv})
	checkRPCErr(rpcerr, "Login")
	res := resp.Interface().(*punch.LoginReply)
	checkRPCCode(res.Head.Retcode, "Login")

	body := genResponseBody(res, false)
	w.Write(body)
	reportSuccResp(r.RequestURI)
	return nil
}

func getAllAps(w http.ResponseWriter, r *http.Request) (apperr *util.AppError) {
	var req Request
	req.Init(r)
	uid := req.GetParamInt("uid")
	response, err := getRspFromSSDB(hotAllApsKey)
	if err == nil {
		log.Printf("getRspFromSSDB succ key:%s\n", hotAllApsKey)
		rspGzip(w, []byte(response))
		reportSuccResp(r.RequestURI)
		return nil
	}

	uuid := util.GenUUID()
	resp, rpcerr := callRPC(util.FetchServerType, uid, "FetchAllAps",
		&common.CommRequest{
			Head: &common.Head{Sid: uuid, Uid: uid}})
	checkRPCErr(rpcerr, "FetchAllAps")
	res := resp.Interface().(*fetch.ApReply)
	checkRPCCode(res.Head.Retcode, "FetchAllAps")

	js, err := simplejson.NewJson([]byte(`{"errno":0}`))
	if err != nil {
		return &util.AppError{errInner, "invalid param"}
	}
	js.SetPath([]string{"data", "infos"}, res.Infos)

	body, err := js.MarshalJSON()
	if err != nil {
		return &util.AppError{errInner, "marshal json failed"}
	}
	rspGzip(w, body)
	data := js.Get("data")
	setSSDBCache(hotAllApsKey, data)
	reportSuccResp(r.RequestURI)
	return nil
}

func getAppAps(w http.ResponseWriter, r *http.Request) (apperr *util.AppError) {
	return getAps(w, r, false)
}

func extractIP(addr string) string {
	arr := strings.Split(addr, ":")
	return arr[0]
}

func register(w http.ResponseWriter, r *http.Request) (apperr *util.AppError) {
	var req Request
	req.Init(r)
	username := req.GetParamString("username")
	password := req.GetParamString("password")
	udid := req.GetParamString("udid")
	model := req.GetParamString("model")
	channel := req.GetParamString("channel")
	version := req.GetParamInt("version")
	term := req.GetParamInt("term")
	regip := extractIP(r.RemoteAddr)
	code := req.GetParamStringDef("code", "")
	log.Printf("register request username:%s password:%s udid:%s model:%s channel:%s version:%d term:%d",
		username, password, udid, model, channel, version, term)

	uuid := util.GenUUID()
	resp, rpcerr := callRPC(util.VerifyServerType, 0, "Register",
		&verify.RegisterRequest{Head: &common.Head{Sid: uuid},
			Username: username, Password: password, Code: code,
			Client: &verify.ClientInfo{Udid: udid, Model: model,
				Channel: channel, Regip: regip,
				Version: version, Term: term}})
	checkRPCErr(rpcerr, "Register")
	res := resp.Interface().(*verify.RegisterReply)
	checkRPCCode(res.Head.Retcode, "Register")

	body := genResponseBody(res, true)
	w.Write(body)
	reportSuccResp(r.RequestURI)
	return nil
}

func wxMpLogin(w http.ResponseWriter, r *http.Request) {
	reportRequest(r.RequestURI)
	r.ParseForm()
	code := r.Form["code"]
	if len(code) == 0 {
		log.Printf("get code failed\n")
		w.Write([]byte(`{"errno":2,"desc":"invalid param"}`))
		return
	}
	echostr := r.Form["echostr"]

	address := getNameServer(0, util.VerifyServerName)
	conn, err := grpc.Dial(address, grpc.WithInsecure())
	if err != nil {
		log.Printf("did not connect: %v", err)
		w.Write([]byte(`{"errno":2,"desc":"invalid param"}`))
		return
	}
	defer conn.Close()
	c := verify.NewVerifyClient(conn)

	log.Printf("code:%s\n", code[0])
	uuid := util.GenUUID()
	res, err := c.WxMpLogin(context.Background(),
		&verify.LoginRequest{Head: &common.Head{Sid: uuid}, Code: code[0]})
	if err != nil {
		log.Printf("Login failed: %v", err)
		w.Write([]byte(`{"errno":2,"desc":"invalid param"}`))
		return
	}

	if res.Head.Retcode != 0 {
		w.Write([]byte(`{"errno":105,"desc":"微信公众号登录失败"}`))
		return
	}

	if len(echostr) == 0 {
		rs := fmt.Sprintf(`{"errno":0, "uid":%d, "token":%s"}`, res.Head.Uid, res.Token)
		w.Write([]byte(rs))
		return
	}

	dst := fmt.Sprintf("%s?uid=%d&token=%s&union=%s", echostr[0], res.Head.Uid, res.Token, res.Privdata)
	http.Redirect(w, r, dst, http.StatusMovedPermanently)
}

func jump(w http.ResponseWriter, r *http.Request) {
	reportRequest(r.RequestURI)
	r.ParseForm()
	file := r.Form["echofile"]
	var echostr string
	if len(file) > 0 {
		echostr = file[0]
		echostr = wxHost + echostr
	}
	ck, err := r.Cookie("UNION")
	if err == nil {
		log.Printf("get cookie UNION succ:%s", ck.Value)
		address := getNameServer(0, util.VerifyServerName)
		conn, err := grpc.Dial(address, grpc.WithInsecure())
		if err != nil {
			log.Printf("did not connect: %v", err)
			w.Write([]byte(`{"errno":2,"desc":"invalid param"}`))
			return
		}
		defer conn.Close()
		c := verify.NewVerifyClient(conn)

		uuid := util.GenUUID()
		res, err := c.UnionLogin(context.Background(),
			&verify.LoginRequest{Head: &common.Head{Sid: uuid}, Unionid: ck.Value})
		if err != nil {
			log.Printf("UnionLogin failed: %v", err)
			w.Write([]byte(`{"errno":2,"desc":"invalid param"}`))
			return
		}

		if res.Head.Retcode != 0 {
			w.Write([]byte(`{"errno":106,"desc":"微信公众号登录失败"}`))
			return
		}
		dst := fmt.Sprintf("%s?uid=%d&token=%s", echostr, res.Head.Uid, res.Token)
		http.Redirect(w, r, dst, http.StatusMovedPermanently)
		return
	}
	redirect := wxHost + "wx_mp_login"
	redirect += "?echostr=" + echostr
	dst := util.GenRedirectURL(redirect)
	http.Redirect(w, r, dst, http.StatusMovedPermanently)
}

func getPortalDir() string {
	if pdir.Expire > time.Now().Unix() {
		return pdir.Dir
	}
	uuid := util.GenUUID()
	resp, rpcerr := callRPC(util.FetchServerType, 0, "FetchPortal",
		&common.CommRequest{Head: &common.Head{Sid: uuid}})
	if rpcerr.Interface() != nil {
		return pdir.Dir
	}
	res := resp.Interface().(*fetch.PortalReply)
	if res.Head.Retcode != 0 {
		return pdir.Dir
	}
	pdir.Expire = time.Now().Unix() + 60
	pdir.Dir = res.Dir
	log.Printf("update pdir dir:%s expire:%d", pdir.Dir, pdir.Expire)
	return res.Dir
}

func portal(w http.ResponseWriter, r *http.Request) {
	reportRequest(r.RequestURI)
	pos := strings.Index(r.RequestURI, "?")
	var postfix string
	var path string
	if pos != -1 {
		postfix = r.RequestURI[pos:]
		path = r.RequestURI[0:pos]
	} else {
		path = r.RequestURI
	}
	prefix := portalDst
	dir := getPortalDir()
	dst := prefix + dir + postfix
	log.Printf("path:%s prefix:%s dir:%s", path, prefix, dir)
	dst += fmt.Sprintf("&ts=%d", time.Now().Unix())
	log.Printf("portal dst:%s", dst)
	http.Redirect(w, r, dst, http.StatusMovedPermanently)
}

func genNonce() string {
	nonce := "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ"
	var res []byte
	r := rand.New(rand.NewSource(time.Now().Unix()))
	for i := 0; i < 12; i++ {
		ch := nonce[r.Int31n(int32(len(nonce)))]
		res = append(res, ch)
	}
	return string(res)
}

func getJsapiSign(w http.ResponseWriter, r *http.Request) {
	reportRequest(r.RequestURI)
	address := getNameServer(0, util.VerifyServerName)
	conn, err := grpc.Dial(address, grpc.WithInsecure())
	if err != nil {
		log.Printf("did not connect: %v", err)
		w.Write([]byte(`{"errno":2,"desc":"invalid param"}`))
		return
	}
	defer conn.Close()
	c := verify.NewVerifyClient(conn)

	uuid := util.GenUUID()
	res, err := c.GetWxTicket(context.Background(),
		&verify.TicketRequest{Head: &common.Head{Sid: uuid}})
	if err != nil {
		log.Printf("GetWxTicket failed: %v", err)
		w.Write([]byte(`{"errno":2,"desc":"invalid param"}`))
		return
	}

	if res.Head.Retcode != 0 {
		w.Write([]byte(`{"errno":107,"desc":"获取微信ticket失败"}`))
		return
	}

	noncestr := genNonce()
	ts := time.Now().Unix()
	url := r.Referer()
	pos := strings.Index(url, "#")
	if pos != -1 {
		url = url[:pos]
	}

	ori := fmt.Sprintf("jsapi_ticket=%s&noncestr=%s&timestamp=%d&url=%s", res.Ticket, noncestr, ts, url)
	sign := util.Sha1(ori)
	log.Printf("origin:%s sign:%s\n", ori, sign)
	out := fmt.Sprintf("var wx_cfg={\"debug\":false, \"appId\":\"%s\",\"timestamp\":%d,\"nonceStr\":\"%s\",\"signature\":\"%s\",\"jsApiList\":[],\"jsapi_ticket\":\"%s\"};", util.WxDgAppid, ts, noncestr, sign, res.Ticket)
	w.Write([]byte(out))
	return
}

func pingppWebhook(w http.ResponseWriter, r *http.Request) {
	if strings.ToUpper(r.Method) == "POST" {
		buf := new(bytes.Buffer)
		buf.ReadFrom(r.Body)
		webhook, err := pingpp.ParseWebhooks(buf.Bytes())
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprintf(w, "fail")
			return
		}
		fmt.Println(webhook.Type)
		if webhook.Type == "charge.succeeded" {
			//TODO for charge success
			w.WriteHeader(http.StatusOK)
		} else if webhook.Type == "refund.succeeded" {
			//TODO for refund success
			w.WriteHeader(http.StatusOK)
		} else {
			w.WriteHeader(http.StatusInternalServerError)
		}
	}
	return
}

func getAppConf(w http.ResponseWriter, r *http.Request) (apperr *util.AppError) {
	return getConf(w, r, false)
}

//NewAppServer return app http handler
func NewAppServer() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/login", appHandler(login))
	mux.Handle("/get_phone_code", appHandler(getPhoneCode))
	mux.Handle("/get_check_code", appHandler(getCheckCode))
	mux.Handle("/register", appHandler(register))
	mux.Handle("/logout", appHandler(logout))
	mux.Handle("/hot", appHandler(getHot))
	mux.Handle("/get_weather_news", appHandler(getWeatherNews))
	mux.Handle("/get_live_info", appHandler(getLiveInfo))
	mux.Handle("/get_jokes", appHandler(getJokes))
	mux.Handle("/get_conf", appHandler(getKvConf))
	mux.Handle("/get_menu", appHandler(getMenu))
	mux.Handle("/get_front_info", appHandler(getFrontInfo))
	mux.Handle("/get_flash_ad", appHandler(getFlashAd))
	mux.Handle("/get_opening", appHandler(getOpening))
	mux.Handle("/get_opened", appHandler(getOpened))
	mux.Handle("/get_hotlist", appHandler(getHotList))
	mux.Handle("/get_wifi_pass", appHandler(getWifiPass))
	mux.Handle("/get_zipcode", appHandler(getZipcode))
	mux.Handle("/get_activity", appHandler(getActivity))
	mux.Handle("/get_intro", appHandler(getGoodsIntro))
	mux.Handle("/get_bet_history", appHandler(getBetHistory))
	mux.Handle("/get_record", appHandler(getPurchaseRecord))
	mux.Handle("/get_user_bet", appHandler(getUserBet))
	mux.Handle("/get_user_award", appHandler(getUserBet))
	mux.Handle("/get_address", appHandler(getAddress))
	mux.Handle("/get_win_status", appHandler(getWinStatus))
	mux.Handle("/post_share", appHandler(addShare))
	mux.Handle("/set_win_status", appHandler(setWinStatus))
	mux.Handle("/get_share_gid", appHandler(getShare))
	mux.Handle("/get_share_list", appHandler(getShare))
	mux.Handle("/get_share_uid", appHandler(getShare))
	mux.Handle("/get_share_detail", appHandler(getShareDetail))
	mux.Handle("/get_detail", appHandler(getDetail))
	mux.Handle("/get_detail_gid", appHandler(getDetail))
	mux.Handle("/add_address", appHandler(addAddress))
	mux.Handle("/feedback", appHandler(addFeedback))
	mux.Handle("/delete_address", appHandler(delAddress))
	mux.Handle("/update_address", appHandler(modAddress))
	mux.Handle("/get_image_token", appHandler(getImageToken))
	mux.Handle("/fetch_wifi", appHandler(fetchWifi))
	mux.Handle("/check_update", appHandler(checkUpdate))
	mux.Handle("/check_login", appHandler(checkLogin))
	mux.Handle("/one_click_login", appHandler(oneClickLogin))
	mux.Handle("/auto_login", appHandler(autoLogin))
	mux.Handle("/portal_login", appHandler(portalLogin))
	mux.Handle("/get_nearby_aps", appHandler(getAppAps))
	mux.Handle("/get_all_aps", appHandler(getAllAps))
	mux.Handle("/report_wifi", appHandler(reportWifi))
	mux.Handle("/report_click", appHandler(reportClick))
	mux.Handle("/report_apmac", appHandler(reportApmac))
	mux.Handle("/connect_wifi", appHandler(connectWifi))
	mux.Handle("/upload_callback", appHandler(uploadCallback))
	mux.Handle("/purchase_sales", appHandler(purchaseSales))
	mux.Handle("/apply_image_upload", appHandler(applyImageUpload))
	mux.Handle("/pingpp_pay", appHandler(pingppPay))
	mux.Handle("/services", appHandler(getService))
	mux.Handle("/get_discovery", appHandler(getDiscovery))
	mux.Handle("/punch", appHandler(punchAp))
	mux.Handle("/get_my_punch", appHandler(getMyPunch))
	mux.Handle("/get_user_info", appHandler(getUserInfo))
	mux.Handle("/get_rand_nick", appHandler(getRandNick))
	mux.Handle("/mod_user_info", appHandler(modUserInfo))
	mux.Handle("/get_def_head", appHandler(getDefHead))
	mux.Handle("/get_portal_menu", appHandler(getPortalMenu))
	mux.Handle("/get_userinfo", appHandler(getUserinfo))
	mux.Handle("/get_punch_stat", appHandler(getPunchStat))
	mux.Handle("/submit_xcx_code", appHandler(submitXcxCode))
	mux.Handle("/xcx_login", appHandler(xcxLogin))
	mux.Handle("/correct_ap", appHandler(correctAp))
	mux.HandleFunc("/jump", jump)
	mux.HandleFunc("/portal", portal)
	mux.HandleFunc("/wx_mp_login", wxMpLogin)
	mux.HandleFunc("/get_jsapi_sign", getJsapiSign)
	mux.HandleFunc("/pingpp_webhook", pingppWebhook)
	mux.Handle("/", http.FileServer(http.Dir("/data/server/html")))
	return mux
}
