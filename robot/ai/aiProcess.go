package ai

import (
	"errors"
	"fmt"
	"github.com/golang/protobuf/proto"
	rand "math/rand"
	"net"
	pbd "robot/ai/pbd"
	"sync"
	"time"
	"unsafe"
)

type IPInfo struct {
	IP   string `json:"host"`
	Port string `json:"port"`
}

var (
	g_Handle  *ActionHandle
	allRobots []*AIRobot
	wg        sync.WaitGroup
)

func init() {
	g_Handle = new(ActionHandle)
	g_Handle.handles = make(map[uint32]HandlersFunc)
	g_Handle.hasFather = make(map[uint32]bool)
	Config("./", "robot", 0)
	rand.Seed(time.Now().UnixNano())
}

func Random(ips []IPInfo) *IPInfo {
	ll := len(ips)
	if ll > 0 {
		if ll == 1 {
			return &ips[0]
		} else {
			index := rand.Int() % ll
			return &ips[index]
		}
	}
	return nil
}

func StartRobot(ips []IPInfo, acc string, password string, cnt uint32, bVip bool) {
	var i uint32
	wg.Add(int(cnt))
	allRobots = make([]*AIRobot, 0, cnt)
	for i = 0; i != cnt; i++ {
		robot := new(AIRobot)
		ipData := Random(ips)
		robot.ip = ipData.IP
		robot.port = ipData.Port
		robot.bVip = bVip
		if bVip {
			robot.account = fmt.Sprintf("%s_%d", acc, i)
		} else {
			robot.account = fmt.Sprintf("%s_m_%d", acc, i)
		}
		robot.passwd = password
		robot.id = i
		allRobots = append(allRobots, robot)
		go robot.Tick()

	}

	for {
		time.Sleep(time.Duration(1) * time.Second)
		var id uint32
		fmt.Printf("would you want to exit?")
		fmt.Scanln(&id)
		if id != 0 {
			break
		} else {
			continue
		}
	}
	DLog("all robot begin exit")

	for i = 0; i != cnt; i++ {
		allRobots[i].Stop()
	}

	wg.Wait()
}

func (robot *AIRobot) TimeOut() {

	for {
		<-robot.beginTimeOut
		num := rand.Int() % 7
		time.Sleep(time.Duration(num*1000) * time.Millisecond)
		robot.timeout <- true

		time.Sleep(time.Duration(1) * time.Millisecond)

		if !robot.IsRun() {
			break
		}
	}
}

// 心跳
func (robot *AIRobot) Tick() {
	robot.Start()
	robot.MsgChan = make(chan *MsgInfo, 100)
	robot.timeout = make(chan bool, 1)
	robot.beginTimeOut = make(chan bool, 1)
	go robot.TimeOut()

	robot.bConnect = false
	for {
		time.Sleep(time.Duration(1) * time.Millisecond)

		if !robot.IsRun() {
			robot.CloseNet()
			close(robot.MsgChan)
			close(robot.timeout)
			close(robot.beginTimeOut)
			DLog("robot %d stop. and connect to server 2s later ", robot.id)
			DLog("robot %d restart.", robot.id)
			time.Sleep(time.Duration(8000) * time.Millisecond)
			robot.Start()
			robot.MsgChan = make(chan *MsgInfo, 100)
			robot.timeout = make(chan bool, 1)
			robot.beginTimeOut = make(chan bool, 1)
			go robot.TimeOut()
			continue
		}

		if !robot.bConnect {
			robot.Connect()
			robot.C2S_Login()
			robot.bConnect = true
			go robot.RecvMsg()
			continue
		}

		bSkip := false
		robot.beginTimeOut <- true

		for {

			select {
			case msg := <-robot.MsgChan:
				{
					if h, ok := g_Handle.handles[msg.index]; ok {
						h(robot, msg.msg)
					}
				}
			case <-robot.timeout:
				{
					bSkip = true
				}
			}

			if bSkip {
				break
			}
		}

		robot.heartCnt++
		if robot.heartCnt > 2000 {
			robot.C2S_HeartBeat()
			robot.heartCnt = 0
		}

		if robot.ChangeScene {

			if robot.CheckChat() {
				robot.C2S_Chat()
				robot.SetChatTime()
			}

			if robot.moveCnt > 15 {
				robot.moveCnt = 0
				robot.C2S_FastTransfer()
			} else if robot.CheckMoveTime() {
				robot.C2S_Request_Move()
			}
		}
	}
	DLog("robot %d exit ", robot.id)
	wg.Done()
}

func (robot *AIRobot) CheckChat() bool {
	tt := GetUnixTime()
	return tt > robot.chatTime
}

func (robot *AIRobot) SetChatTime() {
	tt := GetUnixTime()
	robot.chatTime = tt + int64(rand.Int()%40) + 3
}

func (robot *AIRobot) CheckMoveTime() bool {
	tt := GetUnixTime()
	return tt > robot.moveTime
}

func (robot *AIRobot) SetMoveTime() {
	tt := GetUnixTime()
	robot.moveTime = tt + int64(rand.Int()%5)
}

func (robot *AIRobot) CloseNet() {
	robot.Conn.Close()
	DLog("robot %d disconnect net", robot.id)
	robot.bConnect = false
}

func (robot *AIRobot) Connect() {
	var tcpAddr *net.TCPAddr
	tcpAddr, _ = net.ResolveTCPAddr("tcp", robot.ip+":"+robot.port)

	var err error
	robot.Conn, err = net.DialTCP("tcp", nil, tcpAddr)

	if err != nil {
		DLog("Client connect error ! " + err.Error())
		robot.Stop()
	}

	var skTime time.Time
	robot.Conn.SetWriteDeadline(skTime)
	robot.Conn.SetReadDeadline(skTime)
	DLog("%s %d Client connected!", robot.Conn.LocalAddr().String(), robot.id)
}

func (robot *AIRobot) RecvMsg() {
	for {
		pkMsg := robot.RecvData()
		if pkMsg != nil {
			robot.MsgChan <- pkMsg
		} else if !robot.IsRun() {
			break
		}
		time.Sleep(time.Duration(1) * time.Millisecond)
	}
}

func (robot *AIRobot) RecvN(n int32, buff []byte) (int32, error) {
	var num int32 = 0

	for {
		len, err := robot.Conn.Read(buff[num:n])
		if err != nil {
			return 0, err
		}

		num += int32(len)

		if num > n {
			DLog("%s wtf recvxxx", robot.account)
			return 0, errors.New("wtf num > n")
		}

		if num == n {
			return n, nil
		}
		time.Sleep(time.Duration(1) * time.Millisecond)
	}
}

func (robot *AIRobot) RecvData() *MsgInfo {
	head := make([]byte, 20, 20)
	n, err := robot.RecvN(20, head)
	if err != nil {
		robot.StopMsg("RecvData", err)
		return nil
	}

	if n != 20 {
		DLog("%d invalid recv1  len %d:20", robot.id, n)
		if err != nil {
			robot.StopMsg("recv111", err)
		} else {
			robot.StopMsg("recv111", nil)
		}
		return nil
	}

	pkHead := *(**NetBuff)(unsafe.Pointer(&head))

	if g_Handle.GetAction(pkHead.index) == nil {
		robot.Stop()
		DLog("%d invalid action %d and len %d", robot.id, pkHead.index, pkHead.len)
		return nil
	}

	if pkHead.len > 50*1024 {
		robot.Stop()
		DLog("%d invalid index %d and len %d", robot.id, pkHead.index, pkHead.len)
		return nil
	}

	if !g_Handle.IsFather(pkHead.index) {
		buff := make([]byte, pkHead.len, pkHead.len)
		n, err = robot.RecvN(int32(pkHead.len), buff)

		if err != nil {
			robot.StopMsg("RecvN has err", err)
			return nil
		}

		if n != int32(pkHead.len) {
			DLog("%d invalid recv2  len %d:%d", robot.id, n, pkHead.len)
			if err != nil {
				robot.StopMsg("recv2", err)
			} else {
				robot.StopMsg("recv2", nil)
			}
			return nil
		}

		return nil
	}

	pkMsg := new(MsgInfo)
	pkMsg.index = pkHead.index
	pkMsg.msg = make([]byte, pkHead.len, pkHead.len)

	n, err = robot.RecvN(int32(pkHead.len), pkMsg.msg)

	if err != nil {
		robot.StopMsg("recv body", err)
		return nil
	}

	if n != int32(pkHead.len) {
		DLog("%d invalid recv3  len %d %d", robot.id, n, pkHead.len)
		if err != nil {
			robot.StopMsg("recv3", err)
		} else {
			robot.StopMsg("recv3", nil)
		}
		return nil
	}

	return pkMsg
}

func (robot *AIRobot) WriteN(n int32, msg []byte) (int32, error) {
	var num int32 = 0
	for {
		len, err := robot.Conn.Write(msg[num:n])

		if err != nil {
			return 0, err
		}

		num += int32(len)

		if num > n {
			return 0, errors.New("why write len > n")
		}

		if num == n {
			return n, nil
		}

		time.Sleep(time.Duration(1) * time.Millisecond)
	}
}

func (robot *AIRobot) SendC2SData(id uint32, data []byte) {

	if id != 1586918846 && len(data) == 0 {
		DLog("%d data is empty", id)
		robot.Stop()
		return
	}

	msgLength := 20 + len(data)
	msg := make([]byte, 20, msgLength)
	pkHead := *(**NetBuff)(unsafe.Pointer(&msg))
	pkHead.flag = 0x11111111
	pkHead.index = id
	pkHead.len = (uint32)(len(data))

	msg = append(msg, data...)

	n, err := robot.WriteN(int32(pkHead.len+20), msg)
	if err != nil {
		DLog("write %d fail %d %s", id, n, err.Error())
		robot.Stop()
	}

}

func (robot *AIRobot) Stop() {
	DLog("robot %d MMMMMM stop", robot.id)
	robot.bRun = false
}

func (robot *AIRobot) Start() {
	robot.bRun = true
}

func (robot *AIRobot) IsRun() bool {
	return robot.bRun
}

func (robot *AIRobot) StopMsg(name string, err error) {
	robot.Stop()
	if err != nil {
		DLog("robot %d HHHHHHH %s:%s", robot.id, name, err.Error())
	} else {
		DLog("robot %d HHHHHHH %s", robot.id, name)
	}
}

func (robot *AIRobot) C2S_SendDanmaku() {
}

func (robot *AIRobot) C2S_HangUpBegin() {
}

func (robot *AIRobot) C2S_HangUpEnd() {
}

func (robot *AIRobot) C2S_HangUpInfo() {
}

func (robot *AIRobot) C2S_OtherHomeInfo() {
}

func (robot *AIRobot) C2S_RoleHomeInfo() {
}

func (robot *AIRobot) C2S_BuyHomeShopGood() {
}

func (robot *AIRobot) C2S_HomeExtendShopInfo() {
}

func (robot *AIRobot) C2S_ExtendRoomSize() {
}

func (robot *AIRobot) C2S_HomeDepotSell() {
}

func (robot *AIRobot) C2S_ExtendDepotSize() {
}

func (robot *AIRobot) C2S_UpdateRoomInfo() {
}

func (robot *AIRobot) C2S_MG_CreateRoom() {
}

func (robot *AIRobot) C2S_MG_JoinRoom() {
}

func (robot *AIRobot) C2S_MG_FastJoinRoom() {
}

func (robot *AIRobot) C2S_MG_LeaveRoom() {
}

func (robot *AIRobot) C2S_MG_Ready() {
}

func (robot *AIRobot) C2S_MG_MasterKickPlayer() {
}

func (robot *AIRobot) C2S_MG_ThrowJBEgg() {
}

func (robot *AIRobot) C2S_HeartBeat() {
	var skData pbd.C2S_HeartBeat
	data, _ := proto.Marshal(&skData)
	robot.SendC2SData(GetMD5Name("C2S_HeartBeat"), data)
}

func (robot *AIRobot) C2S_GetServerId() {
}

func (robot *AIRobot) C2S_CheckAccount() {
}

