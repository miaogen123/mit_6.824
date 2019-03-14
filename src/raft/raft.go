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
	"bytes"
	"labgob"
	"labrpc"
	"math/rand"
	"sort"
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

//global varible
//var exitFlag bool

const (
	TermLength = 1000
	HBInterval = 100
)

type ApplyMsg struct {
	CommandValid bool
	Command      interface{}
	CommandIndex int
}

type CommandTerm struct {
	Command interface{}
	Term    int
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
	// all servers

	leaderNum   int
	currentTerm int
	votedFor    int
	log         []CommandTerm
	///volatile state on all servers
	commitIndex int
	lastApplied int

	///for leader
	///use to track the state of replica
	nextIndex  []int
	matchIndex []int
	//for DEBUG and exit
	exitFlag bool

	//the channel to get AE
	AENotifyChan chan bool
	//variables to sequencely execute the command
	commandToBeExec chan bool
	applyCh         chan ApplyMsg

	lastIndexInSnapshot int
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
	Me                    int
	Term                  int
	Success               bool
	LastLogIndex          int
	CommitIndex           int
	TermOfArgsPreLogIndex int
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

	//mycode
	w := new(bytes.Buffer)
	e := labgob.NewEncoder(w)
	e.Encode(rf.currentTerm)
	e.Encode(rf.log)
	e.Encode(rf.exitFlag)

	data := w.Bytes()
	rf.persister.SaveRaftState(data)
	DPrintf("node %d: persist successfully ", rf.me)

}

//the argu is
func (rf *Raft) Snapshots(ClientMaxRequestSeq map[int64]int) {
	w := new(bytes.Buffer)
	//e := labgob.NewEncoder(w)
	endIndex := rf.commitIndex - 1
	if endIndex < 1 {
		return
	}
	originBytes := rf.persister.ReadSnapshot()
	var prelog []CommandTerm
	if len(originBytes) > 0 {
		r := bytes.NewBuffer(originBytes)
		d := labgob.NewDecoder(r)
		if d.Decode(&prelog) != nil {
			panic("first phase read persist error ")
		}
		//OPTIMIZATION NEEDED: get the lastIndex from snapshot from preSnapshot and verify

	}
	//pay attention to the sequences of varibles
	//e.Encode()

	data := w.Bytes()
	rf.persister.SaveRaftState(data)
	DPrintf("node %d: snapshot successfully ", rf.me)

}

func (rf *Raft) GetRaftStateSize() int {
	return rf.persister.RaftStateSize()
}

