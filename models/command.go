package models

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/beego/beego/v2/client/httplib"
	"github.com/beego/beego/v2/server/web"
	"gorm.io/gorm"
)

type CodeSignal struct {
	Command []string
	Admin   bool
	Handle  func(sender *Sender) interface{}
}

type Sender struct {
	UserID            int
	ChatID            int
	Type              string
	Contents          []string
	MessageID         int
	Username          string
	IsAdmin           bool
	ReplySenderUserID int
}

func (sender *Sender) Reply(msg string) {
	switch sender.Type {
	case "tg":
		SendTgMsg(sender.UserID, msg)
	case "tgg":
		SendTggMsg(sender.ChatID, sender.UserID, msg, sender.MessageID, sender.Username)
	case "qq":
		SendQQ(int64(sender.UserID), msg)
	case "qqg":
		SendQQGroup(int64(sender.ChatID), int64(sender.UserID), msg)
	}
}

func (sender *Sender) JoinContens() string {
	return strings.Join(sender.Contents, " ")
}

func (sender *Sender) IsQQ() bool {
	return strings.Contains(sender.Type, "qq")
}

func (sender *Sender) IsTG() bool {
	return strings.Contains(sender.Type, "tg")
}

func (sender *Sender) handleJdCookies(handle func(ck *JdCookie)) error {
	cks := GetJdCookies()
	a := sender.JoinContens()
	ok := false
	if !sender.IsAdmin || a == "" {
		for _, ck := range cks {
			if strings.Contains(sender.Type, "qq") {
				if ck.QQ == sender.UserID {
					if !ok {
						ok = true
					}
					handle(&ck)
				}
			} else if strings.Contains(sender.Type, "tg") {
				if ck.Telegram == sender.UserID {
					if !ok {
						ok = true
					}
					handle(&ck)
				}
			}
		}
		if !ok {
			sender.Reply("你尚未绑定🐶东账号，请对我说扫码，扫码后即可查询账户资产信息。")
			return errors.New("你尚未绑定🐶东账号，请对我说扫码，扫码后即可查询账户资产信息。")
		}
	} else {
		cks = LimitJdCookie(cks, a)
		if len(cks) == 0 {
			sender.Reply("没有匹配的账号")
			return errors.New("没有匹配的账号")
		} else {
			for _, ck := range cks {
				handle(&ck)
			}
		}
	}
	return nil
}

