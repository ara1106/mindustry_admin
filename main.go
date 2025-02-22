package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"math/rand"
	"os"
	"os/exec"
	"os/signal"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/kortemy/lingo"
	"github.com/larspensjo/config"
	"github.com/robfig/cron"
)

var _VERSION_ = "1.0"

const ansi = "[\u001B\u009B][[\\]()#;?]*(?:(?:(?:[a-zA-Z\\d]*(?:;[a-zA-Z\\d]*)*)?\u0007)|(?:(?:\\d{1,4}(?:;\\d{0,4})*)?[\\dA-PRZcf-ntqry=><~]))"

var re = regexp.MustCompile(ansi)

func StripColor(str string) string {
	return re.ReplaceAllString(str, "")
}

type UserCmdProcHandle func(in io.WriteCloser, userName string, userInput string, isOnlyCheck bool) bool

type User struct {
	name         string
	isAdmin      bool
	isSuperAdmin bool
	level        int
}
type Cmd struct {
	name   string
	level  int
	isVote bool
}

type Mindustry struct {
	name               string
	admins             []string
	cfgAdmin           string
	cfgSuperAdmin      string
	jarPath            string
	users              map[string]User
	votetickUsers      map[string]int
	serverOutR         *regexp.Regexp
	cfgAdminCmds       string
	cfgSuperAdminCmds  string
	cfgNormCmds        string
	cfgVoteCmds        string
	cmds               map[string]Cmd
	cmdHelps           map[string]string
	port               int
	mode               string
	cmdFailReason      string
	currProcCmd        string
	notice             string //cron task auto notice msg
	playCnt            int
	serverIsStart      bool
	serverIsRun        bool
	maps               []string
	userCmdProcHandles map[string]UserCmdProcHandle
	l                  *lingo.L
	i18n               lingo.T
}