func (robot *AIRobot) C2S_Login() {
	var skData pbd.C2S_Login
	skData.Account = append(skData.Account, robot.account...)
	skData.Password = append(skData.Password, robot.passwd...)
	data, _ := proto.Marshal(&skData)
	robot.SendC2SData(GetMD5Name("C2S_Login"), data)
}

func (robot *AIRobot) C2S_Register() {
}

func (robot *AIRobot) C2S_RoleSum() {
	var skData pbd.C2S_RoleSum
	skData.Account = append(skData.Account, robot.account...)
	data, _ := proto.Marshal(&skData)
	robot.SendC2SData(GetMD5Name("C2S_RoleSum"), data)
	// robot.RoleSum++
	// if robot.RoleSum > 2 {
	// 	DLog("RoleSum fail 2")
	// 	robot.Stop()
	// }
}

func (robot *AIRobot) C2S_ClearRole() {
}

func (robot *AIRobot) C2S_RandNickName() {
}

func (robot *AIRobot) C2S_CreateRole() {
	var skData pbd.C2S_CreateRole

	hatList := []int32{41101, 41201, 41131, 41231}
	faceList := []int32{80081, 80082, 80083, 80084, 80085, 80086}
	weaponList := []int32{40101, 40131}
	wingList := []int32{44101, 44104, 44105, 44106}

	skData.NickName = append(skData.NickName, robot.account...)
	skData.Account = append(skData.Account, robot.account...)
	skData.DeviceKey = append(skData.DeviceKey, "robot.devicekey"...)
	skData.DeviceToken = append(skData.DeviceToken, "robot.token"...)
	skData.DeviceType = append(skData.DeviceToken, "robot.type"...)
	skData.PlatformType = append(skData.PlatformType, "1"...)
	skData.HelmetId = new(int32)
	*skData.HelmetId = hatList[rand.Int()%len(hatList)]
	skData.FaceId = new(int32)
	*skData.FaceId = faceList[rand.Int()%len(faceList)]
	skData.WeaponId = new(int32)
	*skData.WeaponId = weaponList[rand.Int()%len(weaponList)]
	skData.OrnamentsId = new(int32)
	*skData.OrnamentsId = wingList[rand.Int()%len(wingList)]
	skData.EggId = new(int32)
	*skData.EggId = 0
	skData.Gender = new(int32)
	*skData.Gender = 0
	skData.Channel = append(skData.Channel, "RO"...)
	skData.IsTestVip = new(bool)
	*skData.IsTestVip = robot.bVip
	data, _ := proto.Marshal(&skData)
	robot.SendC2SData(GetMD5Name("C2S_CreateRole"), data)
}

func (robot *AIRobot) C2S_RoleInfo() {
	var skData pbd.C2S_RoleInfo
	skData.Account = append(skData.Account, robot.account...)
	skData.RoleIndex = new(int64)
	*skData.RoleIndex = robot.roleIndex
	skData.ForceApplyRoleInfo = new(bool)
	*skData.ForceApplyRoleInfo = true
	data, _ := proto.Marshal(&skData)
	robot.SendC2SData(GetMD5Name("C2S_RoleInfo"), data)
}

func (robot *AIRobot) C2S_ReadyEnterScene() {
	var skData pbd.C2S_ReadyEnterScene
	skData.RoleIndex = new(int64)
	*skData.RoleIndex = robot.roleIndex
	data, _ := proto.Marshal(&skData)
	robot.SendC2SData(GetMD5Name("C2S_ReadyEnterScene"), data)
}

func (robot *AIRobot) C2S_EnterScene() {
	var skData pbd.C2S_EnterScene
	skData.RoleIndex = new(int64)
	*skData.RoleIndex = robot.roleIndex
	skData.SceneInstanceId = new(int64)
	*skData.SceneInstanceId = 0
	data, _ := proto.Marshal(&skData)
	robot.SendC2SData(GetMD5Name("C2S_EnterScene"), data)
}

func (robot *AIRobot) C2S_BornEnterSceneOK() {
	var skData pbd.C2S_BornEnterSceneOK
	skData.RoleIndex = new(int64)
	*skData.RoleIndex = robot.roleIndex
	data, _ := proto.Marshal(&skData)
	robot.SendC2SData(GetMD5Name("C2S_BornEnterSceneOK"), data)

}

func (robot *AIRobot) C2S_Charset() {
}

func (robot *AIRobot) C2S_Heartbeat() {
}

func (robot *AIRobot) C2S_GameCoin() {
}

func (robot *AIRobot) C2S_DiamondCoin() {
}

func (robot *AIRobot) C2S_Endurance() {
}

func (robot *AIRobot) C2S_Exp() {
}

func (robot *AIRobot) C2S_Level() {
}

func (robot *AIRobot) C2S_SkillPoint() {
}

func (robot *AIRobot) C2S_Request_Move() {

	var skData pbd.C2S_Request_Move
	skData.RoleIndex = new(int64)
	*skData.RoleIndex = robot.roleIndex
	skData.SceneInstanceId = new(int64)
	*skData.SceneInstanceId = robot.roleIndex
	skData.BeginX = new(int32)
	*skData.BeginX = robot.x
	skData.BeginY = new(int32)
	*skData.BeginY = robot.y
	skData.EndX = new(int32)
	valx := int32(rand.Int() % 10)
	valy := int32(rand.Int() % 10)
	if rand.Int()&1 == 1 {
		valx = -valx
	}
	if rand.Int()&1 == 1 {
		valy = -valy
	}
	*skData.EndX = robot.x + valx
	skData.EndY = new(int32)
	*skData.EndY = robot.y + valy

	data, _ := proto.Marshal(&skData)
	robot.SendC2SData(GetMD5Name("C2S_Request_Move"), data)
	robot.SetMoveTime()
}

func (robot *AIRobot) C2S_Chat() {

	var skData pbd.C2S_Chat
	skData.RoleIndex = new(int64)
	*skData.RoleIndex = robot.roleIndex
	skData.SceneInstanceId = new(int64)
	*skData.SceneInstanceId = robot.roleIndex
	skData.ChatType = new(pbd.E_CHAT_TYPE)
	*skData.ChatType = pbd.E_CHAT_TYPE_ECT_WORLD
	robot.chatCnt++
	msg := fmt.Sprintf("hello c world.%d", robot.chatCnt)
	skData.Msg = append(skData.Msg, msg...)

	data, _ := proto.Marshal(&skData)
	robot.SendC2SData(GetMD5Name("C2S_Chat"), data)
}

func (robot *AIRobot) C2S_Use_SPItem() {
}

func (robot *AIRobot) C2S_Change_Scene() {
}

func (robot *AIRobot) C2S_QuerySceneLineInfo() {
}

func (robot *AIRobot) C2S_ChangeSceneLine() {
}

func (robot *AIRobot) C2S_TransPortChangeScene() {
}

func (robot *AIRobot) C2S_NPC_ChangeScene() {
}

func (robot *AIRobot) C2S_Bag_Item() {
}

func (robot *AIRobot) C2S_Use_Item() {
}

func (robot *AIRobot) C2S_Sell_Item() {
}

func (robot *AIRobot) C2S_Sell_Equip() {
}

func (robot *AIRobot) C2S_Bag_Equip() {
}

func (robot *AIRobot) C2S_Dress_Equip() {
}

func (robot *AIRobot) C2S_Role_Equip() {
}

func (robot *AIRobot) C2S_TakeOff_Equip() {
}

func (robot *AIRobot) C2S_SuitShopInfo() {
}

func (robot *AIRobot) C2S_BuySuitInShop() {
}

func (robot *AIRobot) C2S_RoleSuitInfo() {
}

func (robot *AIRobot) C2S_DressSuit() {
}

func (robot *AIRobot) C2S_TakeOffSuit() {
}

func (robot *AIRobot) C2S_FaceShopInfo() {
}

func (robot *AIRobot) C2S_BuyFaceInShop() {
}

func (robot *AIRobot) C2S_RoleFaceInfo() {
}

func (robot *AIRobot) C2S_DressFace() {
}

func (robot *AIRobot) C2S_TakeOffFace() {
}

func (robot *AIRobot) C2S_ManualUpdateFaceShop() {
}

func (robot *AIRobot) C2S_PassRisk() {
}

func (robot *AIRobot) C2S_OpenChapterBox() {
}

func (robot *AIRobot) C2S_OpenRiskBox() {
}

func (robot *AIRobot) C2S_ResetRiskFightCount() {
}

func (robot *AIRobot) C2S_ReadyEnterRisk() {
}

func (robot *AIRobot) C2S_Select_Recommend_Friends() {
}

func (robot *AIRobot) C2S_FindFriend() {
}

func (robot *AIRobot) C2S_Add_Friend() {
}

func (robot *AIRobot) C2S_Remove_Friend() {
}

func (robot *AIRobot) C2S_Request_Add_Friend() {
}

func (robot *AIRobot) C2S_ReadyEnterFriendFightScene() {
}

func (robot *AIRobot) C2S_Friend_Fight() {
}

func (robot *AIRobot) C2S_CommonShopGoodsInfo() {
}

func (robot *AIRobot) C2S_ParkShop() {
}

func (robot *AIRobot) C2S_BuyCommonShopGoods() {
}

func (robot *AIRobot) C2S_Skill_Study() {
}

func (robot *AIRobot) C2S_Skill_LevelUp() {
}

func (robot *AIRobot) C2S_Skill_DressTalent() {
}

func (robot *AIRobot) C2S_Skill_TakeOffTalent() {
}

func (robot *AIRobot) C2S_Skill_DressBook() {
}

func (robot *AIRobot) C2S_Skill_TakeOffBook() {
}

func (robot *AIRobot) C2S_Skill_ChangeBook() {
}

func (robot *AIRobot) C2S_PassMiniGame() {
}

func (robot *AIRobot) C2S_QueryMiniGameRank() {
}

func (robot *AIRobot) C2S_QuerySelfGameRank() {
}

func (robot *AIRobot) C2S_PlayMiniGameAgain() {
}

func (robot *AIRobot) C2S_SubmitMiniGameScore() {
}

func (robot *AIRobot) C2S_ReadyEnterMiniGame() {
}

func (robot *AIRobot) C2S_InteractAct() {
}

func (robot *AIRobot) C2S_SelfHiAct() {
}

func (robot *AIRobot) C2S_BathroomInteract() {
}

func (robot *AIRobot) C2S_Dress_Card_PVE() {
}

func (robot *AIRobot) C2S_TakeOff_Card_PVE() {
}

func (robot *AIRobot) C2S_Dress_Card_PVP() {
}

func (robot *AIRobot) C2S_TakeOff_Card_PVP() {
}

func (robot *AIRobot) C2S_ArenaPlayers() {
}

func (robot *AIRobot) C2S_StopArena() {
}

func (robot *AIRobot) C2S_Arena_NearSelf() {
}

func (robot *AIRobot) C2S_ReadyEnterArenaScene() {
}

func (robot *AIRobot) C2S_RefreshArena() {
}

func (robot *AIRobot) C2S_ArenaInfo() {
}

func (robot *AIRobot) C2S_ArenaFight() {
}

func (robot *AIRobot) C2S_ArenaFightEnd() {
}

func (robot *AIRobot) C2S_FriendFightEnd() {
}

func (robot *AIRobot) C2S_KillListInfo() {
}

func (robot *AIRobot) C2S_VisitingCard() {
}

func (robot *AIRobot) C2S_BuyArenaCount() {
}

func (robot *AIRobot) C2S_ClearArenaCD() {
}

func (robot *AIRobot) C2S_OpenStar() {
}

func (robot *AIRobot) C2S_QueryRisenStar() {
}

func (robot *AIRobot) C2S_QueryLevelRankListInfo() {
}

func (robot *AIRobot) C2S_LevelRankLocationSelf() {
}

func (robot *AIRobot) C2S_QueryArenaStar() {
}

func (robot *AIRobot) C2S_QueryArenaRankListInfo() {
}

func (robot *AIRobot) C2S_ArenaRankLocationSelf() {
}

func (robot *AIRobot) C2S_QueryDiamondRankInfo() {
}

func (robot *AIRobot) C2S_DiamondRankLocationInfo() {
}