var codeSignals = []CodeSignal{
	{
		Command: []string{"status", "状态"},
		Admin:   true,
		Handle: func(sender *Sender) interface{} {
			return Count()
		},
	},
	{
		Command: []string{"sign", "打卡", "签到"},
		Handle: func(sender *Sender) interface{} {
			if sender.Type == "tgg" {
				sender.Type = "tg"
			}
			if sender.Type == "qqg" {
				sender.Type = "qq"
			}
			zero, _ := time.ParseInLocation("2006-01-02", time.Now().Local().Format("2006-01-02"), time.Local)
			var u User
			var ntime = time.Now()
			var first = false
			total := []int{}
			err := db.Where("class = ? and number = ?", sender.Type, sender.UserID).First(&u).Error
			if err != nil {
				first = true
				u = User{
					Class:    sender.Type,
					Number:   sender.UserID,
					Coin:     1,
					ActiveAt: ntime,
				}
				if err := db.Create(&u).Error; err != nil {
					return err.Error()
				}
			} else {
				if zero.Unix() > u.ActiveAt.Unix() {
					first = true
				} else {
					return fmt.Sprintf("你打过卡了，许愿币余额%d。", u.Coin)
				}
			}
			if first {
				db.Model(User{}).Select("count(id) as total").Where("active_at > ?", zero).Pluck("total", &total)
				coin := 1
				if total[0]%3 == 0 {
					coin = 2
				}
				if total[0]%13 == 0 {
					coin = 8
				}
				db.Model(&u).Updates(map[string]interface{}{
					"active_at": ntime,
					"coin":      gorm.Expr(fmt.Sprintf("coin+%d", coin)),
				})
				u.Coin += coin
				return fmt.Sprintf("你是打卡第%d人，奖励%d个许愿币，许愿币余额%d。", total[0]+1, coin, u.Coin)
			}
			return nil
		},
	},
	{
		Command: []string{"coin", "许愿币"},
		Handle: func(sender *Sender) interface{} {
			return fmt.Sprintf("余额%d", GetCoin(sender.UserID))
		},
	},
	{
		Command: []string{"qrcode", "扫码", "二维码", "scan"},
		Handle: func(sender *Sender) interface{} {
			url := fmt.Sprintf("http://127.0.0.1:%d/api/login/qrcode.png?tp=%s&uid=%d&gid=%d", web.BConfig.Listen.HTTPPort, sender.Type, sender.UserID, sender.ChatID)
			if sender.Type == "tgg" {
				url += fmt.Sprintf("&mid=%v&unm=%v", sender.MessageID, sender.Username)
			}
			rsp, err := httplib.Get(url).Response()
			if err != nil {
				return nil
			}
			return rsp
		},
	},
	{
		Command: []string{"升级", "更新", "update", "upgrade"},
		Admin:   true,
		Handle: func(sender *Sender) interface{} {
			if err := Update(sender); err != nil {
				return err.Error()
			}
			sender.Reply("小滴滴重启程序")
			Daemon()
			return nil
		},
	},
	{
		Command: []string{"重启", "reload", "restart", "reboot"},
		Admin:   true,
		Handle: func(sender *Sender) interface{} {
			sender.Reply("小滴滴重启程序")
			Daemon()
			return nil
		},
	},
	{
		Command: []string{"get-ua", "ua"},
		Handle: func(sender *Sender) interface{} {
			if !sender.IsAdmin {
				coin := GetCoin(sender.UserID)
				if coin < 0 {
					return "许愿币不足以查看UserAgent。"
				}
				sender.Reply("查看一次扣1个许愿币。")
				RemCoin(sender.UserID, 1)
			}
			return ua
		},
	},
	{
		Command: []string{"set-ua"},
		Admin:   true,
		Handle: func(sender *Sender) interface{} {
			ctt := sender.JoinContens()
			db.Create(&UserAgent{Content: ctt})
			ua = ctt
			return "已更新User-Agent。"
		},
	},
	{
		Command: []string{"任务列表"},
		Admin:   true,
		Handle: func(_ *Sender) interface{} {
			rt := ""
			for i := range Config.Repos {
				for j := range Config.Repos[i].Task {
					rt += fmt.Sprintf("%s\t%s\n", Config.Repos[i].Task[j].Title, Config.Repos[i].Task[j].Cron)
				}
			}
			return rt
		},
	},
	{
		Command: []string{"查询", "query"},
		Handle: func(sender *Sender) interface{} {
			sender.handleJdCookies(func(ck *JdCookie) {
				sender.Reply(ck.Query())
			})
			return nil
		},
	},
	{
		Command: []string{"发送", "send"},
		Admin:   true,
		Handle: func(sender *Sender) interface{} {
			b.Send(tgg, sender.JoinContens())
			return nil
		},
	},
	{
		Command: []string{"许愿", "wish", "hope", "want"},
		Handle: func(sender *Sender) interface{} {
			b := GetCoin(sender.UserID)
			if b < 25 {
				return "许愿币不足，需要25个许愿币。"
			}
			(&JdCookie{}).Push(fmt.Sprintf("%d许愿%s，许愿币余额%d。", sender.UserID, sender.JoinContens(), b))
			return fmt.Sprintf("收到许愿，已扣除25个许愿币，余额%d。", RemCoin(sender.UserID, 25))
		},
	},
	{
		Command: []string{"run", "执行", "运行"},
		Admin:   true,
		Handle: func(sender *Sender) interface{} {
			name := sender.Contents[0]
			pins := ""
			if len(sender.Contents) > 1 {
				sender.Contents = sender.Contents[1:]
				err := sender.handleJdCookies(func(ck *JdCookie) {
					pins += "&" + ck.PtPin
				})
				if err != nil {
					return nil
				}
			}
			envs := []Env{}
			if pins != "" {
				envs = append(envs, Env{
					Name:  "pins",
					Value: pins,
				})
			}
			runTask(&Task{Path: name, Envs: envs}, sender)
			return nil
		},
	},
	{
		Command: []string{"cmd", "command", "命令"},
		Admin:   true,
		Handle: func(sender *Sender) interface{} {
			ct := sender.JoinContens()
			if regexp.MustCompile(`rm\s+-rf`).FindString(ct) != "" {
				return "over"
			}
			cmd(ct, sender)
			return nil
		},
	},
	{
		Command: []string{"环境变量", "environments", "envs"},
		Admin:   true,
		Handle: func(_ *Sender) interface{} {
			rt := []string{}
			envs := GetEnvs()
			if len(envs) == 0 {
				return "未设置任何环境变量"
			}
			for _, env := range envs {
				rt = append(rt, fmt.Sprintf(`%s="%s"`, env.Name, env.Value))
			}
			return strings.Join(rt, "\n")
		},
	},
	{
		Command: []string{"get-env", "env", "e"},
		Handle: func(sender *Sender) interface{} {
			ct := sender.JoinContens()
			if ct == "" {
				return "未指定变量名"
			}
			value := GetEnv(ct)
			if value == "" {
				return "未设置环境变量"
			}
			return fmt.Sprintf("环境变量的值为：" + value)
		},
	},
	{
		Command: []string{"set-env", "se", "export"},
		Admin:   true,
		Handle: func(sender *Sender) interface{} {
			env := &Env{}
			if len(sender.Contents) >= 2 {
				env.Name = sender.Contents[0]
				env.Value = strings.Join(sender.Contents[1:], " ")
			} else if len(sender.Contents) == 1 {
				ss := regexp.MustCompile(`^([^'"=]+)=['"]?([^=]+?)['"]?$`).FindStringSubmatch(sender.Contents[0])
				if len(ss) != 3 {
					return "无法解析"
				}
				env.Name = ss[1]
				env.Value = ss[2]
			} else {
				return "???"
			}
			ExportEnv(env)
			return "操作成功"
		},
	},
	{
		Command: []string{"unset-env", "ue", "unexport", "de"},
		Admin:   true,
		Handle: func(sender *Sender) interface{} {
			UnExportEnv(&Env{
				Name: sender.JoinContens(),
			})
			return "操作成功"
		},
	},
	{
		Command: []string{"降级"},
		Handle: func(sender *Sender) interface{} {
			return "滚"
		},
	},
	{
		Command: []string{"。。。"},
		Handle: func(sender *Sender) interface{} {
			return "你很无语吗？"
		},
	},
	{
		Command: []string{"祈祷"},
		Handle: func(sender *Sender) interface{} {
			if _, ok := mx[sender.UserID]; ok {
				return "你祈祷过啦，等下次我忘记了再来吧。"
			}
			mx[sender.UserID] = true
			AddCoin(sender.UserID)
			return "许愿币+1"
		},
	},
	{
		Command: []string{"退还许愿币"},
		Admin:   true,
		Handle: func(sender *Sender) interface{} {
			if len(sender.Contents) == 2 {
				db.Model(User{}).Where("number = " + sender.Contents[1]).Updates(map[string]interface{}{
					"coin": gorm.Expr("coin+" + sender.Contents[1]),
				})
				return "操作成功"
			} else {
				return "操作异常"
			}
		},
	},
	{
		Command: []string{"reply", "回复"},
		Admin:   true,
		Handle: func(sender *Sender) interface{} {
			if len(sender.Contents) >= 2 {
				replies[sender.Contents[0]] = strings.Join(sender.Contents[1:], " ")
			} else {
				return "操作失败"
			}
			return "操作成功"
		},
	},
	{
		Command: []string{"help", "助力"},
		Admin:   true,
		Handle: func(sender *Sender) interface{} {
			sender.handleJdCookies(func(ck *JdCookie) {
				ck.Update(Help, True)
				sender.Reply(fmt.Sprintf("已设置助力账号%s(%s)", ck.PtPin, ck.Nickname))
			})
			return nil
		},
	},
	{
		Command: []string{"tool", "工具人", "unhelp", "取消助力"},
		Admin:   true,
		Handle: func(sender *Sender) interface{} {
			sender.handleJdCookies(func(ck *JdCookie) {
				ck.Update(Help, False)
				sender.Reply(fmt.Sprintf("已设置取消助力账号%s(%s)", ck.PtPin, ck.Nickname))
			})
			return nil
		},
	},
	{
		Command: []string{"屏蔽", "hack"},
		Admin:   true,
		Handle: func(sender *Sender) interface{} {
			sender.handleJdCookies(func(ck *JdCookie) {
				ck.Update(Hack, True)
				sender.Reply(fmt.Sprintf("已设置屏蔽助力账号%s(%s)", ck.PtPin, ck.Nickname))
			})
			return nil
		},
	},
	{
		Command: []string{"取消屏蔽", "unhack"},
		Admin:   true,
		Handle: func(sender *Sender) interface{} {
			sender.handleJdCookies(func(ck *JdCookie) {
				ck.Update(Hack, False)
				sender.Reply(fmt.Sprintf("已设置取消屏蔽助力账号%s(%s)", ck.PtPin, ck.Nickname))
			})
			return nil
		},
	},
	{
		Command: []string{"转账"},
		Handle: func(sender *Sender) interface{} {
			if sender.ReplySenderUserID == 0 {
				return "没有转账目标"
			}
			if len(sender.Contents) != 1 {
				return "未设置转账金额"
			}
			amount := Int(sender.Contents[0])
			if !sender.IsAdmin {
				if amount <= 0 {
					return "转账金额必须大于0"
				}
				remain := GetCoin(sender.UserID)
				if remain <= 0 {
					return "余额不足"
				}
			}
			tx := db.Begin()
			if tx.Model(User{}).Where("number = ?", sender.UserID).Updates(map[string]interface{}{
				"coin": gorm.Expr(fmt.Sprintf("coin - %d", amount)),
			}).RowsAffected == 0 {
				tx.Rollback()
				return "转账失败"
			}
			if tx.Model(User{}).Where("number = ?", sender.ReplySenderUserID).Updates(map[string]interface{}{
				"coin": gorm.Expr(fmt.Sprintf("coin + %d", amount)),
			}).RowsAffected == 0 {
				tx.Rollback()
				return "转账失败"
			}
			tx.Commit()
			return "转账成功"
		},
	},
}

var mx = map[int]bool{}

func LimitJdCookie(cks []JdCookie, a string) []JdCookie {
	ncks := []JdCookie{}
	if s := strings.Split(a, "-"); len(s) == 2 {
		for i, ck := range cks {
			if i+1 >= Int(s[0]) && i+1 <= Int(s[1]) {
				ncks = append(ncks, ck)
			}
		}
	} else if x := regexp.MustCompile(`^[\s\d,]+$`).FindString(a); x != "" {
		xx := regexp.MustCompile(`(\d+)`).FindAllStringSubmatch(a, -1)
		for i, ck := range cks {
			for _, x := range xx {
				if fmt.Sprint(i+1) == x[1] {
					ncks = append(ncks, ck)
				}
			}

		}
	} else if a != "" {
		a = strings.Replace(a, " ", "", -1)
		for _, ck := range cks {
			if strings.Contains(ck.Note, a) || strings.Contains(ck.Nickname, a) || strings.Contains(ck.PtPin, a) {
				ncks = append(ncks, ck)
			}
		}
	}
	return ncks
}
