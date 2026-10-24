// Package main ZeroBot-Plugin main file
package main

//go:generate go run github.com/FloatTech/ZeroBot-Plugin/abineundo/ref -r .

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"math/rand"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	_ "github.com/FloatTech/ZeroBot-Plugin/abineundo" // 设置插件优先级&更改控制台属性
	"github.com/FloatTech/ZeroBot-Plugin/internal/aicenter"
	"github.com/FloatTech/ZeroBot-Plugin/internal/basiccommands"
	"github.com/FloatTech/ZeroBot-Plugin/internal/batteryhub"
	"github.com/FloatTech/ZeroBot-Plugin/internal/localadmin"
	"github.com/FloatTech/ZeroBot-Plugin/internal/modelstats"
	"github.com/FloatTech/ZeroBot-Plugin/internal/processidentity"
	"github.com/FloatTech/ZeroBot-Plugin/internal/replystats"
	updatewatchcore "github.com/FloatTech/ZeroBot-Plugin/internal/updatewatch"
	"github.com/FloatTech/ZeroBot-Plugin/kanban" // 打印 banner

	// ---------以下插件均可通过前面加 // 注释，注释后停用并不加载插件--------- //
	// ----------------------插件优先级按顺序从高到低---------------------- //
	//                                                                  //
	//                                                                  //
	//                                                                  //
	//                                                                  //
	//                                                                  //
	// ----------------------------高优先级区---------------------------- //
	// vvvvvvvvvvvvvvvvvvvvvvvvvvvv高优先级区vvvvvvvvvvvvvvvvvvvvvvvvvvvv //
	//               vvvvvvvvvvvvvv高优先级区vvvvvvvvvvvvvv               //
	//                      vvvvvvv高优先级区vvvvvvv                      //
	//                          vvvvvvvvvvvvvv                          //
	//                               vvvv                               //

	_ "github.com/FloatTech/ZeroBot-Plugin/plugin/antiabuse" // 违禁词

	_ "github.com/FloatTech/ZeroBot-Plugin/plugin/chat" // 戳一戳互动

	_ "github.com/FloatTech/ZeroBot-Plugin/plugin/airecord" // 群应用：AI声聊

	_ "github.com/FloatTech/ZeroBot-Plugin/plugin/manager" // 群管

	_ "github.com/FloatTech/zbputils/job" // 定时指令触发器

	//                               ^^^^                               //
	//                          ^^^^^^^^^^^^^^                          //
	//                      ^^^^^^^高优先级区^^^^^^^                      //
	//               ^^^^^^^^^^^^^^高优先级区^^^^^^^^^^^^^^               //
	// ^^^^^^^^^^^^^^^^^^^^^^^^^^^^高优先级区^^^^^^^^^^^^^^^^^^^^^^^^^^^^ //
	// ----------------------------高优先级区---------------------------- //
	//                                                                  //
	//                                                                  //
	//                                                                  //
	//                                                                  //
	//                                                                  //
	// ----------------------------中优先级区---------------------------- //
	// vvvvvvvvvvvvvvvvvvvvvvvvvvvv中优先级区vvvvvvvvvvvvvvvvvvvvvvvvvvvv //
	//               vvvvvvvvvvvvvv中优先级区vvvvvvvvvvvvvv               //
	//                      vvvvvvv中优先级区vvvvvvv                      //
	//                          vvvvvvvvvvvvvv                          //
	//                               vvvv                               //

	_ "github.com/FloatTech/ZeroBot-Plugin/custom"                   // 自定义插件合集
	_ "github.com/FloatTech/ZeroBot-Plugin/plugin/aifalse"           // 服务器监控
	_ "github.com/FloatTech/ZeroBot-Plugin/plugin/aiimage"           // AI画图
	_ "github.com/FloatTech/ZeroBot-Plugin/plugin/bilibiliparse"     // b站相关
	_ "github.com/FloatTech/ZeroBot-Plugin/plugin/ctb"               // FTB整合包工具
	_ "github.com/FloatTech/ZeroBot-Plugin/plugin/event"             // 好友申请群聊邀请事件处理
	_ "github.com/FloatTech/ZeroBot-Plugin/plugin/mcfish"            // 钓鱼模拟器
	_ "github.com/FloatTech/ZeroBot-Plugin/plugin/minecraftobserver" // Minecraft服务器监控&订阅
	_ "github.com/FloatTech/ZeroBot-Plugin/plugin/qqwife"            // 一群一天一夫一妻制群老婆
	_ "github.com/FloatTech/ZeroBot-Plugin/plugin/score"             // 分数
	_ "github.com/FloatTech/ZeroBot-Plugin/plugin/tarot"             // 抽塔罗牌
	_ "github.com/FloatTech/ZeroBot-Plugin/plugin/updatewatch"       // 官方开发进展监控
	_ "github.com/FloatTech/ZeroBot-Plugin/plugin/wife"              // 抽老婆
	_ "github.com/FloatTech/ZeroBot-Plugin/plugin/wordle"            // 猜单词

	// _ "github.com/FloatTech/ZeroBot-Plugin/plugin/wtf"           // 鬼东西

	//                               ^^^^                               //
	//                          ^^^^^^^^^^^^^^                          //
	//                      ^^^^^^^中优先级区^^^^^^^                      //
	//               ^^^^^^^^^^^^^^中优先级区^^^^^^^^^^^^^^               //
	// ^^^^^^^^^^^^^^^^^^^^^^^^^^^^中优先级区^^^^^^^^^^^^^^^^^^^^^^^^^^^^ //
	// ----------------------------中优先级区---------------------------- //
	//                                                                  //
	//                                                                  //
	//                                                                  //
	//                                                                  //
	//                                                                  //
	// ----------------------------低优先级区---------------------------- //
	// vvvvvvvvvvvvvvvvvvvvvvvvvvvv低优先级区vvvvvvvvvvvvvvvvvvvvvvvvvvvv //
	//               vvvvvvvvvvvvvv低优先级区vvvvvvvvvvvvvv               //
	//                      vvvvvvv低优先级区vvvvvvv                      //
	//                          vvvvvvvvvvvvvv                          //
	//                               vvvv                               //

	_ "github.com/FloatTech/ZeroBot-Plugin/plugin/aichat" // AI中心

	_ "github.com/FloatTech/ZeroBot-Plugin/plugin/breakrepeat" // 打断复读

	//                               ^^^^                               //
	//                          ^^^^^^^^^^^^^^                          //
	//                      ^^^^^^^低优先级区^^^^^^^                      //
	//               ^^^^^^^^^^^^^^低优先级区^^^^^^^^^^^^^^               //
	// ^^^^^^^^^^^^^^^^^^^^^^^^^^^^低优先级区^^^^^^^^^^^^^^^^^^^^^^^^^^^^ //
	// ----------------------------低优先级区---------------------------- //
	//                                                                  //
	//                                                                  //
	//                                                                  //
	//                                                                  //
	//                                                                  //
	// -----------------------以下为内置依赖，勿动------------------------ //
	"github.com/FloatTech/floatbox/file"
	"github.com/FloatTech/floatbox/process"
	ctrl "github.com/FloatTech/zbpctrl"
	"github.com/FloatTech/zbputils/control"
	"github.com/sirupsen/logrus"
	zero "github.com/wdvxdr1123/ZeroBot"
	"github.com/wdvxdr1123/ZeroBot/driver"
	"github.com/wdvxdr1123/ZeroBot/message"

	// webctrl "github.com/FloatTech/zbputils/control/web"

	"github.com/FloatTech/ZeroBot-Plugin/kanban/banner"
	// -----------------------以上为内置依赖，勿动------------------------ //
)

