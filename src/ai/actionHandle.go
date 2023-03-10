package ai

import (
	mmd5 "crypto/md5"
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"strings"
	"time"
	"unsafe"
)

func ObjectToJsonStr(v interface{}) (bool, string) {
	str, err := json.Marshal(v)
	if err != nil {
		return false, ""
	}
	return true, string(str)
}

func JsonStrToObject(msg []byte, v interface{}) bool {
	e := json.Unmarshal(msg, v)
	if e != nil {
		fmt.Println(e.Error())
		return false
	}
	return true
}

func GetMD5Name(name string) uint32 {
	w := mmd5.New()
	io.WriteString(w, name) //将str写入到w中
	md5str2 := fmt.Sprintf("%x", w.Sum(nil))
	data := make([]byte, 8)
	copy(data, []byte(md5str2))
	var id uint32
	fmt.Sscanf(string(data[:]), "%x", &id)
	return id
}

func GetMD5Name2(name string) uint32 {
	strList := strings.Split(name, ".")
	sz := len(strList)
	str := strList[sz-1]
	md5str := mmd5.Sum([]byte(str))

	// 取前8个字符
	var id uint32
	md5str[8] = 0
	id = *(*uint32)(unsafe.Pointer(&md5str))
	return id
}

func GetMD5Object(name interface{}) uint32 {
	str := reflect.TypeOf(name).String()
	return GetMD5Name(str)
}

func GetUnixTime() int64 {
	return time.Now().Unix()
}