func (this *Mindustry) loadConfig() {
	this.l = lingo.New("en_US", "./locale")
	cfg, err := config.ReadDefault("config.ini")
	if err != nil {
		log.Println("[ini]not find config.ini,use default config")
		return
	}
	if cfg.HasSection("server") {
		_, err := cfg.SectionOptions("server")
		if err == nil {
			optionValue := ""
			optionValue, err = cfg.String("server", "admins")
			if err == nil {
				optionValue := strings.TrimSpace(optionValue)
				admins := strings.Split(optionValue, ",")
				this.cfgAdmin = optionValue
				log.Printf("[ini]found admins:%v\n", admins)
				for _, admin := range admins {
					this.addUser(admin)
					this.addAdmin(admin)
				}
			}
			optionValue, err = cfg.String("server", "superAdmins")
			if err == nil {
				optionValue := strings.TrimSpace(optionValue)
				supAdmins := strings.Split(optionValue, ",")
				this.cfgSuperAdmin = optionValue
				log.Printf("[ini]found supAdmins:%v\n", supAdmins)
				for _, supAdmin := range supAdmins {
					this.addUser(supAdmin)
					this.addSuperAdmin(supAdmin)
				}
			}
			optionValue, err = cfg.String("server", "superAdminCmds")
			if err == nil {
				optionValue := strings.TrimSpace(optionValue)
				cmds := strings.Split(optionValue, ",")
				this.cfgSuperAdminCmds = optionValue
				log.Printf("[ini]found superAdminCmds:%v\n", cmds)
				for _, cmd := range cmds {
					this.cmds[cmd] = Cmd{cmd, 9, false}
				}
			}

			optionValue, err = cfg.String("server", "adminCmds")
			if err == nil {
				optionValue := strings.TrimSpace(optionValue)
				cmds := strings.Split(optionValue, ",")
				log.Printf("[ini]found adminCmds:%v\n", cmds)
				this.cfgAdminCmds = optionValue
				for _, cmd := range cmds {
					this.cmds[cmd] = Cmd{cmd, 1, false}
				}
			}
			optionValue, err = cfg.String("server", "normCmds")
			if err == nil {
				optionValue := strings.TrimSpace(optionValue)
				cmds := strings.Split(optionValue, ",")
				log.Printf("[ini]found normCmds:%v\n", cmds)
				this.cfgNormCmds = optionValue
				for _, cmd := range cmds {
					this.cmds[cmd] = Cmd{cmd, 0, false}
				}
			}
			optionValue, err = cfg.String("server", "votetickCmds")
			if err == nil {
				optionValue := strings.TrimSpace(optionValue)
				cmds := strings.Split(optionValue, ",")
				log.Printf("[ini]found votetickCmds:%v\n", cmds)
				this.cfgVoteCmds = optionValue
				for _, cmd := range cmds {
					if c, ok := this.cmds[cmd]; ok {
						c.isVote = true
						this.cmds[cmd] = c
					} else {
						log.Printf("[ini]votetick not found cmd:%s\n", cmd)
					}
				}
			}

			optionValue, err = cfg.String("server", "name")
			if err == nil {
				name := strings.TrimSpace(optionValue)
				this.name = name
			}
			optionValue, err = cfg.String("server", "jarPath")
			if err == nil {
				jarPath := strings.TrimSpace(optionValue)
				this.jarPath = jarPath
			}
			optionValue, err = cfg.String("server", "notice")
			if err == nil {
				notice := strings.TrimSpace(optionValue)
				this.notice = notice
			}

			optionValue, err = cfg.String("server", "language")
			if err == nil {
				languageCfg := strings.TrimSpace(optionValue)
				if languageCfg != "" {
					log.Printf("[ini]lanage cfg:%s\n", languageCfg)
					this.i18n = this.l.TranslationsForLocale(languageCfg)
				} else {
					this.i18n = this.l.TranslationsForLocale("en_US")
					log.Printf("[ini]lanage cfg invalid,use english\n")
				}
			} else {
				this.i18n = this.l.TranslationsForLocale("en_US")
				log.Printf("[ini]lanage cfg invalid,use english\n")
			}
		}
	}

}
func (this *Mindustry) init() {
	this.serverOutR, _ = regexp.Compile(".*(\\[INFO\\]|\\[ERR\\])(.*)")
	this.users = make(map[string]User)
	this.votetickUsers = make(map[string]int)
	this.cmds = make(map[string]Cmd)
	this.cmdHelps = make(map[string]string)
	this.userCmdProcHandles = make(map[string]UserCmdProcHandle)
	rand.Seed(time.Now().UnixNano())
	this.name = fmt.Sprintf("mindustry-%d", rand.Int())
	this.jarPath = "server-release.jar"
	this.serverIsStart = true
	this.loadConfig()
	this.addUser("Server")
	this.addSuperAdmin("Server")
	this.userCmdProcHandles["admin"] = this.proc_admin
	this.userCmdProcHandles["directCmd"] = this.proc_directCmd
	this.userCmdProcHandles["gameover"] = this.proc_gameover
	this.userCmdProcHandles["help"] = this.proc_help
	this.userCmdProcHandles["host"] = this.proc_host
	this.userCmdProcHandles["hostx"] = this.proc_host
	this.userCmdProcHandles["save"] = this.proc_save
	this.userCmdProcHandles["load"] = this.proc_load
	this.userCmdProcHandles["maps"] = this.proc_mapsOrStatus
	this.userCmdProcHandles["status"] = this.proc_mapsOrStatus
	this.userCmdProcHandles["slots"] = this.proc_slots
	this.userCmdProcHandles["showAdmin"] = this.proc_showAdmin
	this.userCmdProcHandles["show"] = this.proc_show
	this.userCmdProcHandles["votetick"] = this.proc_votetick

}

