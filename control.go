package main

import (
	"errors"
	"fmt"
	"github.com/spf13/viper"
	"github.com/tidwall/gjson"
	tb "gopkg.in/tucnak/telebot.v2"
	"io"
	"log"
	"os"
	"strconv"
	"strings"
	"time"
)

var SignOk map[int64]int

//If Successfully return "",else return error information
func BindUser(m *tb.Message, cid, cse string) error {
	logger.Printf("%d Begin Bind\n", m.Chat.ID)
	tmp := strings.Split(m.Text, " ")
	if len(tmp) != 2 {
		logger.Printf("%d Bind error:Wrong Bind Format\n", m.Chat.ID)
		return errors.New("绑定格式错误")
	}
	logger.Println("alias: " + tmp[1])
	alias := tmp[1]
	code := GetURLValue(tmp[0], "code")
	//fmt.Println(code)
	access, refresh, err := MSFirGetToken(code, cid, cse)
	if err != nil {
		logger.Printf("%d Bind error:GetRefreshToken %s \n", m.Chat.ID, err.Error())
		return err
	}

	//token has gotten
	bot.Send(m.Chat, "Token获取成功!")
	info, err := MSGetUserInfo(access)
	//fmt.Printf("TGID:%d Refresh Token: %s\n", m.Chat.ID, refresh)
	if err != nil {
		logger.Printf("%d Bind error:Getinfo %s \n", m.Chat.ID, err.Error())
		return err
	}

	var u MSData
	u.tgId = m.Chat.ID
	u.refreshToken = refresh
	//TG的Data传递最高64bytes,一些msid超过了报错BUTTON_DATA_INVALID (0)，采取md5
	u.msId = Get16MD5Encode(gjson.Get(info, "id").String())
	u.uptime = time.Now().Unix()
	logger.Println(u.uptime)
	u.alias = alias
	u.clientId = cid
	u.clientSecret = cse
	u.other = ""
	//MS User Is Exist
	if MSAppIsExist(u.tgId, u.clientId) {
		logger.Printf("%d Bind error:MSUserHasExisted\n", m.Chat.ID)
		return errors.New("该应用已经绑定过了，无需重复绑定")
	}
	//MS information has gotten
	bot.Send(m.Chat, "MS_ID(MD5)： "+u.msId+"\nuserPrincipalName： "+gjson.Get(info, "userPrincipalName").String()+"\ndisplayName： "+gjson.Get(info, "displayName").String()+"\n")
	if ok, err := AddData(db, u); !ok {
		logger.Printf("%d Bind error: %s\n", m.Chat.ID, err)
		return err
	}
	logger.Printf("%d Bind Successfully!\n", m.Chat.ID)
	return nil
}

//get bind num
func GetBindNum(tgId int64) int {
	data := QueryDataByTG(db, tgId)
	return len(data)
}

//return true => exist
func MSAppIsExist(tgId int64, clientId string) bool {
	data := QueryDataByTG(db, tgId)
	var res MSData
	for _, res = range data {
		if res.clientId == clientId {
			return true
		}
	}
	return false
}

//SignTask
func SignTask() {
	var SignOk map[int64]int
	var SignErr []string
	var num, signOk int
	SignOk = make(map[int64]int)
	fmt.Println("----Task Begin----")
	fmt.Println("Time:" + time.Now().Format("2006-01-02 15:04:05"))
	data := QueryDataAll(db)
	num = len(data)
	fmt.Println("Start Sign")
	for _, u := range data {
		pre := "您的账户: " + u.alias + "\n在任务执行时出现了错误!\n错误:"
		access, err := MSGetToken(u.refreshToken, u.clientId, u.clientSecret)
		chat, _ := bot.ChatByID(strconv.FormatInt(u.tgId, 10))
		//生成解绑按钮
		var inlineKeys [][]tb.InlineButton
		UnBindBtn := tb.InlineButton{Unique: "un" + u.msId, Text: "点击解绑该账户", Data: u.msId}
		bot.Handle(&UnBindBtn, bUnBindInlineBtn)
		inlineKeys = append(inlineKeys, []tb.InlineButton{UnBindBtn})
		tmpBtn := &tb.ReplyMarkup{InlineKeyboard: inlineKeys}

		se := u.msId + " ( @" + chat.Username + " )"
		if err != nil {
			logger.Println(u.msId+" ", err)
			bot.Send(chat, pre+gjson.Get(err.Error(), "error").String(), tmpBtn)
			SignErr = append(SignErr, se)
			continue
		}
		if ok, err := OutLookGetMails(access); !ok {
			logger.Println(u.msId+" ", err)
			bot.Send(chat, pre+gjson.Get(err.Error(), "error").String(), tmpBtn)
			SignErr = append(SignErr, se)
			continue
		}
		u.uptime = time.Now().Unix()
		if ok, err := UpdateData(db, u); !ok {
			logger.Println(u.msId+" ", err)
			bot.Send(chat, pre+err.Error(), tmpBtn)
			SignErr = append(SignErr, se)
			continue
		}
		fmt.Println(u.msId + " Sign OK!")
		SignOk[u.tgId]++
		signOk++
	}
	fmt.Println("Sign End,Start Send")
	var isSend map[int64]bool
	isSend = make(map[int64]bool)
	//用户任务反馈
	for _, u := range data {
		if !isSend[u.tgId] {
			chat, err := bot.ChatByID(strconv.FormatInt(u.tgId, 10))
			if err != nil {
				logger.Println("Send Result ERROR", err)
				continue
			}
			//静默发送，过多消息很烦
			_, err = bot.Send(chat, "任务反馈\n时间: "+time.Now().Format("2006-01-02 15:04:05")+"\n结果: "+strconv.Itoa(SignOk[u.tgId])+"/"+strconv.Itoa(GetBindNum(u.tgId)), &tb.SendOptions{DisableNotification: true})
			if err != nil {
				logger.Println(err)
			}
			isSend[u.tgId] = true
		}
	}
	//管理员任务反馈
	var ErrUser string
	for _, eu := range SignErr {
		ErrUser = ErrUser + eu + "\n"
	}
	for _, a := range admin {
		chat, _ := bot.ChatByID(strconv.FormatInt(a, 10))
		bot.Send(chat, "任务反馈(管理员)\n完成时间: "+time.Now().Format("2006-01-02 15:04:05")+"\n结果: "+strconv.Itoa(signOk)+"/"+strconv.Itoa(num)+"\n错误账户msid:\n"+ErrUser)
	}
	fmt.Println("----Task End----")
}
func GetAdmin() []int64 {
	var result []int64
	admins := strings.Split(viper.GetString("admin"), ",")
	for _, v := range admins {
		id, _ := strconv.ParseInt(v, 10, 64)
		result = append(result, id)
	}
	return result
}
func InitLogger() {
	if !PathExists("./log/") {
		os.Mkdir("./log/", 0773)
	}

	path := "./log/" + time.Now().Format("2006-01-02") + ".log"
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0773)
	if err != nil {
		logger.Println(err)
	}
	writers := []io.Writer{
		f,
		os.Stdout}
	faoWriter := io.MultiWriter(writers...)
	logger = log.New(faoWriter, "【E5Sub】", log.Ldate|log.Ltime|log.Lshortfile)
}