func (robot *AIRobot) C2S_QueryKillRankInfo() {
}

func (robot *AIRobot) C2S_KillRankLocationInfo() {
}

func (robot *AIRobot) C2S_SwordInfo() {
}

func (robot *AIRobot) C2S_AddSwordCount() {
}

func (robot *AIRobot) C2S_SwordEnd() {
}

func (robot *AIRobot) C2S_SelectSwordEquip() {
}

func (robot *AIRobot) C2S_SelectMountainGodEquip() {
}

func (robot *AIRobot) C2S_DailyTask() {
}

func (robot *AIRobot) C2S_DailyTaskReward() {
}

func (robot *AIRobot) C2S_DailyActivity() {
}

func (robot *AIRobot) C2S_DailyActivityReward() {
}

func (robot *AIRobot) C2S_BuyStamina() {
}

func (robot *AIRobot) C2S_Mail_Title() {
}

func (robot *AIRobot) C2S_Mail_Full() {
}

func (robot *AIRobot) C2S_Mail_Read() {
}

func (robot *AIRobot) C2S_TodaySign() {
}

func (robot *AIRobot) C2S_SignInfo() {
}

func (robot *AIRobot) C2S_SignAction() {
}

func (robot *AIRobot) C2S_SignLottery() {
}

func (robot *AIRobot) C2S_SignLotteryShow() {
}

func (robot *AIRobot) C2S_SevenDayInfo() {
}

func (robot *AIRobot) C2S_GetSevenDayReward() {
}

func (robot *AIRobot) C2S_GMSetLevel() {
}

func (robot *AIRobot) C2S_GMAddItem() {
}

func (robot *AIRobot) C2S_GMAddCoin() {
}

func (robot *AIRobot) C2S_GMAddStimina() {
}

func (robot *AIRobot) C2S_GMAddDiamond() {
}

func (robot *AIRobot) C2S_GMAddCaliburnCount() {
}

func (robot *AIRobot) C2S_GMAddArenaCount() {
}

func (robot *AIRobot) C2S_GMAddFace() {
}

func (robot *AIRobot) C2S_GMAddSuit() {
}

func (robot *AIRobot) C2S_GMAddEquip() {
}

func (robot *AIRobot) C2S_FastTransfer() {
	sceneIDs := []int32{100, 101, 102, 105, 108}
	var sceneID int32
	for {
		sceneID = sceneIDs[rand.Int()%len(sceneIDs)]
		if sceneID != robot.mapID {
			break
		}
	}
	DLog("C2S_FastTransfer %d to %d", robot.mapID, sceneID)
	var skData pbd.C2S_FastTransfer
	skData.RoleIndex = new(int64)
	*skData.RoleIndex = robot.roleIndex
	skData.SceneId = new(int32)
	*skData.SceneId = sceneID

	data, _ := proto.Marshal(&skData)
	robot.SendC2SData(GetMD5Name("C2S_FastTransfer"), data)
	robot.ChangeScene = false
}

func (robot *AIRobot) C2S_EasterEggTransfer() {
}

func (robot *AIRobot) C2S_EquipStrongthen() {
}

func (robot *AIRobot) C2S_EquipStrongthenEquiped() {
}

func (robot *AIRobot) C2S_EquipResolve() {
}

func (robot *AIRobot) C2S_EquipFix() {
}

func (robot *AIRobot) C2S_EquipFixEquiped() {
}

func (robot *AIRobot) C2S_AskFixEquipsInfo() {
}

func (robot *AIRobot) C2S_EquipFastFix() {
}

func (robot *AIRobot) C2S_GetFixedEquip() {
}

func (robot *AIRobot) C2S_PointGoldInfo() {
}

func (robot *AIRobot) C2S_PointGold() {
}

func (robot *AIRobot) C2S_PersonEasterEggInfo() {
}

func (robot *AIRobot) C2S_TouchPersonEasterEgg() {
}

func (robot *AIRobot) C2S_TouchGlobalEasterEgg() {
}

func (robot *AIRobot) C2S_RechargeMoneyInfo() {
}

func (robot *AIRobot) C2S_RechargeMoney() {
}

func (robot *AIRobot) C2S_GetVipDailyReward() {
}

func (robot *AIRobot) C2S_GetSpecialVipReward() {
}

func (robot *AIRobot) C2S_TouchManInBlack() {
}

func (robot *AIRobot) C2S_PickUpInterActTool() {
}

func (robot *AIRobot) C2S_BathStatusChange() {
}

func (robot *AIRobot) C2S_PreferFace() {
}

func (robot *AIRobot) C2S_FacePreferInfo() {
}

func (robot *AIRobot) C2S_OnOffFaceRandom() {
}

func (robot *AIRobot) C2S_InterActInfo() {
}

func (robot *AIRobot) C2S_QueryUnlockInfo() {
}

func (robot *AIRobot) C2S_UnlockInteract() {
}

func (robot *AIRobot) C2S_DressInteract() {
}

func (robot *AIRobot) C2S_TakeOffInteract() {
}

func (robot *AIRobot) C2S_ExChangeInteractHole() {
}

func (robot *AIRobot) C2S_InterActManualRewardInfo() {
}

func (robot *AIRobot) C2S_GetInterActManualReward() {
}

func (robot *AIRobot) C2S_VehicleCompose() {
}

func (robot *AIRobot) C2S_EquipManualInfo() {
}

func (robot *AIRobot) C2S_GetEquipManualReward() {
}

func (robot *AIRobot) C2S_NPCInteract() {
}

func (robot *AIRobot) C2S_InteractRedPoint() {
}

func (robot *AIRobot) C2S_FriendFightInfo() {
}

func (robot *AIRobot) C2S_ExchargeSkillBook() {
}

func (robot *AIRobot) C2S_UploadSystemConfig() {
}

func (robot *AIRobot) C2S_TouchEquip() {
}

func (robot *AIRobot) C2S_TouchItem() {
}

func (robot *AIRobot) C2S_UploadForceGuideInfo() {
	var skData pbd.C2S_UploadForceGuideInfo
	skData.RoleIndex = new(int64)
	*skData.RoleIndex = robot.roleIndex
	skData.GuideIndex = new(int32)
	*skData.GuideIndex = -1
	data, _ := proto.Marshal(&skData)
	robot.SendC2SData(GetMD5Name("C2S_UploadForceGuideInfo"), data)
}

func (robot *AIRobot) C2S_ExtendSkillHole() {
}

func (robot *AIRobot) C2S_CustomFaceInfo() {
}

func (robot *AIRobot) C2S_CanUploadImage() {
}

func (robot *AIRobot) C2S_UploadImageInfo() {
}

func (robot *AIRobot) C2S_DressCustomFace() {
}

func (robot *AIRobot) C2S_TakeOffCustomFace() {
}

func (robot *AIRobot) C2S_DelelteCustomFace() {
}

func (robot *AIRobot) C2S_RolePetInfo() {
}

func (robot *AIRobot) C2S_SetPetFightStatus() {
}

func (robot *AIRobot) C2S_PetEvolve() {
}

func (robot *AIRobot) C2S_PetTalentRebuild() {
}

func (robot *AIRobot) C2S_FosterPet() {
}

func (robot *AIRobot) C2S_PetEatPill() {
}

func (robot *AIRobot) C2S_RoleNabPet() {
}

func (robot *AIRobot) C2S_AssistantTipInfo() {
}

func (robot *AIRobot) C2S_AssistantTipRead() {
}

func (robot *AIRobot) C2S_NiuDanInfo() {
}

func (robot *AIRobot) C2S_TouchNiuDan() {
}

func (robot *AIRobot) C2S_ExchangeCode() {
}

func (robot *AIRobot) C2S_GetArchieveReward() {
}

func (robot *AIRobot) C2S_ArchieveInfo() {
}

func (robot *AIRobot) C2S_ReadyEnterGuadratic() {
}

func (robot *AIRobot) C2S_SellBagThing() {
}

func (robot *AIRobot) C2S_StoneExchange() {
}

func (robot *AIRobot) C2S_FirstMonery() {
}

func (robot *AIRobot) C2S_GetFirstMonery() {
}

func (robot *AIRobot) C2S_GetOtherRechargeReward() {
}

func (robot *AIRobot) C2S_ShareToWeixin() {
}

func (robot *AIRobot) C2S_IfShareRisk() {
}

func (robot *AIRobot) C2S_ExtendEquipBag() {
}

func (robot *AIRobot) C2S_ActivityInfo() {
}

func (robot *AIRobot) C2S_GetActivityReward() {
}

func (robot *AIRobot) C2S_PianoStart() {
}

func (robot *AIRobot) C2S_PianoPlay() {
}

func (robot *AIRobot) C2S_PianoEnd() {
}

func (robot *AIRobot) C2S_AskPianoStatus() {
}

func (robot *AIRobot) C2S_EquipMake() {
}

func (robot *AIRobot) C2S_GetMinigameBuyInfo() {
}

func (robot *AIRobot) C2S_GetDailyBitCoin() {
}

func (robot *AIRobot) C2S_BuyBitCoin() {
}

func (robot *AIRobot) C2S_ReadyEnterParkScene() {
}

func (robot *AIRobot) C2S_ParkCar() {
}

func (robot *AIRobot) C2S_UseItemSelfCar() {
}

func (robot *AIRobot) C2S_DestoryAction() {
}

func (robot *AIRobot) C2S_DestoryTheCar() {
}

func (robot *AIRobot) C2S_FinalArmorEventResult() {
}

func (robot *AIRobot) C2S_SayYesAction() {
}

func (robot *AIRobot) C2S_SayYesTheCar() {
}

func (robot *AIRobot) C2S_TimeTurbulenceResult() {
}

func (robot *AIRobot) C2S_ExpandCarport() {
}

func (robot *AIRobot) C2S_ExpandFixCarport() {
}

func (robot *AIRobot) C2S_FixCar() {
}

func (robot *AIRobot) C2S_FastFixRoleCar() {
}

func (robot *AIRobot) C2S_FixRoleCarByCrtStal() {
}

func (robot *AIRobot) C2S_DecomposeCar() {
}

func (robot *AIRobot) C2S_TakeCar() {
}

func (robot *AIRobot) C2S_AskCarLog() {
}

func (robot *AIRobot) C2S_AskCarParkInfo() {
}

func (robot *AIRobot) C2S_BuyParkShopGood() {
}

func (robot *AIRobot) C2S_CheckFixCarIsOK() {
}

func (robot *AIRobot) C2S_AcceptTask() {
}

func (robot *AIRobot) C2S_CompleteTask() {
}

func (robot *AIRobot) C2S_InformTalkTask() {
}

func (robot *AIRobot) C2S_NPCInteractCount() {
}

func (robot *AIRobot) C2S_DailyTaskStatus() {
}

func (robot *AIRobot) C2S_UpdateCostItemTask() {
}

func (robot *AIRobot) C2S_TaskSettle() {
}

func (robot *AIRobot) C2S_GiveUpTask() {
}

// XXXX
func S2C_SendDanmaku(ai *AIRobot, msg []byte) {
}

func S2C_HangUpBegin(ai *AIRobot, msg []byte) {
}

func S2C_HangUpEnd(ai *AIRobot, msg []byte) {
}

func S2C_HangUpInfo(ai *AIRobot, msg []byte) {
}

func S2C_SyncHomeInfo(ai *AIRobot, msg []byte) {
}

func S2C_Bag_Furniture_update(ai *AIRobot, msg []byte) {
}

func S2C_OtherHomeInfo(ai *AIRobot, msg []byte) {
}

func S2C_RoleHomeInfo(ai *AIRobot, msg []byte) {
}

func S2C_BuyHomeShopGood(ai *AIRobot, msg []byte) {
}

func S2C_ExtendRoomSize(ai *AIRobot, msg []byte) {
}

func S2C_HomeDepotSell(ai *AIRobot, msg []byte) {
}

func S2C_ExtendDepotSize(ai *AIRobot, msg []byte) {
}

func S2C_UpdateRoomInfo(ai *AIRobot, msg []byte) {
}

func S2C_MG_RoomInfo(ai *AIRobot, msg []byte) {
}