func (this *Mindustry) execCommand(commandName string, params []string) error {
	cmd := exec.Command(commandName, params...)
	fmt.Println(cmd.Args)
	stdout, outErr := cmd.StdoutPipe()
	stdin, inErr := cmd.StdinPipe()
	if outErr != nil {
		return outErr
	}

	if inErr != nil {
		return inErr
	}
	cmd.Start()
	go func(cmd *exec.Cmd) {
		c := make(chan os.Signal)
		signal.Notify(c, os.Interrupt, os.Kill)
		s := <-c
		if cmd.Process != nil {
			log.Printf("sub process exit:%s", s)
			cmd.Process.Kill()
		}
	}(cmd)
	c := cron.New()
	spec := "0 0 * * * ?"
	c.AddFunc(spec, func() {
		this.hourTask(stdin)
	})
	spec = "0 5/10 * * * ?"
	c.AddFunc(spec, func() {
		this.tenMinTask(stdin)
	})
	c.Start()
	go func(cmd *exec.Cmd) {
		reader := bufio.NewReader(os.Stdin)
		for {
			line, err2 := reader.ReadString('\n')
			if err2 != nil || io.EOF == err2 {
				break
			}
			inputCmd := strings.TrimRight(line, "\n")
			if inputCmd == "stop" || inputCmd == "exit" {
				this.serverIsStart = false
				this.serverIsRun = false
			}
			if inputCmd == "host" || inputCmd == "load" {
				this.serverIsStart = true
			}
			this.execCmd(stdin, inputCmd)
		}
	}(cmd)

	reader := bufio.NewReader(stdout)

	for {
		line, err2 := reader.ReadString('\n')
		if err2 != nil || io.EOF == err2 {
			break
		}
		fmt.Printf(line)
		this.output(StripColor(line), stdin)
	}
	cmd.Wait()
	return nil
}
func (this *Mindustry) hourTask(in io.WriteCloser) {
	hour := time.Now().Hour()
	log.Printf("hourTask trig:%d\n", hour)
	if this.serverIsRun {
		this.execCmd(in, "save "+strconv.Itoa(hour))
		this.say(in, "info.auto_save", hour)
	} else {
		log.Printf("game is not running.\n")
	}
}

func (this *Mindustry) tenMinTask(in io.WriteCloser) {
	log.Printf("tenMinTask trig[%.3f°C].\n", getCpuTemp())

	if !this.serverIsStart {
		return
	}
	if !this.serverIsRun {
		log.Printf("game is not running,exit.\n")
		this.execCmd(in, "exit")
	} else {
		this.say(in, this.notice)
		log.Printf("update game status.\n")
		this.currProcCmd = "status"
		this.execCmd(in, "status ")
	}
}
func (this *Mindustry) addUser(name string) {
	if _, ok := this.users[name]; ok {
		return
	}
	this.users[name] = User{name, false, false, 0}
	log.Printf("add user info :%s\n", name)
}
func (this *Mindustry) addAdmin(name string) {
	if _, ok := this.users[name]; !ok {
		log.Printf("user %s not found\n", name)
		return
	}
	tempUser := this.users[name]
	tempUser.isAdmin = true
	tempUser.level = 1
	this.users[name] = tempUser
	log.Printf("add admin :%s\n", name)
}

func (this *Mindustry) addSuperAdmin(name string) {
	if _, ok := this.users[name]; !ok {
		log.Printf("user %s not found\n", name)
		return
	}
	tempUser := this.users[name]
	tempUser.isAdmin = true
	tempUser.isSuperAdmin = true
	tempUser.level = 9
	this.users[name] = tempUser
	log.Printf("add superAdmin :%s\n", name)
}

