package main

import (
	"fmt"
	"io/ioutil"
	"net/http"
	_ "net/http/pprof"
	"net/url"
	"os"
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

func GetServerInfo(Data *ServerInfo, bLocal bool) {
	URL := ""
	if bLocal {
		URL = "http://192.168.1.158/game_server_info/index.php"
	} else {
		URL = "http://gameserver.com:10023/game_server_info/index.php"
	}
	resp, err := http.PostForm(URL,
		url.Values{"id": {"5"}, "account": {"123"}})
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

func startRobot(cnt uint32, bVip bool, bLocal bool) {
	var skData ServerInfo
	GetServerInfo(&skData, bLocal)
	if len(skData.Result) > 0 && len(skData.Result[0].GameInfo) > 0 {
		ai.StartRobot(skData.Result[0].GameInfo, "testrobot", "123", cnt, bVip)
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

	num := 50
	bVip := true
	bLocal := true

	aLen := len(os.Args)
	if aLen > 1 {
		num, _ = strconv.Atoi(os.Args[1])
		if aLen > 2 {
			nn, _ := strconv.Atoi(os.Args[2])
			bVip = nn > 0
		}
		if aLen > 3 {
			nn, _ := strconv.Atoi(os.Args[3])
			bLocal = nn > 0
		}
	}

	cpuNum := runtime.NumCPU()
	runtime.GOMAXPROCS(cpuNum)
	startRobot(uint32(num), bVip, bLocal)
}