func S2C_MG_CreateRoom(ai *AIRobot, msg []byte) {
}

func S2C_MG_JoinRoom(ai *AIRobot, msg []byte) {
}

func S2C_MG_LeaveRoom(ai *AIRobot, msg []byte) {
}

func S2C_MG_Ready(ai *AIRobot, msg []byte) {
}

func S2C_MG_SyncReady(ai *AIRobot, msg []byte) {
}

func S2C_MG_SyncCountDown(ai *AIRobot, msg []byte) {
}

func S2C_MG_MasterKickPlayer(ai *AIRobot, msg []byte) {
}

func S2C_MG_ThrowJBEgg(ai *AIRobot, msg []byte) {
}

func S2C_MG_Count_down(ai *AIRobot, msg []byte) {
}

func S2C_GetServerId(ai *AIRobot, msg []byte) {
}

func S2C_CheckAccount(ai *AIRobot, msg []byte) {
}

func S2C_Login(ai *AIRobot, msg []byte) {
	var skData pbd.S2C_Login
	err := proto.Unmarshal(msg, &skData)
	if err != nil {
		ai.StopMsg("S2C_Login", err)
	} else {
		if skData.GetRetCode() == pbd.E_MSG_RET_CODE_Msg_Ret_Code_OK {
			ai.C2S_RoleSum()
		}
	}

}

func S2C_Register(ai *AIRobot, msg []byte) {
}

func S2C_RoleSum(ai *AIRobot, msg []byte) {

	var skData pbd.S2C_RoleSum
	err := proto.Unmarshal(msg, &skData)
	if err != nil {
		ai.StopMsg("S2C_RoleSum", err)
	} else {
		if len(skData.GetListRoleIndex()) == 1 {
			ai.roleIndex = skData.GetListRoleIndex()[0]
			ai.C2S_RoleInfo()
		} else {
			DLog("robot %d create begin", ai.id)
			ai.C2S_CreateRole()
		}
	}
}

func S2C_ClearRole(ai *AIRobot, msg []byte) {
}

func S2C_RandNickName(ai *AIRobot, msg []byte) {
}

func S2C_CreateRole(ai *AIRobot, msg []byte) {
	var skData pbd.S2C_CreateRole
	err := proto.Unmarshal(msg, &skData)
	if err != nil {
		ai.StopMsg("S2C_CreateRole", err)
	} else {
		DLog("robot %d create end retcode %d", ai.id, skData.GetRetCode())
		if skData.GetRetCode() == pbd.E_MSG_RET_CODE_Msg_Ret_Code_OK {
			ai.C2S_RoleSum()
		} else {
			ai.Stop()
		}
	}
}

func S2C_RoleInfo(ai *AIRobot, msg []byte) {
	var skData pbd.S2C_RoleInfo
	err := proto.Unmarshal(msg, &skData)
	if err != nil {
		ai.StopMsg("S2C_RoleInfo", err)
	} else {
		DLog("robot %d S2C_RoleInfo retcode %d", ai.id, skData.GetRetCode())
		if skData.GetRetCode() == pbd.E_MSG_RET_CODE_Msg_Ret_Code_OK {
			ai.C2S_ReadyEnterScene()
		} else {
			ai.StopMsg("role online", nil)
		}
	}
}

func S2C_ReadyEnterScene(ai *AIRobot, msg []byte) {
	var skData pbd.S2C_ReadyEnterScene
	err := proto.Unmarshal(msg, &skData)
	if err != nil {
		ai.StopMsg("S2C_ReadyEnterScene", err)
	} else {

		DLog("robot %d S2C_ReadyEnterScene retcode %d", ai.id, skData.GetRetCode())
		if skData.GetRetCode() == pbd.E_MSG_RET_CODE_Msg_Ret_Code_OK {
			ai.mapID = skData.GetSceneId()
			DLog("robot %d S2C_ReadyEnterScene enter %d", ai.id, ai.mapID)
			ai.C2S_EnterScene()
		} else {
			ai.Stop()
		}
	}
}

func S2C_EnterScene(ai *AIRobot, msg []byte) {
}

func S2C_BornEnterSceneOK(ai *AIRobot, msg []byte) {
}

func S2C_OffLine(ai *AIRobot, msg []byte) {
}

func S2C_Heartbeat(ai *AIRobot, msg []byte) {
}

func S2C_MaterialNotEnough(ai *AIRobot, msg []byte) {
}

func S2C_GameCoin(ai *AIRobot, msg []byte) {
}

func S2C_DiamondCoin(ai *AIRobot, msg []byte) {
}

func S2C_Endurance(ai *AIRobot, msg []byte) {
}

func S2C_RecoveryEndurance(ai *AIRobot, msg []byte) {
}

func S2C_Exp(ai *AIRobot, msg []byte) {
}

func S2C_Level(ai *AIRobot, msg []byte) {
}

func S2C_SkillPoint(ai *AIRobot, msg []byte) {
}

func S2C_ArenaScore(ai *AIRobot, msg []byte) {
}

func S2C_Vip(ai *AIRobot, msg []byte) {
}

func S2C_RoleInfo_EnterScene_Action(ai *AIRobot, pkData *pbd.S2C_RoleInfo_EnterScene) {
	ai.x = pkData.GetBeginX()
	ai.y = pkData.GetBeginY()
	ai.ChangeScene = true
	// ai.C2S_Request_Move()
}

func S2C_RoleInfo_EnterScene(ai *AIRobot, msg []byte) {
	var skData pbd.S2C_RoleInfo_EnterScene
	err := proto.Unmarshal(msg, &skData)
	if err != nil {
		ai.StopMsg("S2C_RoleInfo_EnterScene", err)
	} else {
		S2C_RoleInfo_EnterScene_Action(ai, &skData)
	}
}

func S2C_BornRoleInfo_EnterScene(ai *AIRobot, msg []byte) {
	var skData pbd.S2C_BornRoleInfo_EnterScene
	err := proto.Unmarshal(msg, &skData)
	if err != nil {
		ai.StopMsg("S2C_BornRoleInfo_EnterScene", err)
	} else {
		ai.C2S_BornEnterSceneOK()
		ai.C2S_UploadForceGuideInfo()
		S2C_RoleInfo_EnterScene_Action(ai, skData.GetRoleInfo())
	}
}

func S2C_OtherRoleInfo_EnterScene(ai *AIRobot, msg []byte) {
}

func S2C_ManyOtherRoleInfo_EnterScene(ai *AIRobot, msg []byte) {
}

func S2C_SyncRoleShowInfo(ai *AIRobot, msg []byte) {
}

func S2C_BroadcastRoleShowInfo(ai *AIRobot, msg []byte) {
}

func S2C_RoleInfo_LeaveScene(ai *AIRobot, msg []byte) {
}

func S2C_Request_Move(ai *AIRobot, msg []byte) {
	var skData pbd.S2C_Request_Move
	err := proto.Unmarshal(msg, &skData)
	if err != nil {
		ai.StopMsg("S2C_Request_Move", err)
	} else {
		if skData.GetRoleIndex() == ai.roleIndex {

			ai.moveCnt++
		}
	}

}

func S2C_Chat(ai *AIRobot, msg []byte) {

}

func S2C_Use_SPItem(ai *AIRobot, msg []byte) {
}

func S2C_QuerySceneLineInfo(ai *AIRobot, msg []byte) {
}

func S2C_ChangeSceneLine(ai *AIRobot, msg []byte) {
}

func S2C_TransPortChangeScene(ai *AIRobot, msg []byte) {
}

func S2C_NPC_ChangeScene(ai *AIRobot, msg []byte) {
}

func S2C_Bag_Item(ai *AIRobot, msg []byte) {
}

func S2C_Use_Item(ai *AIRobot, msg []byte) {
}

func S2C_Sell_Item(ai *AIRobot, msg []byte) {
}

func S2C_Sell_Equip(ai *AIRobot, msg []byte) {
}

func S2C_Bag_Equip(ai *AIRobot, msg []byte) {
}

func S2C_Dress_Equip(ai *AIRobot, msg []byte) {
}

func S2C_Role_Equip(ai *AIRobot, msg []byte) {
}

func S2C_TakeOff_Equip(ai *AIRobot, msg []byte) {
}

func S2C_Bag_Item_Insert(ai *AIRobot, msg []byte) {
}

func S2C_Bag_Item_Remove(ai *AIRobot, msg []byte) {
}

func S2C_Bag_Equip_Insert(ai *AIRobot, msg []byte) {
}

func S2C_Bag_Equip_Remove(ai *AIRobot, msg []byte) {
}

func S2C_Bag_Item_Add(ai *AIRobot, msg []byte) {
}

func S2C_Bag_Equip_Add(ai *AIRobot, msg []byte) {
}

func S2C_Bag_Equip_Update(ai *AIRobot, msg []byte) {
}

func S2C_Role_Equip_Update(ai *AIRobot, msg []byte) {
}

func S2C_SuitShopInfo(ai *AIRobot, msg []byte) {
}

func S2C_BuySuitInShop(ai *AIRobot, msg []byte) {
}

func S2C_RoleSuitInfo(ai *AIRobot, msg []byte) {
}

func S2C_DressSuit(ai *AIRobot, msg []byte) {
}

func S2C_TakeOffSuit(ai *AIRobot, msg []byte) {
}

func S2C_FaceShopInfo(ai *AIRobot, msg []byte) {
}

func S2C_BuyFaceInShop(ai *AIRobot, msg []byte) {
}

func S2C_RoleFaceInfo(ai *AIRobot, msg []byte) {
}

func S2C_DressFace(ai *AIRobot, msg []byte) {
}

func S2C_TakeOffFace(ai *AIRobot, msg []byte) {
}

func S2C_ManualUpdateFaceShop(ai *AIRobot, msg []byte) {
}

func S2C_PassRisk(ai *AIRobot, msg []byte) {
}

func S2C_OpenChapterBox(ai *AIRobot, msg []byte) {
}

func S2C_OpenRiskBox(ai *AIRobot, msg []byte) {
}

func S2C_ResetRiskFightCount(ai *AIRobot, msg []byte) {
}

func S2C_ReadyEnterRisk(ai *AIRobot, msg []byte) {
}

func S2C_FriendAssistTime(ai *AIRobot, msg []byte) {
}

func S2C_NPC_Add_RecordQueue(ai *AIRobot, msg []byte) {
}

func S2C_NPC_Go_RecordQueue(ai *AIRobot, msg []byte) {
}

func S2C_Select_Recommend_Friends(ai *AIRobot, msg []byte) {
}

func S2C_FindFriend(ai *AIRobot, msg []byte) {
}

func S2C_Add_Friend(ai *AIRobot, msg []byte) {
}

func S2C_Remove_Friend(ai *AIRobot, msg []byte) {
}

func S2C_Request_Add_Friend(ai *AIRobot, msg []byte) {
}

func S2C_Friend_Online(ai *AIRobot, msg []byte) {
}

func S2C_Friend_Offline(ai *AIRobot, msg []byte) {
}

func S2C_ReadyEnterFriendFightScene(ai *AIRobot, msg []byte) {
}

func S2C_Friend_Fight(ai *AIRobot, msg []byte) {
}

func S2C_CommonShopGoodsInfo(ai *AIRobot, msg []byte) {
}

func S2C_ParkShop(ai *AIRobot, msg []byte) {
}

func S2C_BuyCommonShopGoods(ai *AIRobot, msg []byte) {
}

func S2C_Skill_Study(ai *AIRobot, msg []byte) {
}

func S2C_Skill_LevelUp(ai *AIRobot, msg []byte) {
}

func S2C_Skill_DressTalent(ai *AIRobot, msg []byte) {
}

func S2C_Skill_TakeOffTalent(ai *AIRobot, msg []byte) {
}

func S2C_Skill_DressBook(ai *AIRobot, msg []byte) {
}

func S2C_Skill_TakeOffBook(ai *AIRobot, msg []byte) {
}

func S2C_Skill_ChangeBook(ai *AIRobot, msg []byte) {
}

func S2C_PassMiniGame(ai *AIRobot, msg []byte) {
}

func S2C_QueryMiniGameRank(ai *AIRobot, msg []byte) {
}