func (this *Mindustry) onlineUser(name string) {
	this.playCnt++

	if _, ok := this.users[name]; ok {
		return
	}
	this.addUser(name)
}
func (this *Mindustry) offlineUser(name string) {
	if this.playCnt > 0 {
		this.playCnt--
	}

	if _, ok := this.users[name]; ok {
		return
	}

	if !(this.users[name].isAdmin || this.users[name].isSuperAdmin) {
		this.delUser(name)
		return
	}
}
func (this *Mindustry) delUser(name string) {
	if _, ok := this.users[name]; !ok {
		log.Printf("del user not exist :%s\n", name)
		return
	}
	delete(this.users, name)
	log.Printf("del user info :%s\n", name)
}
func (this *Mindustry) execCmd(in io.WriteCloser, cmd string) {
	if cmd == "stop" || cmd == "host" || cmd == "hostx" || cmd == "load" {
		this.playCnt = 0
	}
	log.Printf("execCmd :%s\n", cmd)
	data := []byte(cmd + "\n")
	in.Write(data)
}
func (this *Mindustry) say(in io.WriteCloser, strKey string, v ...interface{}) {
	localeStr := "say " + this.i18n.Value(strKey) + "\n"
	info := fmt.Sprintf(localeStr, v...)
	in.Write([]byte(info))
}

func checkSlotValid(slot string) bool {
	files, _ := ioutil.ReadDir("./config/saves")
	for _, f := range files {
		if f.Name() == slot+".msav" {
			return true
		}
	}
	return false
}
func getSlotList() string {
	slotList := []string{}
	files, _ := ioutil.ReadDir("./config/saves")
	for _, f := range files {
		if strings.Count(f.Name(), "backup") > 0 {
			continue
		}
		if strings.HasSuffix(f.Name(), ".msav") {
			slotList = append(slotList, f.Name()[:len(f.Name())-len(".msav")])
		}
	}
	return strings.Join(slotList, ",")
}

func (this *Mindustry) proc_mapsOrStatus(in io.WriteCloser, userName string, userInput string, isOnlyCheck bool) bool {
	if isOnlyCheck {
		return true
	}
	temps := strings.Split(userInput, " ")
	cmdName := temps[0]

	if cmdName == "maps" || cmdName == "status" {
		go func() {
			timer := time.NewTimer(time.Duration(5) * time.Second)
			<-timer.C
			if this.currProcCmd != "" {
				this.say(in, "error.cmd_timeout", this.currProcCmd)
				this.currProcCmd = ""
			}
		}()
		this.currProcCmd = cmdName
	}
	if cmdName == "maps" {
		this.execCmd(in, "reloadmaps")
		this.maps = this.maps[0:0]
		this.execCmd(in, "maps")
	} else if cmdName == "status" {
		this.execCmd(in, "status")
	}
	return true
}
func (this *Mindustry) proc_host(in io.WriteCloser, userName string, userInput string, isOnlyCheck bool) bool {
	mapName := ""
	temps := strings.Split(userInput, " ")
	if len(temps) < 2 {
		this.say(in, "error.cmd_length_invalid", userInput)
		return false
	}
	inputCmd := strings.TrimSpace(temps[0])
	inputMap := strings.TrimSpace(temps[1])
	inputMode := ""
	if this.mode != "" {
		if len(temps) > 2 {
			this.say(in, "error.cmd_host_fix_mode", this.mode)
			return false
		}
		inputMode = inputMode
	}
	if len(temps) > 2 {
		inputMode = strings.TrimSpace(temps[2])
	}
	if inputCmd == "hostx" {
		inputIndex := 0
		var err error = nil
		if inputIndex, err = strconv.Atoi(inputMap); err != nil {
			this.say(in, "error.cmd_hostx_id_not_number", userInput)
			return false
		}
		if inputIndex < 0 || inputIndex >= len(this.maps) {

			this.say(in, "error.cmd_hostx_id_not_found", userInput)
			return false
		}
		mapName = this.maps[inputIndex]
	} else if inputCmd == "host" {
		isFind := false
		for _, name := range this.maps {
			if name == inputMap {
				isFind = true
				mapName = name
				break
			}
		}
		if !isFind {
			this.say(in, "error.cmd_host_map_not_found", userInput)
			return false
		}
	} else {
		this.say(in, "error.cmd_invalid", userInput)
		return false
	}
	if inputMode != "pvp" && inputMode != "attack" && inputMode != "" && inputMode != "sandbox" {
		this.say(in, "error.cmd_host_mode_invalid", userInput)
		return false
	}
	if isOnlyCheck {
		return true
	}
	this.say(in, "info.server_restart")
	this.execCmd(in, "reloadmaps")
	time.Sleep(time.Duration(5) * time.Second)
	this.execCmd(in, "stop")
	time.Sleep(time.Duration(5) * time.Second)
	mapName = strings.Replace(mapName, " ", "_", -1)
	if inputMode == "" {
		this.execCmd(in, "host "+mapName)
	} else {
		this.execCmd(in, "host "+mapName+" "+inputMode)
	}
	return true
}

