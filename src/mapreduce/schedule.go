package mapreduce

import (
	"fmt"
	"sync"
	"time"
)

//
// schedule() starts and waits for all tasks in the given phase (mapPhase
// or reducePhase). the mapFiles argument holds the names of the files that
// are the inputs to the map phase, one per map task. nReduce is the
// number of reduce tasks. the registerChan argument yields a stream
// of registered workers; each item is the worker's RPC address,
// suitable for passing to call(). registerChan will yield all
// existing registered workers (if any) and new ones as they register.
//
func schedule(jobName string, mapFiles []string, nReduce int, phase jobPhase, registerChan chan string) {
	var ntasks int
	var n_other int // number of inputs (for reduce) or outputs (for map)
	switch phase {
	case mapPhase:
		ntasks = len(mapFiles)
		n_other = nReduce
	case reducePhase:
		ntasks = nReduce
		n_other = len(mapFiles)
	}

	fmt.Printf("Schedule: %v %v tasks (%d I/Os)\n", ntasks, phase, n_other)

	// All ntasks tasks have to be scheduled on workers. Once all tasks
	// have completed successfully, schedule() should return.
	//
	// Your code here (Part III, Part IV).
	//
	var workerSet []string
	//tmpFile, errFile := os.OpenFile("tmp", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0664)
	//if errFile != nil {
	//	panic("文件打开出错")
	//}
	//tmpFile.Write([]byte(jobName + " " + strconv.Itoa(ntasks) + " " + strconv.Itoa(n_other) + " " + string(phase) + "\n"))
	workerAddr := ""
	//true为正在工作，false表示已经完成
	flag := true
	workerState := make(map[string]bool)
	//读取所有的worker
	for flag {
		select {
		case workerAddr, _ = <-registerChan:
			workerSet = append(workerSet, workerAddr)
			workerState[workerAddr] = false
		case <-time.After(time.Second):
			fmt.Println("After one second!")
			flag = false
			break
		}
	}
	runningTask := 0
	var wg sync.WaitGroup
	wg.Add(len(workerSet))
	//tmpFile.Write([]byte(strconv.Itoa(len(workerSet)) + " \n"))
	//定义失败返回时的数据结构
	type failedWork struct {
		workerAddr string
		taskNum    int
	}
	failedChan := make(chan failedWork, ntasks)
	taskList := make([]int, ntasks)
	for i := 0; i < ntasks; i++ {
		taskList[i] = i
	}
	//NOTE::这里如果使用队列的方式实现会方便很多很多
	//for _, taskNum := range taskList {
	for i := 0; i < len(taskList); i++ {
		//FIXME::每次都要去找下一个，比较浪费时间
		for key, val := range workerState {
			if val == false {
				workerAddr = key
			}
		}
		//tmpFile.Write([]byte(workerAddr + " \n"))
		doTaskArgsToPass := DoTaskArgs{jobName, mapFiles[taskList[i]], phase, taskList[i], n_other}
		go func() {
			currentTaskNum := taskList[i]
			currentWorkerAddr := workerAddr
			flag := call(workerAddr, "Worker.DoTask", doTaskArgsToPass, nil)
			if !flag {
				//failedChan <- (currentTaskNum + currentWorkerAddr)
				failedChan <- failedWork{currentWorkerAddr, currentTaskNum}
			}
			wg.Done()
		}()

		runningTask++
		workerState[workerAddr] = true
		if runningTask == len(workerState) {
			//FIXME：：这里有需要优化的地方：就是这里等待goroutine结束的时候，使用的是waitGroup，没有用一种等待其中一个OK的方式，(原因：还不会)
			wg.Wait()
			flag := true
			for flag {
				select {
				case failed, _ := <-failedChan:
					delete(workerState, failed.workerAddr)
					taskList = append(taskList, failed.taskNum)
				case <-time.After(time.Millisecond * 10):
					flag = false
				}
			}
			runningTask = 0
			for key := range workerState {
				workerState[key] = false
			}
			wg.Add(len(workerState))
			//tmpFile.Write([]byte(strconv.Itoa(len(workerState)) + " len(workerState)\n"))
		}

		//监听新来的请求
		flag := true
		for flag {
			select {
			case workerAddr, _ = <-registerChan:
				//workerSet = append(workerSet, workerAddr)
				workerState[workerAddr] = false
				wg.Add(1)
			case <-time.After(time.Millisecond * 20):
				flag = false
			}
		}
	}
	//	tmpFile.Write([]byte(string(phase) + "finished \n"))
	fmt.Printf("Schedule: %v done\n", phase)
}
