package raft

//
// this is an outline of the API that raft must expose to
// the service (or tester). see comments below for
// each of these functions for more details.
//
// rf = Make(...)
//   create a new Raft server.
// rf.Start(command interface{}) (index, term, isleader)
//   start agreement on a new log entry
// rf.GetState() (term, isLeader)
//   ask a Raft for its current term, and whether it thinks it is leader
// ApplyMsg
//   each time a new entry is committed to the log, each Raft peer
//   should send an ApplyMsg to the service (or tester)
//   in the same server.
//

import (
	"fmt"
	"labrpc"
	"math/rand"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// import "bytes"
// import "labgob"

//
// as each Raft peer becomes aware that successive log entries are
// committed, the peer should send an ApplyMsg to the service (or
// tester) on the same server, via the applyCh passed to Make(). set
// CommandValid to true to indicate that the ApplyMsg contains a newly
// committed log entry.
//
// in Lab 3 you'll want to send other kinds of messages (e.g.,
// snapshots) on the applyCh; at that point you can add fields to
// ApplyMsg, but set CommandValid to false for these other uses.
//
type ApplyMsg struct {
	CommandValid bool
	Command      interface{}
	CommandIndex int
}

type commandTerm struct {
	command interface{}
	term    int
}

// A Go object implementing a single Raft peer.
//
type Raft struct {
	mu        sync.Mutex          // Lock to protect shared access to this peer's state
	peers     []*labrpc.ClientEnd // RPC end points of all peers
	persister *Persister          // Object to hold this peer's persisted state
	me        int                 // this peer's index into peers[]

	// Your data here (2A, 2B, 2C).
	// Look at the paper's Figure 2 for a description of what
	// state a Raft server must maintain.
	///all servers

	leaderNum   int
	currentTerm int
	votedFor    int
	log         []commandTerm
	///volatile state on all servers
	commitIndex int
	lastApplied int
	///for leader
	///use to track the state of replica
	nextIndex  []int
	matchIndex []int

	//variables to sequencely execute the command
	commandToBeExec chan *UnexecuteCommand
	//taskWL          sync.Mutex // Lock to protect shared access to this peer's state    ---no need the only pos of this chan be writed
	applyCh chan ApplyMsg
}

//UnexecuteCommand : the command need to execute
type UnexecuteCommand struct {
	command interface{}
	term    int
	index   int
	success bool
	//when  process finishes, it will return the index the command laied
	//processFinished chan int
}

//CommandTerm:for outer reference; the same functionality with commandTerm
type CommandTerm struct {
	Command interface{}
	Term    int
}

//AppendEntry to send
type AppendEntry struct {
	Term         int
	LeaderID     int
	PrevLogIndex int

	PrevLogTerm  int
	Entries      []CommandTerm
	LeaderCommit int
}

//AppendEntryReply reply to appendentry
type AppendEntryReply struct {
	Me      int
	Term    int
	Success bool
}

// return currentTerm and whether this server
// believes it is the leader.
func (rf *Raft) GetState() (int, bool) {

	var term int
	var isLeader bool
	// Your code here (2A).
	rf.mu.Lock()
	term = rf.currentTerm
	isLeader = (rf.leaderNum == rf.me)
	rf.mu.Unlock()
	if isLeader {
		DPrintf("node %d is leader", rf.me)
	}
	return term, isLeader
}

//
// save Raft's persistent state to stable storage,
// where it can later be retrieved after a crash and restart.
// see paper's Figure 2 for a description of what should be persistent.
//
func (rf *Raft) persist() {
	// Your code here (2C).
	// Example:
	// w := new(bytes.Buffer)
	// e := labgob.NewEncoder(w)
	// e.Encode(rf.xxx)
	// e.Encode(rf.yyy)
	// data := w.Bytes()
	// rf.persister.SaveRaftState(data)
}

//
// restore previously persisted state.
//
func (rf *Raft) readPersist(data []byte) {
	if data == nil || len(data) < 1 { // bootstrap without any state?
		return
	}
	// Your code here (2C).
	// Example:
	// r := bytes.NewBuffer(data)
	// d := labgob.NewDecoder(r)
	// var xxx
	// var yyy
	// if d.Decode(&xxx) != nil ||
	//    d.Decode(&yyy) != nil {
	//   error...
	// } else {
	//   rf.xxx = xxx
	//   rf.yyy = yyy
	// }
}

//
// example RequestVote RPC arguments structure.
// field names must start with capital letters!
//
type RequestVoteArgs struct {
	// Your data here (2A, 2B).
	Term         int
	CandidateID  int
	LastLogIndex int
	LastLogTerm  int
}

//
// example RequestVote RPC reply structure.
// field names must start with capital letters!
//
type RequestVoteReply struct {
	// Your data here (2A).
	Term        int
	VoteGranted bool
}

//global varible
const (
	TermLength = 1000
	HBInterval = 100
)

var AENotifyChan chan bool

// example RequestVote RPC handler.
//
func (rf *Raft) RequestVote(args *RequestVoteArgs, reply *RequestVoteReply) {
	// Your code here (2A, 2B).
	candidateID := args.CandidateID
	candidateTerm := args.Term
	rf.mu.Lock()
	currTerm := rf.currentTerm
	rf.mu.Unlock()

	reply.Term = currTerm
	var voteForFlag int
	DPrintf("node %d getRV currTerm:%d  candidateTerm:%d args.LastLogIndex:%d len(rf.log):%d", rf.me, currTerm, candidateTerm, args.LastLogIndex, len(rf.log))
	if currTerm < candidateTerm && args.LastLogIndex >= len(rf.log)-1 {
		if len(rf.log) > 1 && args.LastLogIndex == len(rf.log)-1 && rf.log[len(rf.log)-1].term != args.LastLogTerm {
			reply.VoteGranted = false
			voteForFlag = 0
		}
		rf.votedFor = candidateID
		reply.VoteGranted = true
		voteForFlag = 1
	} else {
		reply.VoteGranted = false
		voteForFlag = 0
	}
	DPrintf("node %d get RV from %d voting %d", rf.me, args.CandidateID, voteForFlag)
}

//AppendEntry...
func (rf *Raft) AppendEntry(args *AppendEntry, reply *AppendEntryReply) {
	defer func() {
		if e := recover(); e != nil {
			DPrintf("node %d error value is %v", rf.me, e)
		}
		if reply.Success {
			DPrintf("node %d get command %v from leader %d", rf.me, args.Entries, args.LeaderID)
		}
		AENotifyChan <- true
	}()

	var curTerm int
	rf.mu.Lock()
	curTerm = rf.currentTerm
	rf.mu.Unlock()

	//1. Reply false if term < currentTerm (§5.1)
	reply.Me = rf.me
	reply.Term = curTerm
	//	DPrintf("AE:node %d currTerm:%d  candidateTerm:%d args.LastLogIndex:%d len(rf.log):%d", rf.me, curTerm, candidateTerm, args.LastLogIndex, len(rf.log))
	DPrintf("node %d get AE imply leader %d | args.Term %d,  curTerm %d", rf.me, args.LeaderID, args.Term, curTerm)
	if args.Term < curTerm {
		reply.Success = false
		DPrintf("node %d returned because of args.Term < curTerm", rf.me)
		return
	} else if args.Term > curTerm && rf.leaderNum == rf.me {
		rf.leaderNum = args.LeaderID

	}
	DPrintf("node %d pass 1 |prevlogindex %d", rf.me, args.PrevLogIndex)
	rf.currentTerm = args.Term

	//TODO:2. Reply false if log doesn’t contain an entry at prevLogIndex whose term matches prevLogTerm (§5.3)
	existFlag := true
	if args.PrevLogIndex == 0 {
		existFlag = true
	} else {
		if args.PrevLogIndex >= len(rf.log) {
			existFlag = false
		} else if rf.log[args.PrevLogIndex].term != args.PrevLogTerm {
			existFlag = false
		}
	}
	if !existFlag {
		reply.Success = false
		DPrintf("node %d returned because of existFlag=false ", rf.me)
		//DPrintf("node %d returned because of args.Entries==nil ", rf.me)
		return
	}
	DPrintf("node %d pass 2", rf.me)
	preCommitIndex := rf.commitIndex
	DPrintf("node %d  mycommit %d leadercommit %d |prevLogindex %d", rf.me, preCommitIndex, args.LeaderCommit, args.PrevLogIndex)
	if args.PrevLogIndex < rf.commitIndex {
		panic("ERROR" + strconv.Itoa(rf.me) + " args.PrevLogIndex < rf.commitIndex ")
	}
	if args.LeaderCommit > rf.commitIndex {
		if args.LeaderCommit > args.PrevLogIndex {
			rf.commitIndex = args.PrevLogIndex
		} else {
			rf.commitIndex = args.LeaderCommit
		}
	} else if args.LeaderCommit < rf.commitIndex {
		panic("ERROR" + strconv.Itoa(rf.me) + " node args.LeaderCommit<rf.commitIndex")
	}
	DPrintf("node %d pass 5", rf.me)
	//nil implies the HB packet
	for ind := range rf.log[preCommitIndex:rf.commitIndex] {
		var applyMsg ApplyMsg
		applyMsg.Command = rf.log[ind+preCommitIndex+1].command
		applyMsg.CommandIndex = ind + preCommitIndex + 1
		applyMsg.CommandValid = true
		DPrintf("node %d commandIndex %d command %d", rf.me, applyMsg.CommandIndex, applyMsg.Command)
		rf.applyCh <- applyMsg
	}
	if args.Entries == nil {
		reply.Success = true
		DPrintf("node %d returned because of args.Entries==nil ", rf.me)
		return
	}
	//3. If an existing entry conflicts with a new one (same index but different terms), delete the existing entry and all that follow it (§5.3)
	startInd := args.PrevLogIndex + 1
	for _, oneLog := range args.Entries {
		var comm commandTerm
		comm.term = oneLog.Term
		comm.command = oneLog.Command
		if startInd >= len(rf.log) {
			rf.log = append(rf.log, comm)
			DPrintf("node %d startInd %d , len(rf.log) %d, term %d, command %d, value %v", rf.me, startInd, len(rf.log), comm.term, comm.command, rf.log[0])
		} else {
			if rf.log[startInd].term != oneLog.Term {
				rf.log[startInd].term = oneLog.Term
			}
			rf.log[startInd].command = oneLog.Command
		}
		startInd++

	}

	DPrintf("node %d pass 3", rf.me)
	reply.Success = true
	//TODO::optimize needed
	close(rf.commandToBeExec)
	rf.commandToBeExec = make(chan *UnexecuteCommand, 256)
	DPrintf("node %d pass all ", rf.me)
	if args.Term > curTerm {
		rf.mu.Lock()
		rf.currentTerm = args.Term
		rf.mu.Unlock()
		DPrintf("node %d update term from %d to %d", rf.me, curTerm, args.Term)
	}
	// for ind := range rf.log[preCommitIndex:rf.commitIndex] {
	// 	var applyMsg ApplyMsg
	// 	applyMsg.Command = rf.log[ind+preCommitIndex+1].command
	// 	applyMsg.CommandIndex = ind + preCommitIndex + 1
	// 	applyMsg.CommandValid = true
	// 	DPrintf("node %d commandIndex %d command %d", rf.me, applyMsg.CommandIndex, applyMsg.Command)
	// 	rf.applyCh <- applyMsg
	// }
}

//
// example code to send a RequestVote RPC to a server.
// server is the index of the target server in rf.peers[].
// expects RPC arguments in args.
// fills in *reply with RPC reply, so caller should
// pass &reply.
// the types of the args and reply passed to Call() must be
// the same as the types of the arguments declared in the
// handler function (including whether they are pointers).
//
// The labrpc package simulates a lossy network, in which servers
// may be unreachable, and in which requests and replies may be lost.
// Call() sends a request and waits for a reply. If a reply arrives
// within a timeout interval, Call() returns true; otherwise
// Call() returns false. Thus Call() may not return for a while.
// A false return can be caused by a dead server, a live server that
// can't be reached, a lost request, or a lost reply.
//
// Call() is guaranteed to return (perhaps after a delay) *except* if the
// handler function on the server side does not return.  Thus there
// is no need to implement your own timeouts around Call().
//
// look at the comments in ../labrpc/labrpc.go for more details.
//
//Attention: Captial
//
// if you're having trouble getting RPC to work, check that you've
// capitalized all field names in structs passed over RPC, and
// that the caller passes the address of the reply struct with &, not
// the struct itself.
//

func (rf *Raft) sendRequestVote(server int, args *RequestVoteArgs, reply *RequestVoteReply) bool {
	ok := rf.peers[server].Call("Raft.RequestVote", args, reply)
	return ok
}

func (rf *Raft) sendAppendEntry(server int, args *AppendEntry, reply *AppendEntryReply) bool {
	ok := rf.peers[server].Call("Raft.AppendEntry", args, reply)
	return ok
}

// the service using Raft (e.g. a k/v server) wants to start
// agreement on the next command to be appended to Raft's log. if this
// server isn't the leader, returns false. otherwise start the
// agreement and return immediately. there is no guarantee that this
// command will ever be committed to the Raft log, since the leader
// may fail or lose an election.
// even if the Raft instance has been killed,ppen
// this function should return gracefully.
//
// the first return value is the index that the command will appear at
// if it's ever committed. the second return value is the current
// term. the third return value is true if this server believes it is
// the leader.
//
func (rf *Raft) Start(command interface{}) (int, int, bool) {
	index := -1
	term := -1
	isLeader := true
	// Your code here (2B).
	DPrintf("node %d receive command value %v", rf.me, command)
	rf.mu.Lock()
	leaderNum := rf.leaderNum
	curTerm := rf.currentTerm
	rf.mu.Unlock()
	comTE := &UnexecuteCommand{}
	comTE.command = command
	comTE.term = curTerm
	if leaderNum != rf.me {
		isLeader = false
		return index, term, isLeader
	}
	DPrintf("node %d  waiting %d", rf.me, command)
	rf.commandToBeExec <- comTE
	//comTE.processFinished = make(chan int)
	//index = <-comTE.processFinished
	rf.mu.Lock()
	index = len(rf.log)
	rf.mu.Unlock()
	DPrintf("node %d finished waiting index %d", rf.me, index)
	return index, term, isLeader
}

//only for debug
func GoID() int {
	var buf [64]byte
	n := runtime.Stack(buf[:], false)
	idField := strings.Fields(strings.TrimPrefix(string(buf[:n]), "goroutine "))[0]
	id, err := strconv.Atoi(idField)
	if err != nil {
		panic(fmt.Sprintf("cannot get goroutine id: %v", err))
	}
	return id
}

//
// the tester calls Kill() when a Raft instance won't
// be needed again. you are not required to do anything
// in Kill(), but it might be convenient to (for example)
// turn off debug output from this instance.
//
func (rf *Raft) Kill() {
	// Your code here, if desired.
}

func (rf *Raft) CandidateProcess(electionTimeOut, majority int) bool {
	//startT := time.Now()
	//这个channel地方就这么写了先
	RVchan := make(chan *RequestVoteReply, len(rf.peers))
	DPrintf("node %d candidate TERM: %d len of rf.log %d ", rf.me, rf.currentTerm, len(rf.log))
	var lastLogTerm int
	if len(rf.log) == 0 {
		lastLogTerm = 0
	} else {
		//this part may need a lock
		lastLogTerm = rf.log[len(rf.log)-1].term
	}
	rf.votedFor = rf.me
	requestVote := &RequestVoteArgs{
		Term:         rf.currentTerm,
		CandidateID:  rf.me,
		LastLogIndex: len(rf.log),
		LastLogTerm:  lastLogTerm,
	}
	granted := 1
	for ind := range rf.peers {
		if ind == rf.me {
			continue
		}
		go func(ind int) {
			defer func() {
				if e := recover(); e != nil {
					DPrintf("send routines down", e)
				}
			}()
			//three times try
			for count := 0; count < 3; count++ {
				requestVoteReply := &RequestVoteReply{}
				DPrintf("node %d try RV %d time to %d ", rf.me, count, ind)
				if rf.sendRequestVote(ind, requestVote, requestVoteReply) == true {
					RVchan <- requestVoteReply
					break
				}
				DPrintf("TIME: %d node: %d  send RV  to %d failed ", count, rf.me, ind)
			}
		}(ind)
	}
	var timer *time.Timer
	var pReply *RequestVoteReply
	isTimeOut := false
	//end := time.Since(startT)
	//fmt.Println("before select ", end)
	for !isTimeOut {
		select {
		//listen the reply from RV
		case pReply = <-RVchan:
			if pReply.VoteGranted {
				granted++
				if granted >= majority {
					rf.leaderNum = rf.me
					DPrintf("node %d being leader", rf.me)
					return false
					//there are things to be done
				}
			}
		//listen the signal of timeout
		case <-func() <-chan time.Time {
			if timer == nil {
				timer = time.NewTimer(time.Millisecond * time.Duration(electionTimeOut))
				DPrintf("node %d candidate started ", rf.me)
			}
			return timer.C
		}():
			DPrintf("node %d time out", rf.me)
			isTimeOut = true
			rf.currentTerm++
			close(RVchan)
		//listen the signal of AE
		case <-AENotifyChan:
			DPrintf("node %d in the AENotifyChan", rf.me)
			return false
		}
	}
	if isTimeOut {
		return true
	} else {
		return false
	}
}

//there should have a chan as argument, because the command may come
func (rf *Raft) LeaderProcess(electionTimeOut int, applyCh chan ApplyMsg) int {
	defer func() {
		if e := recover(); e != nil {
			DPrintf("node %d leader error:%v", rf.me, e)
		}
	}()
	var TermTimer *time.Timer
	var HBTimer *time.Timer
	//more buf than it actually needs
	AEDistributedChan := make(chan interface{}, len(rf.peers))
	//TODO::this channel may need more then len(rf.peers) pos
	AEChan := make(chan bool, 2*len(rf.peers))
	//to first notify leader to send empty HB
	AEDistributedChan <- nil

	istermTimeOut := false
	DPrintf("leade %d in the leader process", rf.me)

	nextInd := len(rf.log)
	rf.nextIndex = make([]int, len(rf.peers))
	for ind, _ := range rf.nextIndex {
		rf.nextIndex[ind] = nextInd
	}
	rf.matchIndex = make([]int, len(rf.peers))
	//slice to remember the AER of state of command
	state := make([]int32, len(rf.peers))
	var commandToWriteToLog *UnexecuteCommand
	AERcount := 0
	for !istermTimeOut {
		select {
		//HB timer
		case <-func() <-chan time.Time {
			if HBTimer == nil {
				HBTimer = time.NewTimer(time.Millisecond * time.Duration(HBInterval))
			}
			//here send HB parket
			return HBTimer.C
		}():
			DPrintf("leader %d send HB", rf.me)
			HBTimer.Reset(time.Millisecond * time.Duration(HBInterval))
			AEDistributedChan <- nil
		case commandToBeExec := <-AEDistributedChan:
			//distributes AE(HB)
			DPrintf("leader %d id:%d distributing AE", rf.me, GoID())
			appEn := &AppendEntry{}
			appEn.Term = rf.currentTerm
			appEn.LeaderID = rf.me
			var unExecute *UnexecuteCommand
			if commandToBeExec == nil {
				DPrintf("leader %d command==nil", rf.me)
			} else {
				AERcount = 0
				unExecute = commandToBeExec.(*UnexecuteCommand)
				var commT CommandTerm
				DPrintf("leader %d has the command %d", rf.me, (*unExecute).command.(int))
				commT.Command = unExecute.command.(int)
				commT.Term = unExecute.term
				appEn.Entries = append(appEn.Entries, commT)
			}
			//remember the pre appendentry
			appEn.PrevLogIndex = len(rf.log) - 1
			if len(rf.log) != 0 {
				appEn.PrevLogTerm = rf.log[len(rf.log)-1].term
			}
			appEn.LeaderCommit = rf.commitIndex
			for ind := range rf.peers {
				if ind == rf.me {
					continue
				}
				if appEn.Entries != nil {
					state[ind]++
				}
				go func(ind int, commState int32) {
					defer func() {
						if e := recover(); e != nil {
							DPrintf("AE send routines down %v", e)
						}
					}()
					//three times try
					var appEnPointer *AppendEntry
					appEnPointer = appEn
					prevLogIndex := appEnPointer.PrevLogIndex
					//DPrintf("the length of appnendEntry is %d", len(appEnPointer.Entries))
					for count := 0; ; count++ {
						appendEntryReply := &AppendEntryReply{}
						if atomic.LoadInt32(&state[ind]) != commState {
							break
						}
						DPrintf("leader %d try AE %d time to %d ", rf.me, count, ind)
						if rf.sendAppendEntry(ind, appEnPointer, appendEntryReply) == true {
							if appendEntryReply.Success {
								DPrintf("leader %d get success from %d", rf.me, appendEntryReply.Me)
								if len(appEnPointer.Entries) == 0 || appEnPointer.Entries == nil {
									DPrintf("node %d AE return True because of the nil", appendEntryReply.Me)
									return
								}
								rf.nextIndex[ind] = appEnPointer.PrevLogIndex + len(appEnPointer.Entries) + 1
								rf.matchIndex[ind] = rf.nextIndex[ind] - 1
								DPrintf("leader %d: node %d success apply one command %d AE prevlogindex %v at term %d", rf.me, appendEntryReply.Me, appEn.Entries[0].Command, appEnPointer.PrevLogIndex, rf.currentTerm)
								if appEnPointer.PrevLogIndex != prevLogIndex {
									var appEnTmp AppendEntry
									appEnTmp.LeaderCommit = appEnPointer.LeaderCommit
									appEnTmp.LeaderID = appEnPointer.LeaderID
									appEnTmp.PrevLogIndex = appEnPointer.PrevLogIndex + 1

									appEnTmp.Term = rf.currentTerm
									commToVerify := rf.log[appEnTmp.PrevLogIndex+1]
									appEnTmp.LeaderCommit = rf.commitIndex
									appEnTmp.Entries = append(appEnTmp.Entries, CommandTerm{commToVerify.command, commToVerify.term})
									appEnTmp.PrevLogTerm = rf.log[appEnTmp.PrevLogIndex].term
									appEnPointer = &appEnTmp
									DPrintf("leader %d to %d AE increment to preIndex %d", rf.me, ind, appEnTmp.PrevLogIndex)
									continue
								}
								DPrintf("node %d AER return true | commState:%d state %d of index %d", appendEntryReply.Me, commState, atomic.LoadInt32(&state[ind]), ind)
								if atomic.LoadInt32(&state[ind]) == commState {
									DPrintf("node %d AER return true and notified ", appendEntryReply.Me)
									AEChan <- true
								}
								return
							} else {
								if appendEntryReply.Term > rf.currentTerm {
									rf.leaderNum = -1
									AENotifyChan <- true
									return
								}
								var appEnTmp AppendEntry
								appEnTmp.LeaderCommit = appEnPointer.LeaderCommit
								appEnTmp.LeaderID = appEnPointer.LeaderID
								prevlog := appEnPointer.PrevLogIndex - 1
								if prevlog < 0 {
									DPrintf("leader %d prevlogindex becomes <0", rf.me)
								}
								appEnTmp.PrevLogIndex = prevlog
								commToVerify := rf.log[appEnTmp.PrevLogIndex+1]
								var TmpComTerm CommandTerm
								TmpComTerm.Term = commToVerify.term
								DPrintf("leader %d go to decrement the %d's log to prelog %d count %d command Term:%d", rf.me, appendEntryReply.Me, prevlog, count, commToVerify.term)
								TmpComTerm.Command = commToVerify.command
								appEnTmp.Entries = append(appEnTmp.Entries, TmpComTerm)
								appEnTmp.Term = rf.currentTerm
								appEnTmp.LeaderCommit = rf.commitIndex
								appEnTmp.PrevLogTerm = rf.log[appEnTmp.PrevLogIndex].term
								appEnPointer = &appEnTmp
							}
						} else {
							DPrintf("TIME: %d node: %d  send AE to %d failed ", count, rf.me, ind)
						}
					}
				}(ind, state[ind])
			}
			HBTimer.Reset(time.Millisecond * time.Duration(HBInterval))
			//leader timer
		case <-AEChan:
			AERcount++
			DPrintf("leader %d in the AEChan :AERcount %d", rf.me, AERcount)
			marjority := int((len(rf.peers) - 1) / 2)
			if AERcount >= marjority {
				if commandToWriteToLog == nil {
					continue
				}
				DPrintf("leader %d: set majority %d", rf.me, marjority)
				var commandToAppend commandTerm
				commandToAppend.command = commandToWriteToLog.command
				commandToAppend.term = commandToWriteToLog.term
				rf.log = append(rf.log, commandToAppend)
				rf.commitIndex = len(rf.log) - 1
				//commandToWriteToLog.processFinished <- len(rf.log) - 1
				//DPrintf("leader %d send index %v", rf.me, len(rf.log)-1)
				var applyMsg ApplyMsg
				applyMsg.Command = commandToAppend.command
				applyMsg.CommandIndex = len(rf.log) - 1
				applyMsg.CommandValid = true
				applyCh <- applyMsg
				DPrintf("leader %d command %v apply successful| AERCount: %d", rf.me, commandToAppend.command, AERcount)
				AERcount = 0
				commandToWriteToLog = nil
			}
		case <-func() <-chan time.Time {
			if TermTimer == nil {
				TermTimer = time.NewTimer(time.Millisecond * time.Duration(TermLength))
			}
			return TermTimer.C
		}():
			DPrintf("leader %d :Term timeout...go to new term and elect", rf.me)
			istermTimeOut = true
			rf.mu.Lock()
			rf.currentTerm++
			rf.mu.Unlock()
			TermTimer.Reset(time.Millisecond * time.Duration(TermLength))
			break
		//operation when receive the AE notification of other
		case <-AENotifyChan:
			DPrintf("leader %d receives AE from other", rf.me)
			if rf.me != rf.leaderNum {
				return -1
			}
		case commTBE := <-rf.commandToBeExec:
			DPrintf("leader %d process the task: command %v ", rf.me, commTBE.command)
			commandToWriteToLog = commTBE
			AEDistributedChan <- commTBE
		}
	}
	return 0
}

func (rf *Raft) Follower(electionTimeOut int) {
	var TermTimer *time.Timer
	var isTimeOut = false
	for !isTimeOut {
		select {
		case <-func() <-chan time.Time {
			if TermTimer == nil {
				TermTimer = time.NewTimer(time.Millisecond * time.Duration(electionTimeOut))
			}
			return TermTimer.C
		}():
			isTimeOut = true
			DPrintf("node %d follower Term timeout... go to new term and starting to elect", rf.me)
			rf.mu.Lock()
			rf.currentTerm++
			rf.mu.Unlock()
		case <-AENotifyChan:
			DPrintf("node %d :follower reset timer", rf.me)
			TermTimer.Reset(time.Millisecond * time.Duration(electionTimeOut))
		}
	}
}

//seed use to  generate the random election timeout
func mainProcess(rf *Raft, electionTimeOut int, applyCh chan ApplyMsg) {
	//initially compete for leadership
	//ok := false
	defer func() {
		if e := recover(); e != nil {
			DPrintf("main down %v", e)
		}
	}()
	rf.applyCh = applyCh
	rf.log = append(rf.log, commandTerm{0, 0})
	AENotifyChan = make(chan bool, len(rf.peers))
	peersNum := len(rf.peers)
	majority := int(float32(peersNum)/2 + 0.5)
	//most 256 command in the queue
	rf.commandToBeExec = make(chan *UnexecuteCommand, 256)
	for {
		//waiting for term end
		//if get AE &&rf is not leader
		isTimeOut := rf.CandidateProcess(electionTimeOut, majority)
		if isTimeOut {
			continue
		}

		if rf.me == rf.leaderNum {
			DPrintf("node %d go to leader", rf.me)
			leaderStatus := rf.LeaderProcess(electionTimeOut, applyCh)
			DPrintf("node %d after leader:status %d", rf.me, leaderStatus)
			//if receive AE with higher term turn to follower
			if leaderStatus == -1 {
				rf.Follower(electionTimeOut)
			}
		} else {
			DPrintf("node %d go to follower", rf.me)
			rf.Follower(electionTimeOut)
		}

	}
}

// the service or tester wants to create a Raft server. the ports
// of all the Raft servers (including this one) are in peers[]. this
// server's port is peers[me]. all the servers' peers[] arrays
// have the same order. persister is a place for this server to
// save its persistent state, and also initially holds the most
// recent saved state, if any. applyCh is a channel on which the
// tester or service expects Raft to send ApplyMsg messages.
// Make() must return quickly, so it should start goroutines
// for any long-running work.
//
func Make(peers []*labrpc.ClientEnd, me int,
	persister *Persister, applyCh chan ApplyMsg) *Raft {
	rf := &Raft{}
	rf.peers = peers
	rf.persister = persister
	rf.me = me
	rf.leaderNum = -1

	DPrintf("sizeof peers=%d", len(peers))
	DPrintf("%d: start...", me)

	// Your initialization code here (2A, 2B, 2C).
	rf.currentTerm = 0
	// initialize from state persisted before a crash
	rf.readPersist(persister.ReadRaftState())
	rand.Seed(time.Now().UnixNano())

	//Election timeout set to 256-512ms
	elecTimeOut := rand.Int()
	elecTimeOut = (elecTimeOut&255 + 256)
	DPrintf("node %d elecTimeOut :%d ", rf.me, elecTimeOut)
	go mainProcess(rf, elecTimeOut, applyCh)
	return rf
}