func (this *Mindustry) proc_save(in io.WriteCloser, userName string, userInput string, isOnlyCheck bool) bool {
	targetSlot := ""
	if userInput == "save" {
		minute := time.Now().Minute()
		targetSlot = fmt.Sprintf("%d%02d%02d", time.Now().Day(), time.Now().Hour(), minute/10*10)
	} else {
		targetSlot = userInput[len("save"):]
		targetSlot = strings.TrimSpace(targetSlot)
	}
	if _, ok := strconv.Atoi(targetSlot); ok != nil {
		this.say(in, "error.cmd_save_slot_invalid", targetSlot)
		return false
	}
	if isOnlyCheck {
		return true
	}
	this.execCmd(in, "save "+targetSlot)
	this.say(in, "info.save_slot_succ", targetSlot)
	return true
}

func (this *Mindustry) proc_load(in io.WriteCloser, userName string, userInput string, isOnlyCheck bool) bool {
	targetSlot := userInput[len("load"):]
	targetSlot = strings.TrimSpace(targetSlot)
	if !checkSlotValid(targetSlot) {
		this.say(in, "error.cmd_load_slot_invalid", targetSlot)
		return false
	}
	if isOnlyCheck {
		return true
	}
	this.say(in, "info.server_restart")
	time.Sleep(time.Duration(5) * time.Second)
	this.execCmd(in, "stop")
	time.Sleep(time.Duration(5) * time.Second)
	this.execCmd(in, userInput)
	return true
}
func (this *Mindustry) proc_admin(in io.WriteCloser, userName string, userInput string, isOnlyCheck bool) bool {
	targetName := userInput[len("admin"):]
	targetName = strings.TrimSpace(targetName)
	if targetName == "" {
		this.say(in, "error.cmd_admin_name_invalid")
		return false
	} else {
		if isOnlyCheck {
			return true
		}
		this.addAdmin(targetName)
		this.execCmd(in, userInput)
		this.say(in, "info.admin_added", targetName)
	}
	return true
}
func (this *Mindustry) proc_directCmd(in io.WriteCloser, userName string, userInput string, isOnlyCheck bool) bool {
	if isOnlyCheck {
		return true
	}
	this.execCmd(in, userInput)
	return true
}
func (this *Mindustry) proc_gameover(in io.WriteCloser, userName string, userInput string, isOnlyCheck bool) bool {
	if isOnlyCheck {
		return true
	}
	this.execCmd(in, "reloadmaps")
	this.execCmd(in, userInput)
	return true
}
func (this *Mindustry) proc_help(in io.WriteCloser, userName string, userInput string, isOnlyCheck bool) bool {
	if isOnlyCheck {
		return true
	}
	temps := strings.Split(userInput, " ")
	if len(temps) >= 2 {
		cmd := strings.TrimSpace(temps[1])
		this.say(in, "helps."+cmd, cmd)
	} else {
		if this.users[userName].isSuperAdmin {
			this.say(in, "info.super_admin_cmd", this.cfgSuperAdminCmds)
		} else if this.users[userName].isAdmin {
			this.say(in, "info.admin_cmd", this.cfgAdminCmds)
		} else {
			this.say(in, "info.user_cmd", this.cfgNormCmds)
		}
		this.say(in, "info.votetick_cmd", this.cfgVoteCmds)

	}
	return true
}

