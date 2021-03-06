package main

import (
	"fmt"
	"github.com/fsnotify/fsnotify"
	"github.com/spf13/viper"
	tb "gopkg.in/tucnak/telebot.v2"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	bLogBasePath  string = "./log/"
	bStartContent string = "欢迎使用E5SubBot!"
	bHelpContent  string = `
	命令：
	/my 查看已绑定账户信息
	/bind  绑定新账户
	/unbind 解绑账户
	/help 帮助
	/task 手动执行一次任务(管理员)
	/log 获取最近日志文件(管理员)
	源码及使用方法：https://github.com/iyear/E5SubBot
`
)

var (
	UserStatus  map[int64]int
	UserCid     map[int64]string
	UserCSecret map[int64]string
	BindMaxNum  int
	notice      string
	admin       []int64
)

const (
	USNone = iota
	USBind1
	USBind2
)

func init() {
	//read config
	viper.SetConfigName("config")
	viper.AddConfigPath(".")
	err := viper.ReadInConfig()
	CheckErr(err)

	BindMaxNum = viper.GetInt("bindmax")
	notice = viper.GetString("notice")
	admin = GetAdmin()

	viper.WatchConfig()
	viper.OnConfigChange(func(e fsnotify.Event) {
		BindMaxNum = viper.GetInt("bindmax")
		notice = viper.GetString("notice")
		admin = GetAdmin()
	})

	UserStatus = make(map[int64]int)
	UserCid = make(map[int64]string)
	UserCSecret = make(map[int64]string)
}

func bStart(m *tb.Message) {
	bot.Send(m.Sender, bStartContent)
	bHelp(m)
}

func bMy(m *tb.Message) {
	data := QueryDataByTG(db, m.Chat.ID)
	var inlineKeys [][]tb.InlineButton
	for _, u := range data {
		inlineBtn := tb.InlineButton{
			Unique: "my" + u.msId,
			Text:   u.alias,
			Data:   u.msId,
		}
		bot.Handle(&inlineBtn, bMyInlineBtn)
		inlineKeys = append(inlineKeys, []tb.InlineButton{inlineBtn})
	}
	bot.Send(m.Chat, "选择一个账户查看具体信息\n\n绑定数: "+strconv.Itoa(GetBindNum(m.Chat.ID))+"/"+strconv.Itoa(BindMaxNum), &tb.ReplyMarkup{InlineKeyboard: inlineKeys})
}
func bMyInlineBtn(c *tb.Callback) {
	r := QueryDataByMS(db, c.Data)
	u := r[0]
	bot.Send(c.Message.Chat, "信息\n别名："+u.alias+"\nMS_ID(MD5): "+u.msId+"\nclient_id: "+u.clientId+"\nclient_secret: "+u.clientSecret+"\n最近更新时间: "+time.Unix(u.uptime, 0).Format("2006-01-02 15:04:05")+"\n\nrefresh_token: "+u.refreshToken)
	bot.Respond(c)
}