type zbpcfg struct {
	Z               zero.Config        `json:"zero"`
	W               []*driver.WSClient `json:"ws"`
	S               []*driver.WSServer `json:"wss"`
	ForceBase64File bool               `json:"force_base64_file"`
}

var config zbpcfg
var startupConfigPath string
var startupConfigJSON []byte

var (
	adminEnabled bool
	adminAddr    string
	adminDataDir string
)

func init() {
	sus := make([]int64, 0, 16)
	// 解析命令行参数
	d := flag.Bool("d", false, "Enable debug level log and higher.")
	w := flag.Bool("w", false, "Enable warning level log and higher.")
	h := flag.Bool("h", false, "Display this help.")
	// g := flag.String("g", "127.0.0.1:3000", "Set webui url.")
	// 直接写死 AccessToken 时，请更改下面第二个参数
	token := flag.String("t", "", "Set AccessToken of WSClient.")
	// 直接写死 URL 时，请更改下面第二个参数
	url := flag.String("u", "ws://127.0.0.1:6700", "Set Url of WSClient.")
	// 默认昵称
	adana := flag.String("n", "椛椛", "Set default nickname.")
	prefix := flag.String("p", "/", "Set command prefix.")
	runcfg := flag.String("c", "", "Run from config file.")
	save := flag.String("s", "", "Save default config to file and exit.")
	late := flag.Uint("l", 233, "Response latency (ms).")
	rsz := flag.Uint("r", 4096, "Receiving buffer ring size.")
	maxpt := flag.Uint("x", 4, "Max process time (min).")
	markmsg := flag.Bool("m", false, "Don't mark message as read automatically")
	fb64 := flag.Bool("fb64", false, "Force to send base64 file.")
	flag.BoolVar(&file.SkipOriginal, "mirror", false, "Use mirrored lazy data at first")
	flag.BoolVar(&adminEnabled, "admin", false, "Enable the local admin UI.")
	flag.StringVar(&adminAddr, "admin-addr", "127.0.0.1:3010", "Set loopback local admin address.")
	flag.StringVar(&adminDataDir, "admin-data", ".local/admin", "Set private local admin credential directory.")

	flag.Parse()

	if *h {
		fmt.Println("Usage:")
		flag.PrintDefaults()
		os.Exit(0)
	}
	if *d && !*w {
		logrus.SetLevel(logrus.DebugLevel)
	}
	if *w {
		logrus.SetLevel(logrus.WarnLevel)
	}

	for _, s := range flag.Args() {
		i, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			continue
		}
		sus = append(sus, i)
	}

	// 通过代码写死的方式添加主人账号
	// sus = append(sus, 12345678)
	// sus = append(sus, 87654321)

	// 启用 webui
	// go webctrl.RunGui(*g)

	if *runcfg != "" {
		f, err := os.Open(*runcfg)
		if err != nil {
			panic(err)
		}
		config.W = make([]*driver.WSClient, 0, 2)
		startupConfigJSON, err = io.ReadAll(io.LimitReader(f, (1<<20)+1))
		f.Close()
		if err == nil && len(startupConfigJSON) > 1<<20 {
			err = fmt.Errorf("启动配置超过 1 MiB")
		}
		if err == nil {
			err = json.Unmarshal(startupConfigJSON, &config)
		}
		if err != nil {
			panic(err)
		}
		startupConfigPath = *runcfg
		config.Z.Driver = make([]zero.Driver, len(config.W)+len(config.S))
		for i, w := range config.W {
			config.Z.Driver[i] = w
		}
		for i, s := range config.S {
			config.Z.Driver[i+len(config.W)] = s
		}
		logrus.Infoln("[main] 从", *runcfg, "读取配置文件")
		return
	}
	config.W = []*driver.WSClient{driver.NewWebSocketClient(*url, *token)}
	config.Z = zero.Config{
		NickName:       append([]string{*adana}, "ATRI", "atri", "亚托莉", "アトリ"),
		CommandPrefix:  *prefix,
		SuperUsers:     sus,
		RingLen:        *rsz,
		Latency:        time.Duration(*late) * time.Millisecond,
		MaxProcessTime: time.Duration(*maxpt) * time.Minute,
		MarkMessage:    !*markmsg,
		Driver:         []zero.Driver{config.W[0]},
	}
	config.ForceBase64File = *fb64

	if *save != "" {
		f, err := os.Create(*save)
		if err != nil {
			panic(err)
		}
		err = json.NewEncoder(f).Encode(&config)
		f.Close()
		if err != nil {
			panic(err)
		}
		logrus.Infoln("[main] 配置文件已保存到", *save)
		os.Exit(0)
	}
}