var tempOsPath = "/sys/class/thermal/thermal_zone0/temp"

func getCpuTemp() float64 {
	raw, err := ioutil.ReadFile(tempOsPath)
	if err != nil {
		log.Printf("Failed to read temperature from %q: %v", tempOsPath, err)
		return 0.0
	}

	cpuTempStr := strings.TrimSpace(string(raw))
	cpuTempInt, err := strconv.Atoi(cpuTempStr) // e.g. 55306
	if err != nil {
		log.Printf("%q does not contain an integer: %v", tempOsPath, err)
		return 0.0
	}
	cpuTemp := float64(cpuTempInt) / 1000.0
	//debug.Printf("CPU temperature: %.3f°C", cpuTemp)
	return cpuTemp
}
func (this *Mindustry) proc_show(in io.WriteCloser, userName string, userInput string, isOnlyCheck bool) bool {
	if isOnlyCheck {
		return true
	}
	this.say(in, "info.ver", _VERSION_)
	this.say(in, "info.cpu_temperature", getCpuTemp())
	return true
}
func (this *Mindustry) proc_showAdmin(in io.WriteCloser, userName string, userInput string, isOnlyCheck bool) bool {
	if isOnlyCheck {
		return true
	}
	this.say(in, "info.super_admin_list", this.cfgSuperAdmin)
	this.say(in, "info.admin_list", this.cfgAdmin)
	return true

}