func S2C_QuerySelfGameRank(ai *AIRobot, msg []byte) {
}

func S2C_PlayMiniGameAgain(ai *AIRobot, msg []byte) {
}

func S2C_SubmitMiniGameScore(ai *AIRobot, msg []byte) {
}

func S2C_ReadyEnterMiniGame(ai *AIRobot, msg []byte) {
}

func S2C_InteractAct(ai *AIRobot, msg []byte) {
}

func S2C_SelfHiAct(ai *AIRobot, msg []byte) {
}

func S2C_BathroomInteract(ai *AIRobot, msg []byte) {
}

func S2C_Dress_Card_PVE(ai *AIRobot, msg []byte) {
}

func S2C_TakeOff_Card_PVE(ai *AIRobot, msg []byte) {
}

func S2C_Dress_Card_PVP(ai *AIRobot, msg []byte) {
}

func S2C_TakeOff_Card_PVP(ai *AIRobot, msg []byte) {
}

func S2C_ArenaPlayers(ai *AIRobot, msg []byte) {
}

func S2C_Arena_NearSelf(ai *AIRobot, msg []byte) {
}

func S2C_ReadyEnterArenaScene(ai *AIRobot, msg []byte) {
}

func S2C_RefreshArena(ai *AIRobot, msg []byte) {
}

func S2C_ArenaInfo(ai *AIRobot, msg []byte) {
}

func S2C_ArenaFight(ai *AIRobot, msg []byte) {
}

func S2C_ArenaFightEnd(ai *AIRobot, msg []byte) {
}

func S2C_FriendFightEnd(ai *AIRobot, msg []byte) {
}

func S2C_HistoryRankReward(ai *AIRobot, msg []byte) {
}

func S2C_KillListInfo(ai *AIRobot, msg []byte) {
}

func S2C_VisitingCard(ai *AIRobot, msg []byte) {
}

func S2C_BuyArenaCount(ai *AIRobot, msg []byte) {
}

func S2C_ClearArenaCD(ai *AIRobot, msg []byte) {
}

func S2C_OpenStar(ai *AIRobot, msg []byte) {
}

func S2C_QueryRisenStar(ai *AIRobot, msg []byte) {
}

func S2C_QueryLevelRankListInfo(ai *AIRobot, msg []byte) {
}

func S2C_LevelRankLocationSelf(ai *AIRobot, msg []byte) {
}

func S2C_QueryArenaStar(ai *AIRobot, msg []byte) {
}

func S2C_QueryArenaRankListInfo(ai *AIRobot, msg []byte) {
}

func S2C_ArenaRankLocationSelf(ai *AIRobot, msg []byte) {
}

func S2C_QueryDiamondRankInfo(ai *AIRobot, msg []byte) {
}

func S2C_DiamondRankLocationInfo(ai *AIRobot, msg []byte) {
}

func S2C_QueryKillRankInfo(ai *AIRobot, msg []byte) {
}

func S2C_KillRankLocationInfo(ai *AIRobot, msg []byte) {
}

func S2C_SwordInfo(ai *AIRobot, msg []byte) {
}

func S2C_AddSwordCount(ai *AIRobot, msg []byte) {
}

func S2C_SyncSwordCount(ai *AIRobot, msg []byte) {
}

func S2C_SwordEnd(ai *AIRobot, msg []byte) {
}

func S2C_SelectSwordEquip(ai *AIRobot, msg []byte) {
}

func S2C_SelectMountainGodEquip(ai *AIRobot, msg []byte) {
}

func S2C_DailyTask(ai *AIRobot, msg []byte) {
}

func S2C_DailyTaskReward(ai *AIRobot, msg []byte) {
}

func S2C_DailyActivity(ai *AIRobot, msg []byte) {
}

func S2C_DailyActivityValue(ai *AIRobot, msg []byte) {
}

func S2C_DailyActivityReward(ai *AIRobot, msg []byte) {
}

func S2C_BuyStamina(ai *AIRobot, msg []byte) {
}

func S2C_Mail_Have_New(ai *AIRobot, msg []byte) {
}

func S2C_Mail_Title(ai *AIRobot, msg []byte) {
}

func S2C_Mail_Full(ai *AIRobot, msg []byte) {
}

func S2C_Mail_Read(ai *AIRobot, msg []byte) {
}

func S2C_TodaySign(ai *AIRobot, msg []byte) {
}

func S2C_SignInfo(ai *AIRobot, msg []byte) {
}

func S2C_SignAction(ai *AIRobot, msg []byte) {
}

func S2C_SignLottery(ai *AIRobot, msg []byte) {
}

func S2C_SignLotteryShow(ai *AIRobot, msg []byte) {
}

func S2C_SevenDayInfo(ai *AIRobot, msg []byte) {
}

func S2C_GetSevenDayReward(ai *AIRobot, msg []byte) {
}

func S2C_GMSetLevel(ai *AIRobot, msg []byte) {
}

func S2C_GMAddItem(ai *AIRobot, msg []byte) {
}

func S2C_GMAddCoin(ai *AIRobot, msg []byte) {
}

func S2C_GMAddStimina(ai *AIRobot, msg []byte) {
}

func S2C_GMAddDiamond(ai *AIRobot, msg []byte) {
}

func S2C_GMAddCaliburnCount(ai *AIRobot, msg []byte) {
}

func S2C_GMAddArenaCount(ai *AIRobot, msg []byte) {
}

func S2C_GMAddFace(ai *AIRobot, msg []byte) {
}

func S2C_GMAddSuit(ai *AIRobot, msg []byte) {
}

func S2C_GMAddEquip(ai *AIRobot, msg []byte) {
}

func S2C_FunctionOpen(ai *AIRobot, msg []byte) {
}

func S2C_FastTransfer(ai *AIRobot, msg []byte) {
	var skData pbd.S2C_FastTransfer
	err := proto.Unmarshal(msg, &skData)
	if err != nil {
		DLog(err.Error())
		ai.Stop()
	} else {

		if skData.GetRetCode() == pbd.E_MSG_RET_CODE_Msg_Ret_Code_OK {
			DLog("robot %d S2C_FastTransfer retcode %d", ai.id, skData.GetRetCode())
		} else {
			DLog("S2C_FastTransfer %d", skData.GetRetCode())
			ai.Stop()
		}
	}
}

func S2C_EasterEggTransfer(ai *AIRobot, msg []byte) {
}

func S2C_EquipStrongthen(ai *AIRobot, msg []byte) {
}

func S2C_EquipStrongthenEquiped(ai *AIRobot, msg []byte) {
}

func S2C_EquipResolve(ai *AIRobot, msg []byte) {
}

func S2C_EquipFix(ai *AIRobot, msg []byte) {
}

func S2C_EquipFixEquiped(ai *AIRobot, msg []byte) {
}

func S2C_AskFixEquipsInfo(ai *AIRobot, msg []byte) {
}

func S2C_EquipFastFix(ai *AIRobot, msg []byte) {
}

func S2C_GetFixedEquip(ai *AIRobot, msg []byte) {
}

func S2C_PointGoldInfo(ai *AIRobot, msg []byte) {
}

func S2C_PointGold(ai *AIRobot, msg []byte) {
}

func S2C_TouchPersonEasterEgg(ai *AIRobot, msg []byte) {
}

func S2C_ResetPersonEasterEgg(ai *AIRobot, msg []byte) {
}

func S2C_GlobalEasterEggInfo(ai *AIRobot, msg []byte) {
}

func S2C_TouchGlobalEasterEgg(ai *AIRobot, msg []byte) {
}

func S2C_RechargeMoneyInfo(ai *AIRobot, msg []byte) {
}

func S2C_RechargeMoney(ai *AIRobot, msg []byte) {
}

func S2C_VipDailyRewardFlag(ai *AIRobot, msg []byte) {
}

func S2C_GetVipDailyReward(ai *AIRobot, msg []byte) {
}

func S2C_GetSpecialVipReward(ai *AIRobot, msg []byte) {
}

func S2C_AddInterActTool(ai *AIRobot, msg []byte) {
}

func S2C_RemoveInterActTool(ai *AIRobot, msg []byte) {
}

func S2C_RemoveOwnerTool(ai *AIRobot, msg []byte) {
}

func S2C_BathhouseFightEnd(ai *AIRobot, msg []byte) {
}

func S2C_TouchManInBlack(ai *AIRobot, msg []byte) {
}

func S2C_PickUpInterActTool(ai *AIRobot, msg []byte) {
}

func S2C_BathFightStepInfo(ai *AIRobot, msg []byte) {
}

func S2C_CanPlayWaterBall(ai *AIRobot, msg []byte) {
}

func S2C_SyncBathPlayerInfo(ai *AIRobot, msg []byte) {
}

func S2C_PreferFace(ai *AIRobot, msg []byte) {
}

func S2C_FacePreferInfo(ai *AIRobot, msg []byte) {
}

func S2C_InteractEnergy(ai *AIRobot, msg []byte) {
}

func S2C_InterActInfo(ai *AIRobot, msg []byte) {
}

func S2C_QueryUnlockInfo(ai *AIRobot, msg []byte) {
}

func S2C_UnlockInteract(ai *AIRobot, msg []byte) {
}

func S2C_DressInteract(ai *AIRobot, msg []byte) {
}

func S2C_TakeOffInteract(ai *AIRobot, msg []byte) {
}

func S2C_ExChangeInteractHole(ai *AIRobot, msg []byte) {
}

func S2C_InterActManualRewardInfo(ai *AIRobot, msg []byte) {
}

func S2C_GetInterActManualReward(ai *AIRobot, msg []byte) {
}

func S2C_VehicleCompose(ai *AIRobot, msg []byte) {
}

func S2C_EquipManualInfo(ai *AIRobot, msg []byte) {
}

func S2C_GetEquipManualReward(ai *AIRobot, msg []byte) {
}

func S2C_NPCInteract(ai *AIRobot, msg []byte) {
}

func S2C_InteractRedPoint(ai *AIRobot, msg []byte) {
}

func S2C_FriendFightInfo(ai *AIRobot, msg []byte) {
}

func S2C_UpdateFindwayGuideIndex(ai *AIRobot, msg []byte) {
}

func S2C_ExtendSkillHole(ai *AIRobot, msg []byte) {
}

func S2C_RoleCurrentSkills(ai *AIRobot, msg []byte) {
}

func S2C_EquipBroken(ai *AIRobot, msg []byte) {
}

func S2C_CustomFaceInfo(ai *AIRobot, msg []byte) {
}

func S2C_CanUploadImage(ai *AIRobot, msg []byte) {
}

func S2C_UploadImageInfo(ai *AIRobot, msg []byte) {
}

func S2C_DressCustomFace(ai *AIRobot, msg []byte) {
}

func S2C_TakeOffCustomFace(ai *AIRobot, msg []byte) {
}

func S2C_DelelteCustomFace(ai *AIRobot, msg []byte) {
}

func S2C_CustomFaceAuditResult(ai *AIRobot, msg []byte) {
}

func S2C_FaceUploadSuccess(ai *AIRobot, msg []byte) {
}

func S2C_RolePetInfo(ai *AIRobot, msg []byte) {
}

func S2C_SetPetFightStatus(ai *AIRobot, msg []byte) {
}

func S2C_PetEvolve(ai *AIRobot, msg []byte) {
}

func S2C_PetTalentRebuild(ai *AIRobot, msg []byte) {
}

func S2C_FosterPet(ai *AIRobot, msg []byte) {
}

func S2C_PetAdd(ai *AIRobot, msg []byte) {
}

func S2C_PetUpdate(ai *AIRobot, msg []byte) {
}

func S2C_SyncPetLevel(ai *AIRobot, msg []byte) {
}

func S2C_PetEatPill(ai *AIRobot, msg []byte) {
}

func S2C_PetRefreshInfo(ai *AIRobot, msg []byte) {
}

func S2C_RoleNabPet(ai *AIRobot, msg []byte) {
}

func S2C_HorseLight(ai *AIRobot, msg []byte) {
}

func S2C_AssistantTipInfo(ai *AIRobot, msg []byte) {
}