//
// restore previously persisted state.
//
func (rf *Raft) readPersist(data []byte) {
	if data == nil || len(data) < 1 { // bootstrap without any state?
		return
	}
	// Your code here (2C).

	//my code
	r := bytes.NewBuffer(data)
	d := labgob.NewDecoder(r)
	var currentTerm int
	var log []CommandTerm
	var exitFlag bool

	if d.Decode(&currentTerm) != nil || d.Decode(&log) != nil {
		panic("first phase read persist error ")
	} else {
		rf.currentTerm = currentTerm
		rf.log = log
	}

	if d.Decode(&exitFlag) != nil {
		panic("second phase read persist error ")
	} else {
		rf.exitFlag = exitFlag
	}
	DPrintf("node %d end reading persist state", rf.me)
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

// example RequestVote RPC handler.
// TODO::this part need to be refracted
func (rf *Raft) RequestVote(args *RequestVoteArgs, reply *RequestVoteReply) {
	// Your code here (2A, 2B).
	candidateID := args.CandidateID
	candidateTerm := args.Term

	reply.VoteGranted = false
	reply.Term = rf.currentTerm
	var localLastTerm int
	if len(rf.log) > 1 {
		localLastTerm = rf.log[len(rf.log)-1].Term
	}
	DPrintf("node %d getRV currTerm:%d  candidateTerm:%d args.LastLogIndex:%d localLastLogIndex:%d argsLastTerm %d localLastTerm %d", rf.me, rf.currentTerm, candidateTerm, args.LastLogIndex, len(rf.log)-1, args.LastLogTerm, localLastTerm)
	if rf.currentTerm > candidateTerm || (rf.currentTerm == candidateTerm && rf.votedFor != rf.me) {
		DPrintf("node %d get RV from %d voting false in the test1", rf.me, args.CandidateID)
		return
	}
	if rf.currentTerm == candidateTerm && args.LastLogTerm <= localLastTerm {
		DPrintf("node %d get RV from %d voting false in the test2", rf.me, args.CandidateID)
		return
	}

	//if rf.currentTerm < candidateTerm && (args.LastLogIndex >= len(rf.log)-1 || args.LastLogTerm > rf.log[args.LastLogIndex].Term) {
	if rf.currentTerm < candidateTerm {
		rf.mu.Lock()
		// the if exist because many request may block before rf.mu.Lock() this if can prevent repeat vote
		if rf.currentTerm == candidateTerm {
			rf.mu.Unlock()
			return
		} else {
			rf.currentTerm = args.Term
			rf.mu.Unlock()
			rf.persist()
		}
		if args.LastLogTerm < rf.log[len(rf.log)-1].Term {
			DPrintf("node %d get RV from %d voting false in the test3", rf.me, args.CandidateID)
			return
		}
		if args.LastLogTerm == rf.log[len(rf.log)-1].Term && args.LastLogIndex < len(rf.log)-1 {
			DPrintf("node %d get RV from %d voting false in the test4", rf.me, args.CandidateID)
			return
		}
		rf.leaderNum = candidateID
	}
	reply.VoteGranted = true
	if len(rf.AENotifyChan) < 2 {
		rf.AENotifyChan <- true
	}
	DPrintf("node %d get RV from %d voting true", rf.me, args.CandidateID)
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
	}()

	DPrintf("TEST4: entries in append entries %v ", args.Entries)
	//1. Reply false if term < currentTerm (§5.1)
	curTerm := rf.currentTerm
	reply.Me = rf.me
	reply.Term = curTerm
	//	DPrintf("AE:node %d currTerm:%d  candidateTerm:%d args.LastLogIndex:%d len(rf.log):%d", rf.me, curTerm, candidateTerm, args.LastLogIndex, len(rf.log))
	DPrintf("node %d get AE imply leader %d | args.Term %d,  curTerm %d", rf.me, args.LeaderID, args.Term, curTerm)
	if args.Term < curTerm {
		reply.Success = false
		DPrintf("node %d AE returned false because of args.Term < curTerm", rf.me)
		return
	} else if args.Term > curTerm {
		if rf.leaderNum == rf.me {
			rf.mu.Lock()
			rf.leaderNum = args.LeaderID
			rf.currentTerm = args.Term
			rf.mu.Unlock()
			//quit from leader
		} else {
			rf.mu.Lock()
			rf.currentTerm = args.Term
			rf.mu.Unlock()
		}
		rf.persist()
		DPrintf("node %d AE: update leaderNum to %d and term to %d", rf.me, args.LeaderID, args.Term)
	}
	if len(rf.AENotifyChan) < 3 {
		rf.AENotifyChan <- true
	}
	DPrintf("node %d pass 1 |prevlogindex %d | curlastlogindex %d", rf.me, args.PrevLogIndex, len(rf.log))

	//TODO:2. Reply false if log doesn’t contain an entry at prevLogIndex whose term matches prevLogTerm (§5.3)
	existFlag := true
	if args.PrevLogIndex >= len(rf.log) {
		existFlag = false
		DPrintf("node %d:return false because of short", rf.me)
		reply.LastLogIndex = len(rf.log) - 1
		reply.TermOfArgsPreLogIndex = rf.log[len(rf.log)-1].Term
	} else if rf.log[args.PrevLogIndex].Term != args.PrevLogTerm {
		prevIndexTerm := rf.log[args.PrevLogIndex].Term
		DPrintf("node %d:index %d of leader has term %d but mine is %d", rf.me, args.PrevLogIndex, args.PrevLogTerm, prevIndexTerm)
		//	reply.CommitIndex = rf.commitIndex
		retTerm := prevIndexTerm
		if prevIndexTerm > args.PrevLogTerm {
			for i := args.PrevLogIndex; i > 0; i-- {
				if rf.log[i].Term < args.PrevLogTerm {
					retTerm = rf.log[i].Term
					break
				}
			}
		}
		reply.TermOfArgsPreLogIndex = retTerm
		existFlag = false
	}
	if !existFlag {
		reply.Success = false
		DPrintf("node %d returned because of existFlag=false ", rf.me)
		return
	}
	DPrintf("node %d pass 2| mycommit %d leadercommit %d |prevLogindex %d", rf.me, rf.commitIndex, args.LeaderCommit, args.PrevLogIndex)
	//if args.PrevLogIndex < rf.commitIndex && args.LeaderCommit != 0 {
	if args.PrevLogIndex < rf.commitIndex {
		reply.CommitIndex = rf.commitIndex
		reply.Success = true
		DPrintf("node %d return because of the prevlogindex < local commitindex", rf.me)
		return
		//DPrintf("ERROR " + strconv.Itoa(rf.me) + " args.PrevLogIndex < rf.commitIndex ")
	}
	var preCommitIndex int
	var aftCommitIndex int
	if args.LeaderCommit > rf.commitIndex {
		if args.LeaderCommit > args.PrevLogIndex {
			preCommitIndex = rf.commitIndex
			//rf.mu.Unlock()
			aftCommitIndex = args.PrevLogIndex
		} else {
			//rf.mu.Lock()
			preCommitIndex = rf.commitIndex
			aftCommitIndex = args.LeaderCommit
			//rf.mu.Unlock()
		}
		rf.persist()
	} else if args.LeaderCommit < rf.commitIndex && args.LeaderCommit != 0 {
		DPrintf("node %d args.LeaderCommit <commitIndex", rf.me)
	}
	DPrintf("node %d pass 5", rf.me)
	//nil implies the HB packet
	for ind := range rf.log[preCommitIndex:aftCommitIndex] {
		var applyMsg ApplyMsg
		applyMsg.CommandIndex = ind + preCommitIndex + 1
		rf.mu.Lock()
		applyMsg.Command = rf.log[ind+preCommitIndex+1].Command
		rf.mu.Unlock()
		applyMsg.CommandValid = true
		DPrintf("node %d: commit com at %d command %v", rf.me, applyMsg.CommandIndex, applyMsg.Command)
		rf.applyCh <- applyMsg
	}
	rf.mu.Lock()
	if aftCommitIndex > rf.commitIndex {
		rf.commitIndex = aftCommitIndex
	}
	rf.mu.Unlock()
	rf.persist()
	if args.Entries == nil {
		reply.Success = true
		DPrintf("node %d returned because of args.Entries==nil ", rf.me)
		return
	}
	//3. If an existing entry conflicts with a new one (same index but different terms), delete the existing entry and all that follow it (§5.3)
	startInd := args.PrevLogIndex + 1
	rf.mu.Lock()
	for _, oneLog := range args.Entries {
		if startInd >= len(rf.log) {
			rf.log = append(rf.log, oneLog)
		} else {
			if rf.log[startInd].Term != oneLog.Term {
				rf.log[startInd].Term = oneLog.Term
				rf.log[startInd].Command = oneLog.Command
			}
		}
		DPrintf("node %d startInd %d , len(rf.log) %d, term %d, command %v", rf.me, startInd, len(rf.log), rf.log[startInd].Term, rf.log[startInd].Command)
		startInd++
	}
	if startInd < len(rf.log)-1 && rf.log[startInd-1].Term > rf.log[len(rf.log)-1].Term {
		rf.log = rf.log[:startInd]
	}
	rf.mu.Unlock()
	//	if startInd < len(rf.log)-1 {
	//		rf.mu.Lock()
	//		rf.log = rf.log[:startInd]
	//		rf.mu.Unlock()
	//	}
	rf.persist()
	DPrintf("node %d pass all", rf.me)
	reply.Success = true
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
	isLeader := true
	// Your code here (2B).
	DPrintf("node %d receive command value %v", rf.me, command)
	rf.mu.Lock()
	leaderNum := rf.leaderNum
	curTerm := rf.currentTerm
	rf.mu.Unlock()
	if leaderNum != rf.me {
		isLeader = false
		return index, curTerm, isLeader
	}
	DPrintf("node %d  waiting %v", rf.me, command)
	rf.commandToBeExec <- true
	rf.mu.Lock()
	rf.log = append(rf.log, CommandTerm{command, curTerm})
	index = len(rf.log) - 1
	rf.mu.Unlock()
	rf.matchIndex[rf.me] = index
	rf.persist()
	DPrintf("node %d finished waiting index %d command %v", rf.me, index, command)
	return index, curTerm, isLeader
}

