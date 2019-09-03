package ai

import (
	"fmt"
	"log"
	"os"
	"time"
)

type MobaLog struct {
	realHandle  *log.Logger
	LogDir      string
	LogFileName string
	LogLevel    int
	fileHandle  *os.File
	// mu          sync.Mutex
}

var (
	LogHandle     *MobaLog
	CurrLogDay    string
	LogMsgChannel chan *string
)

func init() {
	CurrLogDay = ""
	LogHandle = new(MobaLog)
	LogMsgChannel = make(chan *string, 1024)
	LogHandle.realHandle = new(log.Logger)
	// LogHandle.realHandle.SetFlags(log.Lshortfile | log.LstdFlags )
	LogHandle.realHandle.SetFlags(log.LstdFlags | log.Lmicroseconds)
	LogHandle.realHandle.SetPrefix("INFO:")

	// for i := 0; i != 10; i++ {
	go WriteLog()
	// }
}

func WriteLog() {
	for {

		select {
		case msg := <-LogMsgChannel:
			TodayLogFile(LogHandle)
			LogHandle.realHandle.Println(*msg)

		}
	}
}

func getCurrDate() string {
	return time.Now().Format("20060102")
}

func IsChangeDay() bool {
	return CurrLogDay != getCurrDate()
}

func TodayLogFile(logHandle *MobaLog) {

	// logHandle.mu.Lock()
	// defer logHandle.mu.Unlock()
	if !IsChangeDay() {
		return
	}
	ymd := getCurrDate()
	realFile := fmt.Sprintf("%s/%s.%s.log", logHandle.LogDir, logHandle.LogFileName, ymd)
	f, err := os.OpenFile(realFile, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0777)
	if err != nil {
		log.Fatalf("error opening file: %v", err)
	}

	CurrLogDay = ymd
	LogHandle.realHandle.SetOutput(f)
	if LogHandle.fileHandle != nil {
		LogHandle.fileHandle.Close()
	}
	LogHandle.fileHandle = f

}

func Config(fileDir string, fileName string, lvl int) {

	LogHandle.LogFileName = fileName
	LogHandle.LogDir = fileDir
	LogHandle.LogLevel = lvl

	TodayLogFile(LogHandle)

}

func DLog(format string, v ...interface{}) {
	// _, file, line, _ := runtime.Caller(1)
	// _, fileName := filepath.Split(file)
	// suffixStr := fmt.Sprintf("%s-%d ", fileName, line)
	// format = suffixStr + format
	str := fmt.Sprintf(format, v...)
	LogMsgChannel <- &str
	// fmt.Println(str)
	// LogHandle.realHandle.Print(str)

}

func FLog(format string, v ...interface{}) {
	// _, file, line, _ := runtime.Caller(1)
	// _, fileName := filepath.Split(file)
	// suffixStr := fmt.Sprintf("%s-%d ", fileName, line)
	// format = suffixStr + format
	str := fmt.Sprintf(format, v...)
	fmt.Print(str)
	LogMsgChannel <- &str
	// fmt.Println(str)
	// LogHandle.realHandle.Print(str)
}

func WriteContent(name string, content string) {

	fout, _ := os.Create(name)
	defer fout.Close()

	fout.WriteString(content)
}