func S2C_NiuDanInfo(ai *AIRobot, msg []byte) {
}

func S2C_TouchNiuDan(ai *AIRobot, msg []byte) {
}

func S2C_ExchangeCode(ai *AIRobot, msg []byte) {
}

func S2C_ArchieveFinish(ai *AIRobot, msg []byte) {
}

func S2C_GetArchieveReward(ai *AIRobot, msg []byte) {
}

func S2C_ArchieveInfo(ai *AIRobot, msg []byte) {
}

func S2C_ReadyEnterGuadratic(ai *AIRobot, msg []byte) {
}

func S2C_EnterGuadratic(ai *AIRobot, msg []byte) {
}

func S2C_GuadraticReady(ai *AIRobot, msg []byte) {
}

func S2C_NextGuadraticDup(ai *AIRobot, msg []byte) {
}

func S2C_GuadraticEnd(ai *AIRobot, msg []byte) {
}

func S2C_SyncGuadraticRoleHP(ai *AIRobot, msg []byte) {
}

func S2C_SyncGuadraticRoleCD(ai *AIRobot, msg []byte) {
}

func S2C_GuadraticBossEnter(ai *AIRobot, msg []byte) {
}

func S2C_GuadraticBossUseSkill(ai *AIRobot, msg []byte) {
}

func S2C_SellBagThing(ai *AIRobot, msg []byte) {
}

func S2C_StoneExchange(ai *AIRobot, msg []byte) {
}

func S2C_FirstMonery(ai *AIRobot, msg []byte) {
}

func S2C_GetFirstMonery(ai *AIRobot, msg []byte) {
}

func S2C_OtherRechargeInfo(ai *AIRobot, msg []byte) {
}

func S2C_GetOtherRechargeReward(ai *AIRobot, msg []byte) {
}

func S2C_IfShareRisk(ai *AIRobot, msg []byte) {
}

func S2C_AreYouKidding(ai *AIRobot, msg []byte) {
}

func S2C_ExtendEquipBag(ai *AIRobot, msg []byte) {
}

func S2C_ActivityInfo(ai *AIRobot, msg []byte) {
}

func S2C_GetActivityReward(ai *AIRobot, msg []byte) {
}

func S2C_ActivityInfoUpdate(ai *AIRobot, msg []byte) {
}

func S2C_PianoStart(ai *AIRobot, msg []byte) {
}

func S2C_PianoPlay(ai *AIRobot, msg []byte) {
}

func S2C_PianoEnd(ai *AIRobot, msg []byte) {
}

func S2C_AskPianoStatus(ai *AIRobot, msg []byte) {
}

func S2C_RoleBagFull(ai *AIRobot, msg []byte) {
}

func S2C_EquipMake(ai *AIRobot, msg []byte) {
}

func S2C_GetMinigameBuyInfo(ai *AIRobot, msg []byte) {
}

func S2C_GetDailyBitCoin(ai *AIRobot, msg []byte) {
}

func S2C_BuyBitCoin(ai *AIRobot, msg []byte) {
}

func S2C_ParkLotInfo(ai *AIRobot, msg []byte) {
}

func S2C_ReadyEnterParkScene(ai *AIRobot, msg []byte) {
}

func S2C_ParkCar(ai *AIRobot, msg []byte) {
}

func S2C_UseItemSelfCar(ai *AIRobot, msg []byte) {
}

func S2C_DestoryAction(ai *AIRobot, msg []byte) {
}

func S2C_FinalArmorEvent(ai *AIRobot, msg []byte) {
}

func S2C_DestoryTheCar(ai *AIRobot, msg []byte) {
}

func S2C_SayYesAction(ai *AIRobot, msg []byte) {
}

func S2C_TimeTurbulence(ai *AIRobot, msg []byte) {
}

func S2C_SayYesTheCar(ai *AIRobot, msg []byte) {
}

func S2C_OldDriver(ai *AIRobot, msg []byte) {
}

func S2C_ExpandCarport(ai *AIRobot, msg []byte) {
}

func S2C_ExpandFixCarport(ai *AIRobot, msg []byte) {
}

func S2C_FixCar(ai *AIRobot, msg []byte) {
}

func S2C_FastFixRoleCar(ai *AIRobot, msg []byte) {
}

func S2C_FixRoleCarByCrtStal(ai *AIRobot, msg []byte) {
}

func S2C_DecomposeCar(ai *AIRobot, msg []byte) {
}

func S2C_TakeCar(ai *AIRobot, msg []byte) {
}

func S2C_AskCarLog(ai *AIRobot, msg []byte) {
}

func S2C_AskCarParkInfo(ai *AIRobot, msg []byte) {
}

func S2C_BuyParkShopGood(ai *AIRobot, msg []byte) {
}

func S2C_AddCar(ai *AIRobot, msg []byte) {
}

func S2C_CheckFixCarIsOK(ai *AIRobot, msg []byte) {
}

func S2C_ParkEventInform(ai *AIRobot, msg []byte) {
}

func S2C_SyncCanAcceptTaskInfo(ai *AIRobot, msg []byte) {
}

func S2C_SyncAcceptedTaskInfo(ai *AIRobot, msg []byte) {
}

func S2C_AcceptTask(ai *AIRobot, msg []byte) {
}

func S2C_CompleteTask(ai *AIRobot, msg []byte) {
}

func S2C_DailyTaskStatus(ai *AIRobot, msg []byte) {
}

func S2C_UpdateCostItemTask(ai *AIRobot, msg []byte) {
}

func S2C_TaskIsCompleted(ai *AIRobot, msg []byte) {
}

func S2C_GiveUpTask(ai *AIRobot, msg []byte) {
}

