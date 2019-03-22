package raft

import (
	"log"
	"os"
)

// Debugging
const Debug = 0

//var debugLog *log.Logger

func init() {
	log.SetFlags(log.Lmicroseconds)
	outputFile, err := os.OpenFile("raft.log", os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0666)
	if err != nil {
		panic("can't open raft.log")
	}
	//	outputWriter := bufio.NewWriter(outputFile)
	//debugLog := log.New(outputFile, "[Debug]", log.LstdFlags)
	log.SetOutput(outputFile)

}
func DPrintf(format string, a ...interface{}) (n int, err error) {
	if Debug > 0 {
		log.Printf(format, a...)
	}
	return
}