func bLog(m *tb.Message) {
	flag := 0
	for _, a := range admin {
		if a == m.Chat.ID {
			flag = 1
		}
	}
	if flag == 0 {
		bot.Send(m.Chat, "您没有权限执行此操作~")
		return
	}
	logs := GetRecentLogs(bLogBasePath, 5)
	var inlineKeys [][]tb.InlineButton
	for _, log := range logs {
		inlineBtn := tb.InlineButton{
			Unique: "log" + strings.Replace(strings.TrimSuffix(filepath.Base(log), ".log"), "-", "", -1),
			Text:   filepath.Base(log),
			Data:   filepath.Base(log),
		}
		bot.Handle(&inlineBtn, bLogsInlineBtn)
		inlineKeys = append(inlineKeys, []tb.InlineButton{inlineBtn})
	}
	_, err := bot.Send(m.Chat, "选择一个日志", &tb.ReplyMarkup{InlineKeyboard: inlineKeys})
	fmt.Println(err)
}
func bLogsInlineBtn(c *tb.Callback) {
	//fmt.Println(c.Data)
	//logger.Println(bLogBasePath + c.Data + ".log")
	logfile := &tb.Document{File: tb.FromDisk(bLogBasePath + c.Data), FileName: c.Data, MIME: "text/plain"}
	_, err := bot.Send(c.Message.Chat, logfile)
	if err != nil {
		logger.Println(err)
		return
	}
	bot.Respond(c)
}
func bBind1(m *tb.Message) {
	logger.Println("ReApp: " + strconv.FormatInt(m.Chat.ID, 10))
	bot.Send(m.Chat, "应用注册： [点击直达]("+MSGetReAppUrl()+")", tb.ModeMarkdown)
	_, err := bot.Send(m.Chat, "请回复client_id+空格+client_secret", &tb.ReplyMarkup{ForceReply: true})
	if err == nil {
		UserStatus[m.Chat.ID] = USBind1
		UserCid[m.Chat.ID] = m.Text
	}

}
func bBind2(m *tb.Message) {
	logger.Println("Auth: " + strconv.FormatInt(m.Chat.ID, 10))
	tmp := strings.Split(m.Text, " ")
	if len(tmp) != 2 {
		logger.Printf("%d Bind error:Wrong Bind Format\n", m.Chat.ID)
		bot.Send(m.Chat, "错误的格式")
		return
	}
	logger.Println("client_id: " + tmp[0] + " client_secret: " + tmp[1])
	cid := tmp[0]
	cse := tmp[1]
	bot.Send(m.Chat, "授权账户： [点击直达]("+MSGetAuthUrl(cid)+")", tb.ModeMarkdown)
	_, err := bot.Send(m.Chat, "请回复http://localhost/…… + 空格 + 别名(用于管理)", &tb.ReplyMarkup{ForceReply: true})
	if err == nil {
		UserStatus[m.Chat.ID] = USBind2
		UserCid[m.Chat.ID] = cid
		UserCSecret[m.Chat.ID] = cse
	}
}

func bUnBind(m *tb.Message) {
	data := QueryDataByTG(db, m.Chat.ID)
	var inlineKeys [][]tb.InlineButton
	for _, u := range data {
		inlineBtn := tb.InlineButton{
			Unique: "unbind" + u.msId,
			Text:   u.alias,
			Data:   u.msId,
		}
		bot.Handle(&inlineBtn, bUnBindInlineBtn)
		inlineKeys = append(inlineKeys, []tb.InlineButton{inlineBtn})
	}
	bot.Send(m.Chat, "选择一个账户将其解绑\n\n当前绑定数: "+strconv.Itoa(GetBindNum(m.Chat.ID))+"/"+strconv.Itoa(BindMaxNum), &tb.ReplyMarkup{InlineKeyboard: inlineKeys})
}
func bUnBindInlineBtn(c *tb.Callback) {
	r := QueryDataByMS(db, c.Data)
	u := r[0]
	if ok, _ := DelData(db, u.msId); !ok {
		logger.Println(u.msId + " UnBind ERROR")
		bot.Send(c.Message.Chat, "解绑失败!")
		return
	}
	logger.Println(u.msId + " UnBind Success")
	bot.Send(c.Message.Chat, "解绑成功!")
	bot.Respond(c)
}

func bHelp(m *tb.Message) {
	bot.Send(m.Sender, bHelpContent+"\n"+notice, &tb.SendOptions{DisableWebPagePreview: false})
}
func bOnText(m *tb.Message) {
	switch UserStatus[m.Chat.ID] {
	case USNone:
		{
			bot.Send(m.Chat, "发送/help获取帮助嗷")
			return
		}
	case USBind1:
		{
			if !m.IsReply() {
				bot.Send(m.Chat, "请通过回复方式绑定")
				return
			}
			bBind2(m)
		}
	case USBind2:
		{
			if !m.IsReply() {
				bot.Send(m.Chat, "请通过回复方式绑定")
				return
			}
			if GetBindNum(m.Chat.ID) == BindMaxNum {
				bot.Send(m.Chat, "已经达到最大可绑定数")
				return
			}
			bot.Send(m.Chat, "正在绑定中……")
			err := BindUser(m, UserCid[m.Chat.ID], UserCSecret[m.Chat.ID])
			if err != nil {
				bot.Send(m.Chat, err.Error())
			} else {
				bot.Send(m.Chat, "绑定成功!")
			}
			UserStatus[m.Chat.ID] = USNone
		}
	}
}
func bTask(m *tb.Message) {
	for _, a := range admin {
		if a == m.Chat.ID {
			SignTask()
			return
		}
	}
	bot.Send(m.Chat, "您没有权限执行此操作~")
}