func (this *Mindustry) proc_slots(in io.WriteCloser, userName string, userInput string, isOnlyCheck bool) bool {
	if isOnlyCheck {
		return true
	}
	this.say(in, "info.slots_list", getSlotList())
	return true
}
func (this *Mindustry) checkVote() (bool, int, int) {
	if this.playCnt == 0 {
		log.Printf("playCnt is zero!\n")
		return false, 0, 0
	}
	agreeCnt := 0
	adminAgainstCnt := 0
	for userName, isAgree := range this.votetickUsers {
		if isAgree == 1 {
			agreeCnt++
		} else if _, ok := this.users[userName]; ok {
			if this.users[userName].isAdmin {
				adminAgainstCnt++
			}
		}
	}
	if adminAgainstCnt > 0 {
		return false, agreeCnt, adminAgainstCnt
	}

	return float32(agreeCnt)/float32(this.playCnt) >= 0.5, agreeCnt, adminAgainstCnt
}
func (this *Mindustry) proc_votetick(in io.WriteCloser, userName string, userInput string, isOnlyCheck bool) bool {
	index := strings.Index(userInput, " ")
	if index < 0 {
		this.say(in, "error.cmd_votetick_target_invalid", userInput)
		return false
	}

	if len(this.votetickUsers) > 0 {
		this.say(in, "error.cmd_votetick_in_progress")
		return false
	}
	votetickCmd := strings.TrimSpace(userInput[index:])
	votetickCmdHead := votetickCmd
	index = strings.Index(votetickCmd, " ")
	if index >= 0 {
		votetickCmdHead = strings.TrimSpace(votetickCmd[:index])
	}

	if cmd, ok := this.cmds[votetickCmdHead]; ok {
		if !cmd.isVote {
			this.say(in, "error.cmd_votetick_not_permit", votetickCmdHead)
			return false
		}
	} else {
		this.say(in, "error.cmd_votetick_cmd_error", votetickCmdHead)
		return false
	}
	if handleFunc, ok := this.userCmdProcHandles[votetickCmdHead]; ok {
		checkRslt := handleFunc(in, userName, votetickCmd, true)
		if !checkRslt {
			return false
		}

		if isOnlyCheck {
			return true
		}

		this.currProcCmd = "votetick"
		this.votetickUsers = make(map[string]int)
		this.votetickUsers[userName] = 1
		go func() {
			timer := time.NewTimer(time.Duration(60) * time.Second)
			<-timer.C
			isSucc, agreeCnt, adminAgainstCnt := this.checkVote()
			if isSucc {
				this.say(in, "info.votetick_pass", this.playCnt, agreeCnt)
				handleFunc(in, userName, votetickCmd, false)
			} else {
				this.say(in, "info.votetick_fail", this.playCnt, agreeCnt, adminAgainstCnt)
			}
			this.votetickUsers = make(map[string]int)
			this.currProcCmd = ""
		}()

	} else {
		this.say(in, "error.cmd_votetick_cmd_not_support", votetickCmd)
		return false
	}
	this.say(in, "info.votetick_begin_info")
	return true
}
func (this *Mindustry) procUsrCmd(in io.WriteCloser, userName string, userInput string) {
	temps := strings.Split(userInput, " ")
	cmdName := temps[0]

	if cmd, ok := this.cmds[cmdName]; ok {
		if this.users[userName].level < cmd.level {
			this.say(in, "error.cmd_permission_denied", userName, cmdName)
			return
		} else {
			if this.currProcCmd != "" {
				this.say(in, "error.cmd_is_exceuting", this.currProcCmd)
				return
			}

			if handleFunc, ok := this.userCmdProcHandles[cmdName]; ok {
				handleFunc(in, userName, userInput, false)
			} else {
				this.userCmdProcHandles["directCmd"](in, userName, userInput, false)
			}
		}

	} else {
		this.say(in, "error.cmd_invalid_user", userName, cmdName)
	}
}
func (this *Mindustry) multiLineRsltCmdComplete(in io.WriteCloser, line string) bool {
	index := -1
	if this.currProcCmd == "maps" {
		if strings.Index(line, "Map directory:") >= 0 {
			mapsInfo := ""
			for index, name := range this.maps {
				if mapsInfo != "maps:" {
					mapsInfo += " "
				}
				mapsInfo += ("[" + strconv.Itoa(index) + "]" + name)
			}
			this.say(in, "info.maps_list", mapsInfo)
			return true
		}
		mapNameEndIndex := -1
		index = strings.Index(line, ": Custom /")
		if index >= 0 {
			mapNameEndIndex = index
		}
		index = strings.Index(line, ": Default /")
		if index >= 0 {
			mapNameEndIndex = index
		}
		if mapNameEndIndex >= 0 {
			this.maps = append(this.maps, strings.TrimSpace(line[:mapNameEndIndex]))
		}
	} else if this.currProcCmd == "status" {

		index = strings.Index(line, "Players:")
		if index >= 0 {
			countStr := strings.TrimSpace(line[index+len("Players:")+1:])
			if count, ok := strconv.Atoi(countStr); ok == nil {
				this.playCnt = count
			}
			return true
		} else if strings.Index(line, "No players connected.") >= 0 {
			this.playCnt = 0
			return true
		} else if strings.Index(line, "Status: server closed") >= 0 {
			this.serverIsRun = false
			this.playCnt = 0

			return true
		}
	}
	return false
}

const USER_CONNECTED_KEY string = " has connected."
const USER_DISCONNECTED_KEY string = " has disconnected."
const SERVER_INFO_LOG string = "[INFO] "
const SERVER_ERR_LOG string = "[ERR!] "
const SERVER_READY_KEY string = "Server loaded. Type 'help' for help."
const SERVER_STSRT_KEY string = "Opened a server on port"

