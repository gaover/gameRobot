package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	_ "net/http/pprof"
	"net/url"
	ai "robot/ai"
	"runtime"
	"strconv"
)

func init() {

}

type ServerList struct {
	ServerName   string      `json:"serverName"`
	ServerId     int32       `json:"serverId"`
	Onlineusers  int32       `json:"onlineusers"`
	ServerStatus string      `json:"serverStatus"`
	GameInfo     []ai.IPInfo `json:"GameInfo"`
	Account      interface{} `json:"accountinfo"`
}

type ServerInfo struct {
	Status int32        `json:"status"`
	Result []ServerList `json:"result"`
}

func GetServerInfo(Data *ServerInfo, platId, serverId uint32, host string) {
	URL := "http://" + host + "/game_server_info/index.php"
	resp, err := http.PostForm(URL,
		url.Values{"id": {strconv.Itoa(int(platId))}, "serverid": {strconv.Itoa(int(serverId))}, "account": {"123"}})
	if err != nil {
		fmt.Println(err)
	}

	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		// handle error
	}

	fmt.Println(string(body))
	if ai.JsonStrToObject(body, Data) {
	}
	fmt.Println(*Data)
}

func startRobot(cnt uint32, bVip bool, platId, serverId uint32, host string) {
	var skData ServerInfo
	GetServerInfo(&skData, platId, serverId, host)
	if len(skData.Result) > 0 && len(skData.Result[0].GameInfo) > 0 {
		ai.StartRobot(skData.Result[0].GameInfo, "TESTROBOT", "123", cnt, bVip)
	} else {
		fmt.Printf("server is empty")
	}
}

func web() {
	go func() {
		http.ListenAndServe("0.0.0.0:8080", nil)
	}()
}

func main() {
	// web()

	num := flag.Int("n", 50, "test robot num")
	bVip := flag.Int("vip", 1, "is robot vip?")
	platId := flag.Int("plat", 5, "platform id")
	serverId := flag.Int("sid", 9, "server id")
	host := flag.String("host", "127.0.0.1:8080", "web host")
	flag.Parse()

	cpuNum := runtime.NumCPU()
	runtime.GOMAXPROCS(cpuNum)
	startRobot(uint32(*num), *bVip > 0, uint32(*platId), uint32(*serverId), *host)
}
