package shardmaster

import (
	"fmt"
	"log"
	"os"
	"time"
)

// Debugging
const Debug = 1

var WriteFile *os.File

func init_() {
	log.SetFlags(log.Lmicroseconds)
	outputFile, err := os.OpenFile("sharedmaster.log", os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0666)
	if err != nil {
		panic("can't open sharedmaster.log")
	}
	//	outputWriter := bufio.NewWriter(outputFile)
	//debugLog := log.New(outputFile, "[Debug]", log.LstdFlags)
	log.SetOutput(outputFile)

}

func DPrintf(format string, a ...interface{}) (n int, err error) {
	if WriteFile == nil {
		file, err := os.OpenFile("sharedmaster.log", os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0666)
		if err != nil {
			panic("can't open sharedmaster.log")
		}
		WriteFile = file
	}
	if Debug > 0 {
		//fmt.Fprintf(WriteFile, "%v ", time.Now().Format("3:04PM"))
		fmt.Fprintf(WriteFile, "%v ", time.Now().Format("Jan _2 15:04:05.000000"))
		//fmt.Fprintf(WriteFile, "%v ", time.Now().UnixNano()%1e6)
		fmt.Fprintf(WriteFile, format, a...)
		fmt.Fprintf(WriteFile, "\n")
	}
	return
}
