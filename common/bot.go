package common

import (
	"fmt"
	"github.com/eatmoreapple/openwechat"
	"github.com/skip2/go-qrcode"
	"sync"
)


//目标群
var targetGroup *openwechat.Group
var mu sync.Mutex

//打印二维码到控制台
func consoleQrcode(uuid string) {
	q, _ := qrcode.New("https://login.weixin.qq.com/l/"+uuid, qrcode.Low)
	fmt.Println(q.ToSmallString(true))
}

//需要开一个协程运行
func StartBot(targetGroupName string){
	var err error
	fmt.Println("startBot")
	bot := openwechat.DefaultBot(openwechat.Desktop)

	//打印二维码到控制台	
	bot.UUIDCallback = consoleQrcode

	//热登录，第一次会生成一个storage.json文件，用于存储登录信息。另一种方式是不扫码登录和每次都扫描登录，但是不扫码登录但是发现不行
	reloadStorage := openwechat.NewFileHotReloadStorage("storage.json")
	defer reloadStorage.Close()
	if err = bot.HotLogin(reloadStorage, openwechat.NewRetryLoginOption()); err != nil {
		fmt.Println("loginErr",err)
		return
	}

	//消息处理函数,单聊或者@ta/所有人时会触发
	//之后可以考虑接入大模型/连接数据库
	bot.MessageHandler = func(msg *openwechat.Message) {
		if msg.IsText() {
			fmt.Println(msg.Content)
			msg.ReplyText("你好")
		}
	}

	//获取登录的用户
	self, err := bot.GetCurrentUser()
	if err != nil {
		fmt.Println("getCurrentUserErr",err)
		return
	}
	//获取群组，必须是手机端通讯录里保存的群
	groups, err := self.Groups()
	if err != nil {
		fmt.Println("getGroupsErr",err)
		return
	}
	//获取目标群，按群名获取
	targetGroup = groups.GetByNickName(targetGroupName)
	if targetGroup == nil {
		fmt.Println("groupNotFound")
		return
	}

	if err = bot.Block(); err != nil {
		fmt.Println("logout",err)
		return
	}
}

//看数据库好像是这样返回的而不是直接使用全局变量？
func GetGroup() *openwechat.Group {
	mu.Lock()
	defer mu.Unlock()
	return targetGroup
}