func init() {
	g_Handle.Register("S2C_SendDanmaku", S2C_SendDanmaku)
	g_Handle.Register("S2C_HangUpBegin", S2C_HangUpBegin)
	g_Handle.Register("S2C_HangUpEnd", S2C_HangUpEnd)
	g_Handle.Register("S2C_HangUpInfo", S2C_HangUpInfo)
	g_Handle.Register("S2C_SyncHomeInfo", S2C_SyncHomeInfo)
	g_Handle.Register("S2C_Bag_Furniture_update", S2C_Bag_Furniture_update)
	g_Handle.Register("S2C_OtherHomeInfo", S2C_OtherHomeInfo)
	g_Handle.Register("S2C_RoleHomeInfo", S2C_RoleHomeInfo)
	g_Handle.Register("S2C_BuyHomeShopGood", S2C_BuyHomeShopGood)
	g_Handle.Register("S2C_ExtendRoomSize", S2C_ExtendRoomSize)
	g_Handle.Register("S2C_HomeDepotSell", S2C_HomeDepotSell)
	g_Handle.Register("S2C_ExtendDepotSize", S2C_ExtendDepotSize)
	g_Handle.Register("S2C_UpdateRoomInfo", S2C_UpdateRoomInfo)
	g_Handle.Register("S2C_MG_RoomInfo", S2C_MG_RoomInfo)
	g_Handle.Register("S2C_MG_CreateRoom", S2C_MG_CreateRoom)
	g_Handle.Register("S2C_MG_JoinRoom", S2C_MG_JoinRoom)
	g_Handle.Register("S2C_MG_LeaveRoom", S2C_MG_LeaveRoom)
	g_Handle.Register("S2C_MG_Ready", S2C_MG_Ready)
	g_Handle.Register("S2C_MG_SyncReady", S2C_MG_SyncReady)
	g_Handle.Register("S2C_MG_SyncCountDown", S2C_MG_SyncCountDown)
	g_Handle.Register("S2C_MG_MasterKickPlayer", S2C_MG_MasterKickPlayer)
	g_Handle.Register("S2C_MG_ThrowJBEgg", S2C_MG_ThrowJBEgg)
	g_Handle.Register("S2C_MG_Count_down", S2C_MG_Count_down)
	g_Handle.Register("S2C_GetServerId", S2C_GetServerId)
	g_Handle.Register("S2C_CheckAccount", S2C_CheckAccount)
	g_Handle.Register("S2C_Login", S2C_Login)
	g_Handle.Register("S2C_Register", S2C_Register)
	g_Handle.Register("S2C_RoleSum", S2C_RoleSum)
	g_Handle.Register("S2C_ClearRole", S2C_ClearRole)
	g_Handle.Register("S2C_RandNickName", S2C_RandNickName)
	g_Handle.Register("S2C_CreateRole", S2C_CreateRole)
	g_Handle.Register("S2C_RoleInfo", S2C_RoleInfo)
	g_Handle.Register("S2C_ReadyEnterScene", S2C_ReadyEnterScene)
	g_Handle.Register("S2C_EnterScene", S2C_EnterScene)
	g_Handle.Register("S2C_BornEnterSceneOK", S2C_BornEnterSceneOK)
	g_Handle.Register("S2C_OffLine", S2C_OffLine)
	g_Handle.Register("S2C_Heartbeat", S2C_Heartbeat)
	g_Handle.Register("S2C_MaterialNotEnough", S2C_MaterialNotEnough)
	g_Handle.Register("S2C_GameCoin", S2C_GameCoin)
	g_Handle.Register("S2C_DiamondCoin", S2C_DiamondCoin)
	g_Handle.Register("S2C_Endurance", S2C_Endurance)
	g_Handle.Register("S2C_RecoveryEndurance", S2C_RecoveryEndurance)
	g_Handle.Register("S2C_Exp", S2C_Exp)
	g_Handle.Register("S2C_Level", S2C_Level)
	g_Handle.Register("S2C_SkillPoint", S2C_SkillPoint)
	g_Handle.Register("S2C_ArenaScore", S2C_ArenaScore)
	g_Handle.Register("S2C_Vip", S2C_Vip)
	g_Handle.Register("S2C_RoleInfo_EnterScene", S2C_RoleInfo_EnterScene)
	g_Handle.Register("S2C_BornRoleInfo_EnterScene", S2C_BornRoleInfo_EnterScene)
	g_Handle.Register("S2C_OtherRoleInfo_EnterScene", S2C_OtherRoleInfo_EnterScene)
	g_Handle.Register("S2C_ManyOtherRoleInfo_EnterScene", S2C_ManyOtherRoleInfo_EnterScene)
	g_Handle.Register("S2C_SyncRoleShowInfo", S2C_SyncRoleShowInfo)
	g_Handle.Register("S2C_BroadcastRoleShowInfo", S2C_BroadcastRoleShowInfo)
	g_Handle.Register("S2C_RoleInfo_LeaveScene", S2C_RoleInfo_LeaveScene)
	g_Handle.Register("S2C_Request_Move", S2C_Request_Move)
	g_Handle.Register("S2C_Chat", S2C_Chat)
	g_Handle.Register("S2C_Use_SPItem", S2C_Use_SPItem)
	g_Handle.Register("S2C_QuerySceneLineInfo", S2C_QuerySceneLineInfo)
	g_Handle.Register("S2C_ChangeSceneLine", S2C_ChangeSceneLine)
	g_Handle.Register("S2C_TransPortChangeScene", S2C_TransPortChangeScene)
	g_Handle.Register("S2C_NPC_ChangeScene", S2C_NPC_ChangeScene)
	g_Handle.Register("S2C_Bag_Item", S2C_Bag_Item)
	g_Handle.Register("S2C_Use_Item", S2C_Use_Item)
	g_Handle.Register("S2C_Sell_Item", S2C_Sell_Item)
	g_Handle.Register("S2C_Sell_Equip", S2C_Sell_Equip)
	g_Handle.Register("S2C_Bag_Equip", S2C_Bag_Equip)
	g_Handle.Register("S2C_Dress_Equip", S2C_Dress_Equip)
	g_Handle.Register("S2C_Role_Equip", S2C_Role_Equip)
	g_Handle.Register("S2C_TakeOff_Equip", S2C_TakeOff_Equip)
	g_Handle.Register("S2C_Bag_Item_Insert", S2C_Bag_Item_Insert)
	g_Handle.Register("S2C_Bag_Item_Remove", S2C_Bag_Item_Remove)
	g_Handle.Register("S2C_Bag_Equip_Insert", S2C_Bag_Equip_Insert)
	g_Handle.Register("S2C_Bag_Equip_Remove", S2C_Bag_Equip_Remove)
	g_Handle.Register("S2C_Bag_Item_Add", S2C_Bag_Item_Add)
	g_Handle.Register("S2C_Bag_Equip_Add", S2C_Bag_Equip_Add)
	g_Handle.Register("S2C_Bag_Equip_Update", S2C_Bag_Equip_Update)
	g_Handle.Register("S2C_Role_Equip_Update", S2C_Role_Equip_Update)
	g_Handle.Register("S2C_SuitShopInfo", S2C_SuitShopInfo)
	g_Handle.Register("S2C_BuySuitInShop", S2C_BuySuitInShop)
	g_Handle.Register("S2C_RoleSuitInfo", S2C_RoleSuitInfo)
	g_Handle.Register("S2C_DressSuit", S2C_DressSuit)
	g_Handle.Register("S2C_TakeOffSuit", S2C_TakeOffSuit)
	g_Handle.Register("S2C_FaceShopInfo", S2C_FaceShopInfo)
	g_Handle.Register("S2C_BuyFaceInShop", S2C_BuyFaceInShop)
	g_Handle.Register("S2C_RoleFaceInfo", S2C_RoleFaceInfo)
	g_Handle.Register("S2C_DressFace", S2C_DressFace)
	g_Handle.Register("S2C_TakeOffFace", S2C_TakeOffFace)
	g_Handle.Register("S2C_ManualUpdateFaceShop", S2C_ManualUpdateFaceShop)
	g_Handle.Register("S2C_PassRisk", S2C_PassRisk)
	g_Handle.Register("S2C_OpenChapterBox", S2C_OpenChapterBox)
	g_Handle.Register("S2C_OpenRiskBox", S2C_OpenRiskBox)
	g_Handle.Register("S2C_ResetRiskFightCount", S2C_ResetRiskFightCount)
	g_Handle.Register("S2C_ReadyEnterRisk", S2C_ReadyEnterRisk)
	g_Handle.Register("S2C_FriendAssistTime", S2C_FriendAssistTime)
	g_Handle.Register("S2C_NPC_Add_RecordQueue", S2C_NPC_Add_RecordQueue)
	g_Handle.Register("S2C_NPC_Go_RecordQueue", S2C_NPC_Go_RecordQueue)
	g_Handle.Register("S2C_Select_Recommend_Friends", S2C_Select_Recommend_Friends)
	g_Handle.Register("S2C_FindFriend", S2C_FindFriend)
	g_Handle.Register("S2C_Add_Friend", S2C_Add_Friend)
	g_Handle.Register("S2C_Remove_Friend", S2C_Remove_Friend)
	g_Handle.Register("S2C_Request_Add_Friend", S2C_Request_Add_Friend)
	g_Handle.Register("S2C_Friend_Online", S2C_Friend_Online)
	g_Handle.Register("S2C_Friend_Offline", S2C_Friend_Offline)
	g_Handle.Register("S2C_ReadyEnterFriendFightScene", S2C_ReadyEnterFriendFightScene)
	g_Handle.Register("S2C_Friend_Fight", S2C_Friend_Fight)
	g_Handle.Register("S2C_CommonShopGoodsInfo", S2C_CommonShopGoodsInfo)
	g_Handle.Register("S2C_ParkShop", S2C_ParkShop)
	g_Handle.Register("S2C_BuyCommonShopGoods", S2C_BuyCommonShopGoods)
	g_Handle.Register("S2C_Skill_Study", S2C_Skill_Study)
	g_Handle.Register("S2C_Skill_LevelUp", S2C_Skill_LevelUp)
	g_Handle.Register("S2C_Skill_DressTalent", S2C_Skill_DressTalent)
	g_Handle.Register("S2C_Skill_TakeOffTalent", S2C_Skill_TakeOffTalent)
	g_Handle.Register("S2C_Skill_DressBook", S2C_Skill_DressBook)
	g_Handle.Register("S2C_Skill_TakeOffBook", S2C_Skill_TakeOffBook)
	g_Handle.Register("S2C_Skill_ChangeBook", S2C_Skill_ChangeBook)
	g_Handle.Register("S2C_PassMiniGame", S2C_PassMiniGame)
	g_Handle.Register("S2C_QueryMiniGameRank", S2C_QueryMiniGameRank)
	g_Handle.Register("S2C_QuerySelfGameRank", S2C_QuerySelfGameRank)
	g_Handle.Register("S2C_PlayMiniGameAgain", S2C_PlayMiniGameAgain)
	g_Handle.Register("S2C_SubmitMiniGameScore", S2C_SubmitMiniGameScore)
	g_Handle.Register("S2C_ReadyEnterMiniGame", S2C_ReadyEnterMiniGame)
	g_Handle.Register("S2C_InteractAct", S2C_InteractAct)
	g_Handle.Register("S2C_SelfHiAct", S2C_SelfHiAct)
	g_Handle.Register("S2C_BathroomInteract", S2C_BathroomInteract)
	g_Handle.Register("S2C_Dress_Card_PVE", S2C_Dress_Card_PVE)
	g_Handle.Register("S2C_TakeOff_Card_PVE", S2C_TakeOff_Card_PVE)
	g_Handle.Register("S2C_Dress_Card_PVP", S2C_Dress_Card_PVP)
	g_Handle.Register("S2C_TakeOff_Card_PVP", S2C_TakeOff_Card_PVP)
	g_Handle.Register("S2C_ArenaPlayers", S2C_ArenaPlayers)
	g_Handle.Register("S2C_Arena_NearSelf", S2C_Arena_NearSelf)
	g_Handle.Register("S2C_ReadyEnterArenaScene", S2C_ReadyEnterArenaScene)
	g_Handle.Register("S2C_RefreshArena", S2C_RefreshArena)
	g_Handle.Register("S2C_ArenaInfo", S2C_ArenaInfo)
	g_Handle.Register("S2C_ArenaFight", S2C_ArenaFight)
	g_Handle.Register("S2C_ArenaFightEnd", S2C_ArenaFightEnd)
	g_Handle.Register("S2C_FriendFightEnd", S2C_FriendFightEnd)
	g_Handle.Register("S2C_HistoryRankReward", S2C_HistoryRankReward)
	g_Handle.Register("S2C_KillListInfo", S2C_KillListInfo)
	g_Handle.Register("S2C_VisitingCard", S2C_VisitingCard)
	g_Handle.Register("S2C_BuyArenaCount", S2C_BuyArenaCount)
	g_Handle.Register("S2C_ClearArenaCD", S2C_ClearArenaCD)
	g_Handle.Register("S2C_OpenStar", S2C_OpenStar)
	g_Handle.Register("S2C_QueryRisenStar", S2C_QueryRisenStar)
	g_Handle.Register("S2C_QueryLevelRankListInfo", S2C_QueryLevelRankListInfo)
	g_Handle.Register("S2C_LevelRankLocationSelf", S2C_LevelRankLocationSelf)
	g_Handle.Register("S2C_QueryArenaStar", S2C_QueryArenaStar)
	g_Handle.Register("S2C_QueryArenaRankListInfo", S2C_QueryArenaRankListInfo)
	g_Handle.Register("S2C_ArenaRankLocationSelf", S2C_ArenaRankLocationSelf)
	g_Handle.Register("S2C_QueryDiamondRankInfo", S2C_QueryDiamondRankInfo)
	g_Handle.Register("S2C_DiamondRankLocationInfo", S2C_DiamondRankLocationInfo)
	g_Handle.Register("S2C_QueryKillRankInfo", S2C_QueryKillRankInfo)
	g_Handle.Register("S2C_KillRankLocationInfo", S2C_KillRankLocationInfo)
	g_Handle.Register("S2C_SwordInfo", S2C_SwordInfo)
	g_Handle.Register("S2C_AddSwordCount", S2C_AddSwordCount)
	g_Handle.Register("S2C_SyncSwordCount", S2C_SyncSwordCount)
	g_Handle.Register("S2C_SwordEnd", S2C_SwordEnd)
	g_Handle.Register("S2C_SelectSwordEquip", S2C_SelectSwordEquip)
	g_Handle.Register("S2C_SelectMountainGodEquip", S2C_SelectMountainGodEquip)
	g_Handle.Register("S2C_DailyTask", S2C_DailyTask)
	g_Handle.Register("S2C_DailyTaskReward", S2C_DailyTaskReward)
	g_Handle.Register("S2C_DailyActivity", S2C_DailyActivity)
	g_Handle.Register("S2C_DailyActivityValue", S2C_DailyActivityValue)
	g_Handle.Register("S2C_DailyActivityReward", S2C_DailyActivityReward)
	g_Handle.Register("S2C_BuyStamina", S2C_BuyStamina)
	g_Handle.Register("S2C_Mail_Have_New", S2C_Mail_Have_New)
	g_Handle.Register("S2C_Mail_Title", S2C_Mail_Title)
	g_Handle.Register("S2C_Mail_Full", S2C_Mail_Full)
	g_Handle.Register("S2C_Mail_Read", S2C_Mail_Read)
	g_Handle.Register("S2C_TodaySign", S2C_TodaySign)
	g_Handle.Register("S2C_SignInfo", S2C_SignInfo)
	g_Handle.Register("S2C_SignAction", S2C_SignAction)
	g_Handle.Register("S2C_SignLottery", S2C_SignLottery)
	g_Handle.Register("S2C_SignLotteryShow", S2C_SignLotteryShow)
	g_Handle.Register("S2C_SevenDayInfo", S2C_SevenDayInfo)
	g_Handle.Register("S2C_GetSevenDayReward", S2C_GetSevenDayReward)
	g_Handle.Register("S2C_GMSetLevel", S2C_GMSetLevel)
	g_Handle.Register("S2C_GMAddItem", S2C_GMAddItem)
	g_Handle.Register("S2C_GMAddCoin", S2C_GMAddCoin)
	g_Handle.Register("S2C_GMAddStimina", S2C_GMAddStimina)
	g_Handle.Register("S2C_GMAddDiamond", S2C_GMAddDiamond)
	g_Handle.Register("S2C_GMAddCaliburnCount", S2C_GMAddCaliburnCount)
	g_Handle.Register("S2C_GMAddArenaCount", S2C_GMAddArenaCount)
	g_Handle.Register("S2C_GMAddFace", S2C_GMAddFace)
	g_Handle.Register("S2C_GMAddSuit", S2C_GMAddSuit)
	g_Handle.Register("S2C_GMAddEquip", S2C_GMAddEquip)
	g_Handle.Register("S2C_FunctionOpen", S2C_FunctionOpen)
	g_Handle.Register("S2C_FastTransfer", S2C_FastTransfer)
	g_Handle.Register("S2C_EasterEggTransfer", S2C_EasterEggTransfer)
	g_Handle.Register("S2C_EquipStrongthen", S2C_EquipStrongthen)
	g_Handle.Register("S2C_EquipStrongthenEquiped", S2C_EquipStrongthenEquiped)
	g_Handle.Register("S2C_EquipResolve", S2C_EquipResolve)
	g_Handle.Register("S2C_EquipFix", S2C_EquipFix)
	g_Handle.Register("S2C_EquipFixEquiped", S2C_EquipFixEquiped)
	g_Handle.Register("S2C_AskFixEquipsInfo", S2C_AskFixEquipsInfo)
	g_Handle.Register("S2C_EquipFastFix", S2C_EquipFastFix)
	g_Handle.Register("S2C_GetFixedEquip", S2C_GetFixedEquip)
	g_Handle.Register("S2C_PointGoldInfo", S2C_PointGoldInfo)
	g_Handle.Register("S2C_PointGold", S2C_PointGold)
	g_Handle.Register("S2C_TouchPersonEasterEgg", S2C_TouchPersonEasterEgg)
	g_Handle.Register("S2C_ResetPersonEasterEgg", S2C_ResetPersonEasterEgg)
	g_Handle.Register("S2C_GlobalEasterEggInfo", S2C_GlobalEasterEggInfo)
	g_Handle.Register("S2C_TouchGlobalEasterEgg", S2C_TouchGlobalEasterEgg)
	g_Handle.Register("S2C_RechargeMoneyInfo", S2C_RechargeMoneyInfo)
	g_Handle.Register("S2C_RechargeMoney", S2C_RechargeMoney)
	g_Handle.Register("S2C_VipDailyRewardFlag", S2C_VipDailyRewardFlag)
	g_Handle.Register("S2C_GetVipDailyReward", S2C_GetVipDailyReward)
	g_Handle.Register("S2C_GetSpecialVipReward", S2C_GetSpecialVipReward)
	g_Handle.Register("S2C_AddInterActTool", S2C_AddInterActTool)
	g_Handle.Register("S2C_RemoveInterActTool", S2C_RemoveInterActTool)
	g_Handle.Register("S2C_RemoveOwnerTool", S2C_RemoveOwnerTool)
	g_Handle.Register("S2C_BathhouseFightEnd", S2C_BathhouseFightEnd)
	g_Handle.Register("S2C_TouchManInBlack", S2C_TouchManInBlack)
	g_Handle.Register("S2C_PickUpInterActTool", S2C_PickUpInterActTool)
	g_Handle.Register("S2C_BathFightStepInfo", S2C_BathFightStepInfo)
	g_Handle.Register("S2C_CanPlayWaterBall", S2C_CanPlayWaterBall)
	g_Handle.Register("S2C_SyncBathPlayerInfo", S2C_SyncBathPlayerInfo)
	g_Handle.Register("S2C_PreferFace", S2C_PreferFace)
	g_Handle.Register("S2C_FacePreferInfo", S2C_FacePreferInfo)
	g_Handle.Register("S2C_InteractEnergy", S2C_InteractEnergy)
	g_Handle.Register("S2C_InterActInfo", S2C_InterActInfo)
	g_Handle.Register("S2C_QueryUnlockInfo", S2C_QueryUnlockInfo)
	g_Handle.Register("S2C_UnlockInteract", S2C_UnlockInteract)
	g_Handle.Register("S2C_DressInteract", S2C_DressInteract)
	g_Handle.Register("S2C_TakeOffInteract", S2C_TakeOffInteract)
	g_Handle.Register("S2C_ExChangeInteractHole", S2C_ExChangeInteractHole)
	g_Handle.Register("S2C_InterActManualRewardInfo", S2C_InterActManualRewardInfo)
	g_Handle.Register("S2C_GetInterActManualReward", S2C_GetInterActManualReward)
	g_Handle.Register("S2C_VehicleCompose", S2C_VehicleCompose)
	g_Handle.Register("S2C_EquipManualInfo", S2C_EquipManualInfo)
	g_Handle.Register("S2C_GetEquipManualReward", S2C_GetEquipManualReward)
	g_Handle.Register("S2C_NPCInteract", S2C_NPCInteract)
	g_Handle.Register("S2C_InteractRedPoint", S2C_InteractRedPoint)
	g_Handle.Register("S2C_FriendFightInfo", S2C_FriendFightInfo)
	g_Handle.Register("S2C_UpdateFindwayGuideIndex", S2C_UpdateFindwayGuideIndex)
	g_Handle.Register("S2C_ExtendSkillHole", S2C_ExtendSkillHole)
	g_Handle.Register("S2C_RoleCurrentSkills", S2C_RoleCurrentSkills)
	g_Handle.Register("S2C_EquipBroken", S2C_EquipBroken)
	g_Handle.Register("S2C_CustomFaceInfo", S2C_CustomFaceInfo)
	g_Handle.Register("S2C_CanUploadImage", S2C_CanUploadImage)
	g_Handle.Register("S2C_UploadImageInfo", S2C_UploadImageInfo)
	g_Handle.Register("S2C_DressCustomFace", S2C_DressCustomFace)
	g_Handle.Register("S2C_TakeOffCustomFace", S2C_TakeOffCustomFace)
	g_Handle.Register("S2C_DelelteCustomFace", S2C_DelelteCustomFace)
	g_Handle.Register("S2C_CustomFaceAuditResult", S2C_CustomFaceAuditResult)
	g_Handle.Register("S2C_FaceUploadSuccess", S2C_FaceUploadSuccess)
	g_Handle.Register("S2C_RolePetInfo", S2C_RolePetInfo)
	g_Handle.Register("S2C_SetPetFightStatus", S2C_SetPetFightStatus)
	g_Handle.Register("S2C_PetEvolve", S2C_PetEvolve)
	g_Handle.Register("S2C_PetTalentRebuild", S2C_PetTalentRebuild)
	g_Handle.Register("S2C_FosterPet", S2C_FosterPet)
	g_Handle.Register("S2C_PetAdd", S2C_PetAdd)
	g_Handle.Register("S2C_PetUpdate", S2C_PetUpdate)
	g_Handle.Register("S2C_SyncPetLevel", S2C_SyncPetLevel)
	g_Handle.Register("S2C_PetEatPill", S2C_PetEatPill)
	g_Handle.Register("S2C_PetRefreshInfo", S2C_PetRefreshInfo)
	g_Handle.Register("S2C_RoleNabPet", S2C_RoleNabPet)
	g_Handle.Register("S2C_HorseLight", S2C_HorseLight)
	g_Handle.Register("S2C_AssistantTipInfo", S2C_AssistantTipInfo)
	g_Handle.Register("S2C_NiuDanInfo", S2C_NiuDanInfo)
	g_Handle.Register("S2C_TouchNiuDan", S2C_TouchNiuDan)
	g_Handle.Register("S2C_ExchangeCode", S2C_ExchangeCode)
	g_Handle.Register("S2C_ArchieveFinish", S2C_ArchieveFinish)
	g_Handle.Register("S2C_GetArchieveReward", S2C_GetArchieveReward)
	g_Handle.Register("S2C_ArchieveInfo", S2C_ArchieveInfo)
	g_Handle.Register("S2C_ReadyEnterGuadratic", S2C_ReadyEnterGuadratic)
	g_Handle.Register("S2C_EnterGuadratic", S2C_EnterGuadratic)
	g_Handle.Register("S2C_GuadraticReady", S2C_GuadraticReady)
	g_Handle.Register("S2C_NextGuadraticDup", S2C_NextGuadraticDup)
	g_Handle.Register("S2C_GuadraticEnd", S2C_GuadraticEnd)
	g_Handle.Register("S2C_SyncGuadraticRoleHP", S2C_SyncGuadraticRoleHP)
	g_Handle.Register("S2C_SyncGuadraticRoleCD", S2C_SyncGuadraticRoleCD)
	g_Handle.Register("S2C_GuadraticBossEnter", S2C_GuadraticBossEnter)
	g_Handle.Register("S2C_GuadraticBossUseSkill", S2C_GuadraticBossUseSkill)
	g_Handle.Register("S2C_SellBagThing", S2C_SellBagThing)
	g_Handle.Register("S2C_StoneExchange", S2C_StoneExchange)
	g_Handle.Register("S2C_FirstMonery", S2C_FirstMonery)
	g_Handle.Register("S2C_GetFirstMonery", S2C_GetFirstMonery)
	g_Handle.Register("S2C_OtherRechargeInfo", S2C_OtherRechargeInfo)
	g_Handle.Register("S2C_GetOtherRechargeReward", S2C_GetOtherRechargeReward)
	g_Handle.Register("S2C_IfShareRisk", S2C_IfShareRisk)
	g_Handle.Register("S2C_AreYouKidding", S2C_AreYouKidding)
	g_Handle.Register("S2C_ExtendEquipBag", S2C_ExtendEquipBag)
	g_Handle.Register("S2C_ActivityInfo", S2C_ActivityInfo)
	g_Handle.Register("S2C_GetActivityReward", S2C_GetActivityReward)
	g_Handle.Register("S2C_ActivityInfoUpdate", S2C_ActivityInfoUpdate)
	g_Handle.Register("S2C_PianoStart", S2C_PianoStart)
	g_Handle.Register("S2C_PianoPlay", S2C_PianoPlay)
	g_Handle.Register("S2C_PianoEnd", S2C_PianoEnd)
	g_Handle.Register("S2C_AskPianoStatus", S2C_AskPianoStatus)
	g_Handle.Register("S2C_RoleBagFull", S2C_RoleBagFull)
	g_Handle.Register("S2C_EquipMake", S2C_EquipMake)
	g_Handle.Register("S2C_GetMinigameBuyInfo", S2C_GetMinigameBuyInfo)
	g_Handle.Register("S2C_GetDailyBitCoin", S2C_GetDailyBitCoin)
	g_Handle.Register("S2C_BuyBitCoin", S2C_BuyBitCoin)
	g_Handle.Register("S2C_ParkLotInfo", S2C_ParkLotInfo)
	g_Handle.Register("S2C_ReadyEnterParkScene", S2C_ReadyEnterParkScene)
	g_Handle.Register("S2C_ParkCar", S2C_ParkCar)
	g_Handle.Register("S2C_UseItemSelfCar", S2C_UseItemSelfCar)
	g_Handle.Register("S2C_DestoryAction", S2C_DestoryAction)
	g_Handle.Register("S2C_FinalArmorEvent", S2C_FinalArmorEvent)
	g_Handle.Register("S2C_DestoryTheCar", S2C_DestoryTheCar)
	g_Handle.Register("S2C_SayYesAction", S2C_SayYesAction)
	g_Handle.Register("S2C_TimeTurbulence", S2C_TimeTurbulence)
	g_Handle.Register("S2C_SayYesTheCar", S2C_SayYesTheCar)
	g_Handle.Register("S2C_OldDriver", S2C_OldDriver)
	g_Handle.Register("S2C_ExpandCarport", S2C_ExpandCarport)
	g_Handle.Register("S2C_ExpandFixCarport", S2C_ExpandFixCarport)
	g_Handle.Register("S2C_FixCar", S2C_FixCar)
	g_Handle.Register("S2C_FastFixRoleCar", S2C_FastFixRoleCar)
	g_Handle.Register("S2C_FixRoleCarByCrtStal", S2C_FixRoleCarByCrtStal)
	g_Handle.Register("S2C_DecomposeCar", S2C_DecomposeCar)
	g_Handle.Register("S2C_TakeCar", S2C_TakeCar)
	g_Handle.Register("S2C_AskCarLog", S2C_AskCarLog)
	g_Handle.Register("S2C_AskCarParkInfo", S2C_AskCarParkInfo)
	g_Handle.Register("S2C_BuyParkShopGood", S2C_BuyParkShopGood)
	g_Handle.Register("S2C_AddCar", S2C_AddCar)
	g_Handle.Register("S2C_CheckFixCarIsOK", S2C_CheckFixCarIsOK)
	g_Handle.Register("S2C_ParkEventInform", S2C_ParkEventInform)
	g_Handle.Register("S2C_SyncCanAcceptTaskInfo", S2C_SyncCanAcceptTaskInfo)
	g_Handle.Register("S2C_SyncAcceptedTaskInfo", S2C_SyncAcceptedTaskInfo)
	g_Handle.Register("S2C_AcceptTask", S2C_AcceptTask)
	g_Handle.Register("S2C_CompleteTask", S2C_CompleteTask)
	g_Handle.Register("S2C_DailyTaskStatus", S2C_DailyTaskStatus)
	g_Handle.Register("S2C_UpdateCostItemTask", S2C_UpdateCostItemTask)
	g_Handle.Register("S2C_TaskIsCompleted", S2C_TaskIsCompleted)
	g_Handle.Register("S2C_GiveUpTask", S2C_GiveUpTask)

	g_Handle.Father(GetMD5Name("S2C_FastTransfer"))
	g_Handle.Father(GetMD5Name("S2C_Login"))
	g_Handle.Father(GetMD5Name("S2C_RoleSum"))
	g_Handle.Father(GetMD5Name("S2C_CreateRole"))
	g_Handle.Father(GetMD5Name("S2C_RoleInfo"))
	g_Handle.Father(GetMD5Name("S2C_ReadyEnterScene"))
	g_Handle.Father(GetMD5Name("S2C_RoleInfo_EnterScene"))
	g_Handle.Father(GetMD5Name("S2C_BornRoleInfo_EnterScene"))
	g_Handle.Father(GetMD5Name("S2C_Request_Move"))

}