func (this *Mindustry) output(line string, in io.WriteCloser) {
	index := strings.Index(line, SERVER_ERR_LOG)
	if index >= 0 {
		errInfo := strings.TrimSpace(line[index+len(SERVER_ERR_LOG):])
		if strings.Contains(errInfo, "io.anuke.arc.util.ArcRuntimeException: File not found") {
			log.Printf("map not found , force exit!\n")
			this.execCmd(in, "exit")
		}
		this.cmdFailReason = errInfo
		return
	}

	index = strings.Index(line, SERVER_INFO_LOG)
	if index < 0 {
		return
	}
	cmdBody := strings.TrimSpace(line[index+len(SERVER_INFO_LOG):])
	if this.currProcCmd == "maps" || this.currProcCmd == "status" {
		//this.say(in, line)
		if this.multiLineRsltCmdComplete(in, cmdBody) {
			this.currProcCmd = ""
		}
		return
	}
	index = strings.Index(cmdBody, ":")
	if index > -1 {
		userName := strings.TrimSpace(cmdBody[:index])
		if _, ok := this.users[userName]; ok {
			if userName == "Server" {
				return
			}
			sayBody := strings.TrimSpace(cmdBody[index+1:])
			if strings.HasPrefix(sayBody, "\\") || strings.HasPrefix(sayBody, "/") || strings.HasPrefix(sayBody, "!") {
				this.procUsrCmd(in, userName, sayBody[1:])
			} else if len(this.votetickUsers) > 0 {
				if sayBody == "1" {
					log.Printf("%s votetick agree\n", userName)
					this.votetickUsers[userName] = 1
				} else if sayBody == "0" {
					log.Printf("%s votetick not agree\n", userName)
					this.votetickUsers[userName] = 0
				}
			} else {
				//fmt.Printf("%s : %s\n", userName, sayBody)
			}
		}
	}

	if strings.HasSuffix(cmdBody, USER_CONNECTED_KEY) {
		userName := strings.TrimSpace(cmdBody[:len(cmdBody)-len(USER_CONNECTED_KEY)])
		if userName == "Server" {
			this.say(in, "error.login_forbbidden_username")
			this.execCmd(in, "kick "+userName)
			return
		}
		this.onlineUser(userName)

		if this.users[userName].isAdmin {
			time.Sleep(1 * time.Second)
			if this.users[userName].isSuperAdmin {
				this.say(in, "info.welcom_super_admin", userName)
			} else {
				this.say(in, "info.welcom_admin", userName)
			}
			this.execCmd(in, "admin "+userName)
		}

	} else if strings.HasSuffix(cmdBody, USER_DISCONNECTED_KEY) {
		userName := strings.TrimSpace(cmdBody[:len(cmdBody)-len(USER_DISCONNECTED_KEY)])
		this.offlineUser(userName)
	} else if strings.HasPrefix(cmdBody, SERVER_READY_KEY) {
		this.playCnt = 0
		this.serverIsRun = true

		this.execCmd(in, "name "+this.name)
		this.execCmd(in, "port "+strconv.Itoa(this.port))
		this.execCmd(in, "host Fortress")
	} else if strings.HasPrefix(cmdBody, SERVER_STSRT_KEY) {
		log.Printf("server starting!\n")
		this.serverIsRun = true
		this.playCnt = 0
	}
}
func (this *Mindustry) run() {
	var para = []string{"-jar", this.jarPath}
	for {
		this.execCommand("java", para)
		if this.serverIsStart {
			log.Printf("server crash,wait(10s) reboot!\n")
			time.Sleep(time.Duration(10) * time.Second)
		} else {
			break
		}
	}
}
func startMapUpServer(port int) {
	go func(serverPort int) {
		StartFileUpServer(serverPort)
	}(port)
}
func main() {
	mode := flag.String("mode", "", "fix mode:survival,attack,sandbox,pvp")
	port := flag.Int("port", 6567, "Input port")
	map_port := flag.Int("up", 6569, "map up port")
	flag.Parse()
	log.Printf("version:%s!\n", _VERSION_)

	startMapUpServer(*map_port)
	mindustry := Mindustry{}
	mindustry.init()
	mindustry.mode = *mode
	mindustry.port = *port
	mindustry.run()
}
