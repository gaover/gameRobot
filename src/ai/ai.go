package ai

import (
	"net"
)

type NetBuff struct {
	flag  uint32
	index uint32
	len   uint32
	para  uint64
}

type MsgInfo struct {
	index uint32
	msg   []byte
}

type AIRobot struct {
	ip   string
	port string

	account string
	passwd  string

	roleIndex int64

	Conn     net.Conn
	bRun     bool
	bConnect bool
	// 发送时间
	bVip  bool
	id    uint32
	mapID int32

	moveCnt        int32
	x              int32
	y              int32
	chatTime       int64
	heartCnt       int64
	exitChan       chan bool
	MsgChan        chan *MsgInfo
	sendChan       chan *MsgInfo
	moveTime       int64
	ChangeScene    bool
	RoleSum        int32
	chatCnt        int32
	enterScene     bool
	isLogin        bool
	bStop          bool
	enterSceneTime int64
}

type HandlersFunc func(*AIRobot, []byte)

type ActionHandle struct {
	handles   map[uint32]HandlersFunc
	hasFather map[uint32]bool
}

func (pkHandle *ActionHandle) Father(id uint32) {
	if _, ok := pkHandle.hasFather[id]; !ok {
		pkHandle.hasFather[id] = true
	}
}

func (pkHandle *ActionHandle) IsFather(id uint32) bool {
	if _, ok := pkHandle.hasFather[id]; ok {
		return true
	}
	return false
}

func (pkHandle *ActionHandle) Register(name string, handle HandlersFunc) {
	id := GetMD5Name(name)
	if _, ok := pkHandle.handles[id]; !ok {
		pkHandle.handles[id] = handle
	} else {
		panic("why register again " + name)
	}
}

func (pkHandle *ActionHandle) GetAction(id uint32) HandlersFunc {
	if h, ok := pkHandle.handles[id]; ok {
		return h
	}
	return nil
}
