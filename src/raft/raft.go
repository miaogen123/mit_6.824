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
	"labrpc"
	"math/rand"
	"sync"
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
}

//AppendEntry to send
type AppendEntry struct {
	Term         int
	LeaderID     int
	PrevLogIndex int

	PreLogTerm int
	Entries    []int

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
	var isleader bool
	// Your code here (2A).
	rf.mu.Lock()
	term = rf.currentTerm
	isleader = (rf.leaderNum == rf.me)
	rf.mu.Unlock()
	if isleader {
		DPrintf("node %d is leader", rf.me)
	}
	return term, isleader
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
	TermLength = 1400
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
	var voteFor int
	DPrintf("node %d currTerm:%d  candidateTerm:%d args.LastLogIndex:%d len(rf.log):%d", rf.me, currTerm, candidateTerm, args.LastLogIndex, len(rf.log))
	if rf.votedFor == -1 && currTerm < candidateTerm && args.LastLogIndex >= len(rf.log) {
		rf.votedFor = candidateID
		reply.VoteGranted = true
		voteFor = 1
	} else {
		reply.VoteGranted = false
		voteFor = 0
	}
	DPrintf("node %d get RV from %d voting %d", rf.me, args.CandidateID, voteFor)
}

//AppendEntry...
func (rf *Raft) AppendEntry(args *AppendEntry, reply *AppendEntryReply) {
	defer func() {
		AENotifyChan <- true
		if e := recover(); e != nil {
			DPrintf("node%d write to close channel")
		}
	}()
	DPrintf("node %d get AE imply leader %d ", rf.me, args.LeaderID)

	var curTerm int
	rf.mu.Lock()
	curTerm = rf.currentTerm
	rf.mu.Unlock()

	//1. Reply false if term < currentTerm (§5.1)
	reply.Me = rf.me
	if args.Term < curTerm {
		reply.Success = false
		return
	} else if args.Term > curTerm && rf.leaderNum == rf.me {
		rf.leaderNum = args.LeaderID
	}
	rf.currentTerm = args.Term
	//TODO:2. Reply false if log doesn’t contain an entry at prevLogIndex whose term matches prevLogTerm (§5.3)
	// existFlag := false
	// if len(rf.log) == 0 && args.PrevLogIndex == -1 {
	// 	existFlag = true
	// } else {
	// 	for _, oneLog := range rf.log {
	//
	// 	}
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
// even if the Raft instance has been killed,
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
	return index, term, isLeader
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
	rf.votedFor = -1
	requestVote := &RequestVoteArgs{
		Term:         rf.currentTerm,
		CandidateID:  rf.me,
		LastLogIndex: len(rf.log),
		LastLogTerm:  lastLogTerm,
	}
	granted := 0
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
				DPrintf("node %d started ", rf.me)
			} else {
				timer.Reset(time.Millisecond * time.Duration(electionTimeOut))
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
func (rf *Raft) LeaderProcess(electionTimeOut int) int {
	var TermTimer *time.Timer
	var HBTimer *time.Timer
	//more buf than it actually needs
	AEDistributedChan := make(chan bool, len(rf.peers))
	AEChan := make(chan *AppendEntryReply, len(rf.peers))
	//to first notify leader to send empty HB
	AEDistributedChan <- true

	istermTimeOut := false
	DPrintf("node %d in the leader process", rf.me)
	for !istermTimeOut {
		select {
		//HB timer
		case <-func() <-chan time.Time {
			if HBTimer == nil {
				HBTimer = time.NewTimer(time.Millisecond * time.Duration(HBInterval))
			} else {
				HBTimer.Reset(time.Millisecond * time.Duration(HBInterval))
			}
			//here send HB parket
			return HBTimer.C
		}():
			DPrintf("node %d send HB", rf.me)
			AEDistributedChan <- true
		case <-AEDistributedChan:
			//distributes AE(HB)
			DPrintf("node %d distributing AE", rf.me)
			appEn := &AppendEntry{}
			appEn.Term = rf.currentTerm
			appEn.LeaderID = rf.me
			appEn.PrevLogIndex = len(rf.log) - 1
			if len(rf.log) != 0 {
				appEn.PreLogTerm = rf.log[len(rf.log)-1].term
			}
			appEn.LeaderCommit = rf.commitIndex
			for ind := range rf.peers {
				if ind == rf.me {
					continue
				}
				go func(ind int) {
					defer func() {
						if e := recover(); e != nil {
							DPrintf("AE send routines down %v", e)
						}
					}()
					//three times try
					for count := 0; count < 3; count++ {
						appendEntryReply := &AppendEntryReply{}
						DPrintf("node %d try AE %d time to %d ", rf.me, count, ind)
						if rf.sendAppendEntry(ind, appEn, appendEntryReply) == true {
							//AEChan <- appendEntryReply
							break
						}
						DPrintf("TIME: %d node: %d  send AE to %d failed ", count, rf.me, ind)
					}
				}(ind)
			}
			//leader timer
		case <-AEChan:
			continue
		case <-func() <-chan time.Time {
			if TermTimer == nil {
				TermTimer = time.NewTimer(time.Millisecond * time.Duration(TermLength))
			} else {
				TermTimer.Reset(time.Millisecond * time.Duration(TermLength))
			}
			return TermTimer.C
		}():
			DPrintf("leader %d :Term timeout...go to new term and elect", rf.me)
			istermTimeOut = true
			break
		//operation when receive the AE notification of other
		case <-AENotifyChan:
			DPrintf("node %d receives AE from other", rf.me)
			if rf.me != rf.leaderNum {
				return -1
			}
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
			} else {
				TermTimer.Reset(time.Millisecond * time.Duration(electionTimeOut))
			}
			return TermTimer.C
		}():
			isTimeOut = true
			DPrintf("node %d :follower Term timeout... go to new term and starting to elect", rf.me)
		case <-AENotifyChan:
			DPrintf("node %d :follower reset timer", rf.me)
			TermTimer.Reset(time.Millisecond * time.Duration(electionTimeOut))
		}
	}
}

//seed use to  generate the random election timeout
func mainProcess(rf *Raft, electionTimeOut int) {
	//initially compete for leadership
	//ok := false
	defer func() {
		if e := recover(); e != nil {
			DPrintf("main down")
		}
	}()
	AENotifyChan = make(chan bool, len(rf.peers))
	peersNum := len(rf.peers)
	majority := int((peersNum - 1) / 2)
	for {
		//waiting for term end
		//if get AE &&rf is not leader
		isTimeOut := rf.CandidateProcess(electionTimeOut, majority)
		if isTimeOut {
			continue
		}

		if rf.me == rf.leaderNum {
			DPrintf("node %d go to leader", rf.me)
			leaderStatus := rf.LeaderProcess(electionTimeOut)
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
	go mainProcess(rf, elecTimeOut)
	return rf
}