func main() {
	if adminEnabled {
		plugins := []localadmin.Plugin{}
		controls := []*ctrl.Control[*zero.Ctx]{}
		control.ForEachByPrio(func(_ int, manager *ctrl.Control[*zero.Ctx]) bool {
			plugins = append(plugins, localadmin.Plugin{Name: manager.Service, Brief: manager.Options.Brief, Usage: manager.Options.Help, DefaultEnabledAtStartup: !manager.Options.DisableOnDefault})
			controls = append(controls, manager)
			return true
		})
		var connections *localadmin.ConnectionStore
		if startupConfigPath != "" {
			var err error
			connections, err = localadmin.NewConnectionStore(startupConfigPath, filepath.Join(adminDataDir, "backups"), startupConfigJSON)
			if err != nil {
				logrus.Fatalln("[admin] 启动配置不可管理:", err)
			}
		}
		pluginControl := localadmin.NewPluginStore(controls, filepath.Join(adminDataDir, "backups"))
		logs := localadmin.NewLogStore()
		logConfig, _ := json.Marshal(config)
		logs.ProtectConfig(logConfig)
		backups := localadmin.NewBackupStore(filepath.Join(adminDataDir, "backups"), connections, pluginControl, logs)
		identity, identityErr := processidentity.Read()
		if identityErr != nil {
			logrus.Warnln("[admin] 观察报告进程信息不可读取，保留无关联报告")
		}
		admin, err := localadmin.Start(localadmin.Config{Addr: adminAddr, DataDir: adminDataDir, Plugins: plugins, PluginControl: pluginControl, Connections: connections, AI: localadmin.NewUnifiedAIStore(controls, aicenter.Settings, aicenter.Commands), UpdateWatch: updatewatchcore.RuntimeAdmin, Logs: logs, Backups: backups, ModelRequests: modelstats.Default.Read, ReplyTasks: replystats.Default.Read, ProcessIdentity: identity, Overview: localadmin.RuntimeOverview(time.Now(), len(config.W)+len(config.S), len(plugins))})
		if err != nil {
			logrus.Fatalln("[admin] 本地后台启动失败:", err)
		}
		logrus.AddHook(logs)
		logger := logrus.StandardLogger()
		logger.SetFormatter(localadmin.RedactingFormatter{Logs: logs, Next: logger.Formatter})
		defer func() {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			_ = admin.Close(ctx)
		}()
		logrus.Infoln("[admin] 本地后台已启动: http://" + adminAddr)
	}
	if !strings.Contains(runtime.Version(), "go1.2") { // go1.20之前版本需要全局 seed，其他插件无需再 seed
		rand.Seed(time.Now().UnixNano()) //nolint: staticcheck
	}
	message.SetForceBase64File(config.ForceBase64File)
	// 帮助
	basiccommands.RegisterHelp(banner.Banner)
	zero.OnFullMatch("查看zbp公告", zero.OnlyToMe, zero.AdminPermission).SetBlock(true).
		Handle(func(ctx *zero.Ctx) {
			ctx.SendChain(message.Text(strings.ReplaceAll(kanban.Kanban(), "\t", "")))
		})
	batteryhub.Start(config.Z.SuperUsers)
	zero.RunAndBlock(&config.Z, process.GlobalInitMutex.Unlock)
}