//GetLeader: for upper service to get current leader number
func (rf *Raft) GetLeaderNum() int {
	return rf.leaderNum
}

//
// the tester calls Kill() when a Raft instance won't
// be needed again. you are not required to do anything
// in Kill(), but it might be convenient to (for example)
// turn off debug output from this instance.
//
func (rf *Raft) Kill() {
	// Your code here, if desired.
	rf.exitFlag = true
	rf.persist()
	DPrintf("node %d exited", rf.me)
}

func (rf *Raft) CandidateProcess(electionTimeOut, majority int) bool {
	//startT := time.Now()
	//这个channel地方就这么写了先
	RVchan := make(chan *RequestVoteReply, len(rf.peers))
	DPrintf("candidate %d TERM: %d len of rf.log %d ", rf.me, rf.currentTerm, len(rf.log))
	requestVote := &RequestVoteArgs{
		Term:         rf.currentTerm,
		CandidateID:  rf.me,
		LastLogIndex: len(rf.log) - 1,
		LastLogTerm:  rf.log[len(rf.log)-1].Term,
	}
	granted := 1
	rf.votedFor = rf.me
	rf.leaderNum = -1
	for ind := range rf.peers {
		if ind == rf.me {
			continue
		}
		go func(ind int) {
			defer func() {
				if e := recover(); e != nil {
					DPrintf("candidate %d send routines to %d down %v", rf.me, ind, e)
				}
			}()
			//three times try
			for count := 0; count < 3; count++ {
				requestVoteReply := &RequestVoteReply{}
				DPrintf("candidate %d try RV %d time to %d ", rf.me, count, ind)
				//check whether i'm a candidate
				if rf.votedFor != rf.me {
					return
				}
				if rf.sendRequestVote(ind, requestVote, requestVoteReply) == true {
					RVchan <- requestVoteReply
					break
				}
				DPrintf("candidate: %d  send RV  to %d failed %d time", rf.me, ind, count)
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
					DPrintf("candidate %d majority %d", rf.me, majority)
					rf.leaderNum = rf.me
					DPrintf("candidate %d being leader", rf.me)
					return false
				}
			}
			if pReply.Term > rf.currentTerm {
				DPrintf("candidate %d update term to %d", rf.me, pReply.Term)
				rf.mu.Lock()
				rf.currentTerm = pReply.Term
				rf.mu.Unlock()
				rf.persist()
				return false
			}
		//listen the signal of timeout
		case <-func() <-chan time.Time {
			if timer == nil {
				timer = time.NewTimer(time.Millisecond * time.Duration(electionTimeOut))
				DPrintf("candidate %d started ", rf.me)
			}
			return timer.C
		}():
			DPrintf("candidate %d time out", rf.me)
			isTimeOut = true
			rf.currentTerm++
			rf.persist()
			close(RVchan)
		//listen the signal of AE
		case <-rf.AENotifyChan:
			DPrintf("candidate %d in the AENotifyChan", rf.me)
			//remain_element := len(rf.AENotifyChan) - 2
			//DPrintf("candidate %d AENotifyChan has %d eles", rf.me, len(rf.AENotifyChan))
			for len(rf.AENotifyChan) > 0 {
				<-rf.AENotifyChan
			}
			//DPrintf("candidate %d AENotifyChan has %d eles", rf.me, len(rf.AENotifyChan))
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
			DPrintf("leader  %d error:%v", rf.me, e)
		}
	}()
	var TermTimer *time.Timer
	var HBTimer *time.Timer
	//more buf than it actually needs
	AEDistributedChan := make(chan bool, len(rf.peers))
	//to first notify leader to send empty HB
	AEDistributedChan <- true
	DPrintf("leader %d in the leader process  ", rf.me)
	//rf.log = append(rf.log, CommandTerm{-1, rf.currentTerm})
	nextInd := len(rf.log)
	rf.nextIndex = make([]int, len(rf.peers))
	for ind, _ := range rf.nextIndex {
		rf.nextIndex[ind] = nextInd
	}
	rf.matchIndex = make([]int, len(rf.peers))
	//command id
	var state uint32
	istermTimeOut := false
	//no use
	commandIdList := make([]uint32, 256)
	type TermIndex struct {
		Term            int
		LastIndexOfTerm int
	}
	IndexIte := nextInd - 1
	var termList []TermIndex
	curLastTerm := rf.log[IndexIte].Term + 1
	//get cache of term
	t1 := time.Now()
	for IndexIte >= 0 {
		if rf.log[IndexIte].Term != curLastTerm {
			termList = append(termList, TermIndex{rf.log[IndexIte].Term, IndexIte})
			curLastTerm = rf.log[IndexIte].Term
		}
		if rf.log[IndexIte].Term > curLastTerm {
			DPrintf("leader %d: ATTENTION term %d comand %v ", curLastTerm, rf.log[IndexIte-1].Command)
		}
		IndexIte--
	}
	DPrintf("termList: %v ", termList)
	// for ind := range termList {
	// 	DPrintf("(%d %d )", termList[ind].Term, termList[ind].LastIndexOfTerm)
	// }
	t2 := time.Since(t1)
	DPrintf("leader %d: create index cost  %v", rf.me, t2)
	commIndtoState := make(map[int]int)
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
			AEDistributedChan <- true
		case <-AEDistributedChan:
			//distributes AE(HB)
			DPrintf("leader %d distributing AE", rf.me)
			//remember the pre appendentry
			logLen := len(rf.log)
			if rf.nextIndex[rf.me] == logLen {
				DPrintf("leader %d command==nil", rf.me)
			} else {
				DPrintf("leader %d has the command %v", rf.me, rf.log[rf.nextIndex[rf.me]:])
				if _, ok := commIndtoState[logLen-1]; !ok {
					atomic.AddUint32(&state, 1)
					commIndtoState[logLen-1] = int(atomic.LoadUint32(&state))
				}
			}
			//majority := uint32(float32(len(rf.peers)-1)/2 + 0.5)
			for ind := range rf.peers {
				if ind == rf.me {
					continue
				}
				appEn := &AppendEntry{}
				appEnTerm := rf.currentTerm
				appEn.Term = appEnTerm
				appEn.LeaderID = rf.me
				appEn.LeaderCommit = rf.commitIndex
				currNextInd := rf.nextIndex[ind]
				if rf.nextIndex[ind] < logLen {
					appEn.Entries = append(appEn.Entries, rf.log[currNextInd:]...)
					//appEn.Entries = append(appEn.Entries, CommandTerm{123, 456})
					DPrintf("TEST4: entries %v ", appEn.Entries)
				}
				appEn.PrevLogIndex = currNextInd - 1
				prevLogIndex := appEn.PrevLogIndex
				appEn.PrevLogTerm = rf.log[appEn.PrevLogIndex].Term
				DPrintf("DEBUG:2 node %d appEn.PrevlogIndex %d len rf.log %d", ind, appEn.PrevLogIndex, len(rf.log))
				//DPrintf("node %d before the AE send goroutine %d", rf.me, ind)
				go func(ind int, commState uint32, prevLogIndex int) {
					defer func() {
						if e := recover(); e != nil {
							DPrintf("leader %d send AE to %d routines down :%v", rf.me, ind, e)
						}
					}()
					var appEnPointer *AppendEntry
					appEnPointer = appEn
					for count := 0; ; {
						appendEntryReply := &AppendEntryReply{}
						DPrintf("leader %d try AE %d time to %d ", rf.me, count, ind)
						if rf.leaderNum != rf.me {
							DPrintf("leader %d : node %d is leader ", rf.me, rf.leaderNum)
							if len(rf.AENotifyChan) < 2 {
								rf.AENotifyChan <- true
							}
							return
						}
						if rf.sendAppendEntry(ind, appEnPointer, appendEntryReply) == true {
							if appendEntryReply.Success {
								if len(appEnPointer.Entries) == 0 || appEnPointer.Entries == nil {
									DPrintf("node %d react to the HB successfully", appendEntryReply.Me)
									return
								}
								if appendEntryReply.CommitIndex == 0 {
									if rf.nextIndex[ind] < appEnPointer.PrevLogIndex+len(appEnPointer.Entries)+1 {
										rf.nextIndex[ind] = appEnPointer.PrevLogIndex + len(appEnPointer.Entries) + 1
									}
									if rf.matchIndex[ind] != rf.nextIndex[ind]-1 {
										rf.matchIndex[ind] = rf.nextIndex[ind] - 1
										atomic.AddUint32(&commandIdList[commState], 1)
									} else {
										DPrintf("leader %d: node %d  gone cause index has been processed |AE prevlogindex %v at term %d matchindex %d term%d ", rf.me, appendEntryReply.Me, appEnPointer.PrevLogIndex, rf.currentTerm, rf.matchIndex[ind], rf.log[rf.matchIndex[ind]].Term)
										return
									}
								}
								currComInd := rf.matchIndex[ind]
								DPrintf("leader %d: node %d success get one command %v AE prevlogindex %v at term %d", rf.me, appendEntryReply.Me, appEnPointer.Entries, appEnPointer.PrevLogIndex, rf.currentTerm)
								DPrintf("leader %d: node %d PrevLogIndex %d, should be %d |matchIndex %d ", rf.me, ind, appEnPointer.PrevLogIndex, prevLogIndex, rf.matchIndex[ind])
								if currComInd < prevLogIndex {
									var appEnTmp AppendEntry
									appEnTmp.LeaderID = appEnPointer.LeaderID
									appEnTerm = rf.currentTerm
									appEnTmp.Term = appEnTerm
									if appendEntryReply.CommitIndex != 0 {
										DPrintf("leader %d: node %d because commit out to PrevLogIndex %d", rf.me, ind, appEnTmp.PrevLogIndex)
										//appEnTmp.PrevLogIndex = appEnTmp.PrevLogIndex + 1
										appEnTmp.PrevLogIndex = appendEntryReply.CommitIndex
										if appEnTmp.PrevLogIndex+1 >= len(rf.log) {
											return
										}
										appEnTmp.Entries = append(appEnTmp.Entries, rf.log[appEnTmp.PrevLogIndex+1:appEnTmp.PrevLogIndex+2]...)
									} else {
										appEnTmp.PrevLogIndex = currComInd
										appEnTmp.Entries = append(appEnTmp.Entries, rf.log[appEnTmp.PrevLogIndex+1:]...)
									}
									appEnTmp.PrevLogTerm = rf.log[appEnTmp.PrevLogIndex].Term
									appEnPointer = &appEnTmp
									DPrintf("leader %d send AE to %d increment to preIndex %d lenof(entris) %d", rf.me, ind, appEnTmp.PrevLogIndex, len(appEnTmp.Entries))
									appEnTmp.LeaderCommit = rf.commitIndex
									continue
								}
								//var upComCount uint32
								if currComInd <= rf.commitIndex {
									DPrintf("leader %d node  %d index %d already committed ", rf.me, ind, currComInd)
									return
								}
								tmpMatchList := make([]int, len(rf.matchIndex))
								DPrintf("leader %d matchindex %v", rf.me, rf.matchIndex)
								copy(tmpMatchList, rf.matchIndex[:])
								sort.Ints(tmpMatchList)
								DPrintf("leader %d matchindex %v", rf.me, rf.matchIndex)
								DPrintf("leader %d sorted matchindex %v", rf.me, tmpMatchList)
								toBeCommitIndex := tmpMatchList[int(len(rf.matchIndex)/2)]
								rf.mu.Lock()
								if toBeCommitIndex <= rf.commitIndex {
									DPrintf("leader %d node  %d toBeCommitIndex %d already committed ", rf.me, ind, toBeCommitIndex)
									rf.mu.Unlock()
									return
								}
								rf.mu.Unlock()
								currComInd = toBeCommitIndex
								DPrintf("leader %d start currComInd %d rf.commitIndex %d", rf.me, currComInd, rf.commitIndex)
								if currComInd < rf.commitIndex {
									panic("ERROR: currComInd<rf.commitIndex ")
								}
								if rf.leaderNum != rf.me {
									DPrintf("leader %d node %d is leader", rf.me, rf.leaderNum)
									return
								}
								for i := rf.commitIndex + 1; i <= currComInd; i++ {
									var applyMsg ApplyMsg
									applyMsg.Command = rf.log[i].Command
									applyMsg.CommandIndex = i
									applyMsg.CommandValid = true
									applyCh <- applyMsg
									rf.mu.Lock()
									if i > rf.commitIndex {
										rf.commitIndex = i
									}
									rf.mu.Unlock()
									DPrintf("leader %d: commit com at %d of term %d command %v term %d", rf.me, i, rf.log[i].Term, rf.log[i].Command, rf.log[i].Term)
								}
								DPrintf("leader %d AE send to %d end", rf.me, ind)
								return
							} else {
								DPrintf("leader %d: appendEntryReply.Term %d rf.currentTerm %d reply.CommitIndex %d rf.commitIndex %d", rf.me, appendEntryReply.Term, rf.currentTerm, appendEntryReply.CommitIndex, rf.commitIndex)
								if appendEntryReply.Term > appEnTerm { //|| appendEntryReply.CommitIndex > rf.commitIndex {
									DPrintf("leader %d: go to follower because of term or commitIndex", rf.me)
									rf.leaderNum = -1
									if len(rf.AENotifyChan) < 2 {
										rf.AENotifyChan <- true
									}
									return
								}
								//already get this command so just return
								if appEnPointer.PrevLogIndex <= rf.matchIndex[ind] {
									DPrintf("INFO: leader %d to node %d AE: prevLogIndex %d matchIndex %d", rf.me, ind, appEnPointer.PrevLogIndex, rf.matchIndex[ind])
									return
								}
								prevlog := appEnPointer.PrevLogIndex - 1
								var appEnTmp AppendEntry
								appEnTmp.LeaderID = appEnPointer.LeaderID
								if appendEntryReply.LastLogIndex != 0 && prevlog > appendEntryReply.LastLogIndex {
									prevlog = appendEntryReply.LastLogIndex
								}
								lastTerm := appendEntryReply.TermOfArgsPreLogIndex
								if lastTerm != 0 {
									index := 0
									DPrintf("leader %d term from reply %d lastTerm of node %d\n", rf.me, lastTerm, appendEntryReply.Me)
									for index < len(termList)-1 && termList[index].Term > lastTerm {
										index++
									}
									if termList[index].Term <= lastTerm && termList[index].LastIndexOfTerm < prevlog {
										prevlog = termList[index].LastIndexOfTerm
										DPrintf("leader %d to node %d prevlogIndex go to %d |in termList index %d termList[ind] %d", rf.me, ind, prevlog, index, termList[index].Term)
									}
								}
								// if appendEntryReply.CommitIndex != 0 && prevlog > appendEntryReply.CommitIndex {
								// 	prevlog = appendEntryReply.CommitIndex
								// }
								rf.nextIndex[ind] = prevlog + 1
								if prevlog < 0 {
									DPrintf("leader %d prevlogindex becomes <0", rf.me)
								}
								appEnTmp.PrevLogIndex = prevlog
								appEnTmp.Entries = append(appEnTmp.Entries, rf.log[prevlog+1])
								appEnTmp.Term = rf.currentTerm
								appEnTmp.PrevLogTerm = rf.log[prevlog].Term
								appEnPointer = &appEnTmp
								DPrintf("leader %d go to decrement the %d's log to prelog %d  command Term:%d   count %d", rf.me, appendEntryReply.Me, prevlog, rf.log[prevlog].Term, count)
							}
						} else {
							DPrintf("leader: %d  send AE to %d failed the %d time", rf.me, ind, count)
							count++
							// if count >= 3 {
							// return
							// }
						}
					}
				}(ind, uint32(commIndtoState[logLen-1]), prevLogIndex)
			}
			HBTimer.Reset(time.Millisecond * time.Duration(HBInterval))
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
			rf.persist()
			TermTimer.Reset(time.Millisecond * time.Duration(TermLength))
			break
		//operation when receive the AE notification of other
		case <-rf.AENotifyChan:
			for len(rf.AENotifyChan)-2 > 0 {
				<-rf.AENotifyChan
			}
			if rf.me != rf.leaderNum {
				DPrintf("leader %d return to follower because  receiving AE from other or internal ", rf.me)
				return -1
			}
			DPrintf("leader %d receives AE from other", rf.me)
		case commTBE := <-rf.commandToBeExec:
			for len(AEDistributedChan)-1 > 0 {
				<-AEDistributedChan
			}
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
			rf.leaderNum = -1
			rf.mu.Unlock()
			rf.persist()
		case <-rf.AENotifyChan:
			for len(rf.AENotifyChan)-1 > 0 {
				<-rf.AENotifyChan
			}
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
			DPrintf("node  %d main down %v", rf.me, e)
		}
		DPrintf("node %d mainProcess exit", rf.me)
	}()
	peersNum := len(rf.peers)
	majority := int(float32(peersNum)/2 + 0.5)
	for {
		if rf.exitFlag == true {
			rf.leaderNum = -1
			DPrintf("node %d end process   ", rf.me)
			return
		}
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
	rf.votedFor = -1
	DPrintf("me %d: start... sizeof peers=%d", me, len(peers))

	// Your initialization code here (2A, 2B, 2C).
	rf.currentTerm = 0
	// initialize from state persisted before a crash
	rf.readPersist(persister.ReadRaftState())

	rf.applyCh = applyCh
	if len(rf.log) < 1 {
		rf.log = append(rf.log, CommandTerm{nil, 0})
	}
	rf.commitIndex = 0
	rf.lastApplied = 0
	rf.AENotifyChan = make(chan bool, len(rf.peers))
	//most 256 command in the queue
	rf.commandToBeExec = make(chan bool, 256)

	rand.Seed(time.Now().UnixNano())
	//Election timeout set to 256-512ms
	elecTimeOut := rand.Int()
	elecTimeOut = (elecTimeOut&255 + 256)
	DPrintf("node %d elecTimeOut :%d ", rf.me, elecTimeOut)
	go mainProcess(rf, elecTimeOut, applyCh)
	return rf
}
