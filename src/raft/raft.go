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
	"fmt"
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
	CommandValid        bool
	Command             interface{}
	CommandIndex        int
	ClientMaxRequestSeq map[int32]int
	Rsm                 map[string]string
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
	logRWLock   sync.RWMutex
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
	lastTermInSnapshot  int
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

type InstallSnapShot struct {
	Term              int
	LeaderId          int
	LastIncludedIndex int
	LastIncludedTerm  int
	Data              []byte
}

type TermIndex struct {
	Term            int
	LastIndexOfTerm int
}
type InstallSnapShotReply struct {
	Success  bool
	Term     int
	Err_code int
	Value    int
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

func (rf *Raft) getLastLogIndex() int {
	return rf.lastIndexInSnapshot + len(rf.log) - 1
}
func (rf *Raft) GetRaftStateSize() int {
	return rf.persister.RaftStateSize()
}
func (rf *Raft) getTrueIndex(index int) int {
	if index-rf.lastIndexInSnapshot < 0 {
		return 0
	}
	//	if index-rf.lastIndexInSnapshot > len(rf.log)-1 {
	//		return len(rf.log) - 1
	//	}
	return index - rf.lastIndexInSnapshot
}
func (rf *Raft) getNameIndex(index int) int {
	return index + rf.lastIndexInSnapshot
}

func (rf *Raft) truncate(endIndex, preIndex int, lastTerm int) []CommandTerm {
	var newLog []CommandTerm
	if endIndex <= preIndex {
		panic(fmt.Sprintf("node %d:endIndex %d <= rf.lastIndexInSnapshot %d", rf.me, endIndex, preIndex))
	}
	DPrintf("node %d:be len new %d len(log) %d endindex %d", rf.me, len(newLog), len(rf.log), endIndex)
	for i := endIndex - preIndex; i < len(rf.log); i++ {
		newLog = append(newLog, rf.log[i])
	}
	if len(newLog) == 0 {
		newLog = append(newLog, CommandTerm{nil, lastTerm})
	}
	DPrintf("node %d:af len new %d len(log) %d endindex %d", rf.me, len(newLog), len(rf.log), endIndex)
	return newLog
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
	//	e.Encode(rf.lastIndexInSnapshot)

	data := w.Bytes()
	rf.persister.SaveRaftState(data)
	DPrintf("node %d: persist successfully ", rf.me)
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
	//var lastIndexInSnapshot int

	if d.Decode(&currentTerm) != nil || d.Decode(&log) != nil {
		panic("first phase read persist error ")
	} else {
		rf.currentTerm = currentTerm
		rf.log = log
	}

	//if d.Decode(&lastIndexInSnapshot) != nil {
	//	panic("second phase read persist error ")
	//} else {
	//	rf.lastIndexInSnapshot = lastIndexInSnapshot
	//}
	DPrintf("node %d end reading persist state", rf.me)
}

//the argu is
func (rf *Raft) TakeSnapshots(ClientMaxRequestSeq map[int32]int, rsm map[string]string, index int, lock *sync.Mutex) {
	endIndex := index - 1
	if endIndex < 1 || endIndex <= rf.lastIndexInSnapshot {
		DPrintf("node %d: endIndex<1 or < lastIndexInSnapshot return ", rf.me)
		return
	}
	rf.mu.Lock()
	if endIndex < 1 || endIndex <= rf.lastIndexInSnapshot {
		rf.mu.Unlock()
		DPrintf("node %d: endIndex<1 or < lastIndexInSnapshot return ", rf.me)
		return
	}
	defer rf.mu.Unlock()
	trueEndIndex := rf.getTrueIndex(endIndex)
	DPrintf("node %d: snapshotting|trueIndex %d endIndex %d len(log)", rf.me, trueEndIndex, endIndex, len(rf.log))
	//save snapshot
	//pay attention to the sequences of varibles
	endIndexTerm := rf.log[trueEndIndex].Term
	w := new(bytes.Buffer)
	e := labgob.NewEncoder(w)
	lock.Lock()
	DPrintf("node %d:before snapshot endIndex %d commitIndex %d len(log) %d lastInSnapshot %d seq %v", rf.me, endIndex, trueEndIndex, rf.commitIndex, len(rf.log), rf.lastIndexInSnapshot, ClientMaxRequestSeq)
	e.Encode(ClientMaxRequestSeq)
	e.Encode(rsm)
	e.Encode(endIndex)
	DPrintf("node %d write lastIndex snapshot %d", rf.me, endIndex)
	e.Encode(endIndexTerm)
	lock.Unlock()
	data := w.Bytes()
	preIndex := rf.lastIndexInSnapshot
	if endIndex > preIndex {
		rf.lastTermInSnapshot = endIndexTerm
		rf.lastIndexInSnapshot = endIndex
		rf.log = rf.truncate(rf.lastIndexInSnapshot, preIndex, endIndexTerm)
	} else {
		DPrintf("node %d endindex %d already snapshotted ", rf.me, endIndex)
		return
	}
	//rf.log[0].Term = rf.lastTermInSnapshot

	w2 := new(bytes.Buffer)
	e2 := labgob.NewEncoder(w2)
	e2.Encode(rf.currentTerm)
	e2.Encode(rf.log)
	e2.Encode(rf.lastIndexInSnapshot)
	data2 := w2.Bytes()
	DPrintf("node %d: before raft state size %d ", rf.me, rf.persister.RaftStateSize())
	rf.persister.SaveStateAndSnapshot(data2, data)
	DPrintf("node %d: end raft state size %d ", rf.me, rf.persister.RaftStateSize())
	DPrintf("node %d: snapshot successfully | now endIndex:%d, rf.lastIndexInSnap:%d len(log):%d", rf.me, endIndex, rf.lastIndexInSnapshot, len(rf.log))
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
	if len(rf.log) >= 1 {
		localLastTerm = rf.log[len(rf.log)-1].Term
	}
	DPrintf("node %d getRV currTerm:%d candidateTerm:%d args.LastLogIndex:%d localLastLogIndex:%d argsLastTerm %d localLastTerm %d", rf.me, rf.currentTerm, candidateTerm, args.LastLogIndex, rf.getLastLogIndex(), args.LastLogTerm, localLastTerm)
	if rf.currentTerm > candidateTerm || (rf.currentTerm == candidateTerm && rf.votedFor != rf.me) {
		DPrintf("node %d get RV from %d voting false in the test1", rf.me, args.CandidateID)
		return
	}
	if rf.currentTerm == candidateTerm && args.LastLogTerm <= localLastTerm {
		DPrintf("node %d get RV from %d voting false in the test2", rf.me, args.CandidateID)
		return
	}

	if rf.currentTerm < candidateTerm {
		rf.mu.Lock()
		// the if exist because many request may block before rf.mu.Lock() this if can prevent repeat vote
		//DCL
		if rf.currentTerm == candidateTerm {
			rf.mu.Unlock()
			return
		} else {
			rf.currentTerm = args.Term
			rf.mu.Unlock()
			rf.persist()
		}
		DPrintf("TEST5: node %d :term %d, len(rf.log) %d, term %d", rf.me, args.LastLogTerm, len(rf.log), rf.log[len(rf.log)-1].Term)
		if args.LastLogTerm < rf.log[len(rf.log)-1].Term {
			DPrintf("node %d get RV from %d voting false in the test3", rf.me, args.CandidateID)
			return
		}
		if args.LastLogTerm == rf.log[len(rf.log)-1].Term && args.LastLogIndex < rf.getLastLogIndex() {
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

	//	defer func() {
	//		if e := recover(); e != nil {
	//			DPrintf("node %d error value is %v", rf.me, e)
	//		}
	//		if reply.Success {
	//			DPrintf("node %d get command %v from leader %d", rf.me, args.Entries, args.LeaderID)
	//		}
	//	}()
	DPrintf("TEST4:node %d entries in append entries %v ", rf.me, args.Entries)
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
	DPrintf("node %d pass 1 |prevlogindex %d | curlastlogindex %d", rf.me, args.PrevLogIndex, rf.getLastLogIndex())

	//TODO:2. Reply false if log doesn’t contain an entry at prevLogIndex whose term matches prevLogTerm (§5.3)
	existFlag := true
	if args.PrevLogIndex >= rf.getLastLogIndex()+1 {
		existFlag = false
		DPrintf("node %d:return false because of short", rf.me)
		reply.LastLogIndex = rf.getLastLogIndex()
		reply.TermOfArgsPreLogIndex = rf.log[len(rf.log)-1].Term
		return
	} else if rf.log[rf.getTrueIndex(args.PrevLogIndex)].Term != args.PrevLogTerm {
		prevIndexTerm := rf.log[rf.getTrueIndex(args.PrevLogIndex)].Term
		DPrintf("node %d:index %d of leader has term %d but mine is %d", rf.me, args.PrevLogIndex, args.PrevLogTerm, prevIndexTerm)
		//	reply.CommitIndex = rf.commitIndex
		retTerm := prevIndexTerm
		if prevIndexTerm > args.PrevLogTerm {
			for i := rf.getTrueIndex(args.PrevLogIndex); i > 0; i-- {
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
	} else if args.LeaderCommit < rf.commitIndex && args.LeaderCommit != 0 {
		DPrintf("node %d args.LeaderCommit <commitIndex", rf.me)
	}
	DPrintf("node %d pass 5", rf.me)
	//nil implies the HB packet
	DPrintf("node %d:precomInd %d aftcomInd %d ", rf.me, preCommitIndex, aftCommitIndex)
	rf.mu.Lock()
	preCommitIndex = rf.getTrueIndex(preCommitIndex)
	aftCommitIndex = rf.getTrueIndex(aftCommitIndex)
	if aftCommitIndex >= len(rf.log) {
		aftCommitIndex = preCommitIndex
	}
	DPrintf("node %d:precomInd %d aftcomInd %d len(rf.log) %d", rf.me, preCommitIndex, aftCommitIndex, len(rf.log))
	var applyMsgList []ApplyMsg
	for ind := range rf.log[preCommitIndex:aftCommitIndex] {
		var applyMsg ApplyMsg
		applyMsg.CommandIndex = rf.getNameIndex(ind + preCommitIndex + 1)
		applyMsg.Command = rf.log[rf.getTrueIndex(applyMsg.CommandIndex)].Command
		applyMsg.CommandValid = true
		if applyMsg.CommandIndex > rf.commitIndex {
			DPrintf("node %d commindex %d trueindex %d len(rf.log) %d", rf.me, applyMsg.CommandIndex, rf.getTrueIndex(applyMsg.CommandIndex), len(rf.log))
			rf.commitIndex = applyMsg.CommandIndex
			applyMsgList = append(applyMsgList, applyMsg)
			DPrintf("node %d: commit com at %d command %v", rf.me, applyMsg.CommandIndex, applyMsg.Command)
		}
	}
	for _, applyMsg := range applyMsgList {
		rf.applyCh <- applyMsg
	}
	rf.mu.Unlock()
	rf.persist()
	if args.Entries == nil {
		reply.Success = true
		DPrintf("node %d returned because of args.Entries==nil ", rf.me)
		return
	}
	//3. If an existing entry conflicts with a new one (same index but different terms), delete the existing entry and all that follow it (§5.3)
	rf.mu.Lock()
	DPrintf("node %d pre %d , len(rf.log) %d, snapshot %d ", rf.me, args.PrevLogIndex, len(rf.log), rf.lastIndexInSnapshot)
	startInd := rf.getTrueIndex(args.PrevLogIndex + 1)
	DPrintf("node %d start %d", rf.me, startInd)
	if startInd >= 1 && startInd <= len(rf.log) {
		for _, oneLog := range args.Entries {
			//avoid override the past log
			if rf.getNameIndex(startInd) <= rf.commitIndex {
				break
			}
			if startInd >= len(rf.log) {
				rf.log = append(rf.log, oneLog)
			} else {
				//if rf.log[startInd].Term != oneLog.Term {
				rf.log[startInd].Term = oneLog.Term
				rf.log[startInd].Command = oneLog.Command
				//}
			}
			if oneLog.Command == nil {
				panic(fmt.Sprintf("node %d comm is nil ", rf.me))
			}
			DPrintf("node %d startInd %d , len(rf.log) %d, command in entries %v", rf.me, rf.getNameIndex(startInd), len(rf.log), oneLog.Command)
			DPrintf(" term %d, command %v", rf.log[startInd].Term, rf.log[startInd].Command)
			startInd++
		}
		if startInd < len(rf.log)-1 && rf.log[startInd-1].Term > rf.log[len(rf.log)-1].Term {
			rf.log = rf.log[:startInd]
		}
	}
	rf.mu.Unlock()
	rf.persist()
	DPrintf("node %d pass all", rf.me)
	reply.Success = true
}

func (rf *Raft) InstallSnapShot(args *InstallSnapShot, reply *InstallSnapShotReply) {
	reply.Term = rf.currentTerm
	reply.Success = true
	if args.Term < rf.currentTerm {
		DPrintf("node %d: when install snapshot return because small term", rf.me)
		reply.Success = false
		reply.Err_code = 1
		return
	}
	if args.LastIncludedIndex <= rf.lastIndexInSnapshot {
		DPrintf("node %d: when install snapshot return because the lastIncludedIndex has been snapshotted", rf.me)
		DPrintf("node %d: args.LastIncludedIndex %d rf.lastIndex %d", rf.me, args.LastIncludedIndex, rf.lastIndexInSnapshot)
		reply.Success = false
		reply.Err_code = 2
		reply.Value = rf.lastIndexInSnapshot
		return
	}
	if rf.getLastLogIndex() >= args.LastIncludedIndex {
		if rf.log[rf.getTrueIndex(args.LastIncludedIndex)].Term == args.LastIncludedTerm {
			DPrintf("node %d: when install snapshot return because we have same last Index and Term ", rf.me)
			reply.Success = false
			reply.Err_code = 3
			return
		}
	}
	rf.readSnapshotPersist(args.Data)
	if args.LastIncludedIndex > rf.lastIndexInSnapshot {
		DPrintf("node %d args.lastindex %d localLastIndex %d, len(log) %d, snapshot %d ", rf.me, args.LastIncludedIndex, rf.getLastLogIndex(), len(rf.log), rf.lastIndexInSnapshot)
		panic("install error")
	}
}

func (rf *Raft) readSnapshotPersist(data []byte) {
	if data == nil || len(data) < 1 { // bootstrap without any state?
		return
	}

	var ClientMaxRequestSeq map[int32]int
	var rsm map[string]string
	var lastIndexInSnapshot int
	var lastTerm int

	r := bytes.NewBuffer(data)
	d := labgob.NewDecoder(r)
	if d.Decode(&ClientMaxRequestSeq) != nil {
		panic(fmt.Sprintf("node %d: install snapshot meet decode ClientMaxRequestSeq error", rf.me))
	}
	if d.Decode(&rsm) != nil {
		panic(fmt.Sprintf("node %d: install snapshot meet decode RSM error", rf.me))
	}
	if d.Decode(&lastIndexInSnapshot) != nil || d.Decode(&lastTerm) != nil {
		panic(fmt.Sprintf("node %d: install snapshot meet decode index& term error", rf.me))
	}
	DPrintf("node %d read lastIndex snapshot %d", rf.me, lastIndexInSnapshot)
	rf.mu.Lock()
	if lastIndexInSnapshot <= rf.lastIndexInSnapshot {
		DPrintf("node %d lastIndex snapshot %d already snapshotted, cur is %d ", rf.me, lastIndexInSnapshot, rf.lastIndexInSnapshot)
		rf.mu.Unlock()
		return
	}
	rf.lastTermInSnapshot = lastTerm
	var newlog []CommandTerm
	newlog = append(newlog, CommandTerm{nil, lastTerm})
	rf.log = newlog
	rf.lastIndexInSnapshot = lastIndexInSnapshot
	rf.commitIndex = lastIndexInSnapshot
	rf.mu.Unlock()
	applyMsg := &ApplyMsg{}
	applyMsg.CommandValid = false
	applyMsg.ClientMaxRequestSeq = ClientMaxRequestSeq
	DPrintf("node %d read client seq %v", rf.me, applyMsg.ClientMaxRequestSeq)
	applyMsg.Rsm = rsm
	rf.applyCh <- *applyMsg
	DPrintf("node %d : end read snapshot len(log) %d lastIndex %d, snapshot %d", rf.me, len(rf.log), rf.getLastLogIndex(), rf.lastIndexInSnapshot)
	rf.persister.SaveStateAndSnapshot(rf.persister.ReadRaftState(), data)
}

func (rf *Raft) sendSnapShot(ind int) {
	DPrintf("node %d: send snapshot to %d  and next %d", rf.me, ind, rf.nextIndex[ind])
	rf.mu.Lock()
	args := &InstallSnapShot{}
	args.Term = rf.currentTerm
	args.LeaderId = rf.leaderNum
	args.LastIncludedIndex = rf.lastIndexInSnapshot
	args.LastIncludedTerm = rf.lastTermInSnapshot
	args.Data = rf.persister.ReadSnapshot()
	rf.mu.Unlock()
	reply := &InstallSnapShotReply{}
	count := 5
	for ; count > 0; count-- {
		ok := rf.peers[ind].Call("Raft.InstallSnapShot", args, reply)
		if ok {
			break
		}
		DPrintf("node %d: call %d installsnapshot failed, %d times ", rf.me, ind, count)
	}
	if reply.Err_code != 0 {
		switch reply.Err_code {
		case 1:
			rf.mu.Lock()
			rf.currentTerm = reply.Term
			rf.leaderNum = reply.Value
			rf.mu.Unlock()
		case 2:
			//a case that should not appear
			DPrintf("ERROR: it's a heartbreak mistake,may because of the reorder or network lantency ")
			rf.nextIndex[ind] = reply.Value + 1
			rf.matchIndex[ind] = reply.Value
		case 3:
			rf.nextIndex[ind] = args.LastIncludedIndex + 1
			rf.matchIndex[ind] = args.LastIncludedIndex
		}
	} else {
		rf.nextIndex[ind] = args.LastIncludedIndex + 1
		rf.matchIndex[ind] = args.LastIncludedIndex
	}
	DPrintf("node %d: send snapshot to %d end get reply %d and next %d", rf.me, ind, reply.Err_code, rf.nextIndex[ind])
}

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
	index = rf.getNameIndex(len(rf.log) - 1)
	rf.matchIndex[rf.me] = index
	rf.mu.Unlock()
	rf.persist()
	DPrintf("node %d finished waiting index %d command %v", rf.me, index, command)
	return index, curTerm, isLeader
}

func (rf *Raft) makeTermIndex() []TermIndex {
	IndexIte := len(rf.log) - 1
	var termList []TermIndex
	curLastTerm := rf.log[IndexIte].Term + 1
	//get cache of term
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
	return termList
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
	rf.leaderNum = -1
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
		LastLogIndex: rf.getLastLogIndex(),
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
	//defer func() {
	//if e := recover(); e != nil {
	//DPrintf("leader  %d error:%v", rf.me, e)
	//}
	//}()
	var TermTimer *time.Timer
	var HBTimer *time.Timer
	//more buf than it actually needs
	AEDistributedChan := make(chan bool, len(rf.peers))
	//to first notify leader to send empty HB
	AEDistributedChan <- true
	DPrintf("leader %d in the leader process  ", rf.me)
	rf.matchIndex = make([]int, len(rf.peers))
	//command id
	var state uint32
	istermTimeOut := false
	//no use
	//commandIdList := make([]uint32, 256)
	var termList []TermIndex
	//get cache of term
	t1 := time.Now()
	termList = rf.makeTermIndex()
	t2 := time.Since(t1)
	DPrintf("termList: %v ", termList)
	curLastSnapshotIndex := rf.lastIndexInSnapshot
	DPrintf("leader %d: create index cost  %v", rf.me, t2)
	commIndtoState := make(map[int]int)
	nextInd := rf.getLastLogIndex() + 1
	rf.nextIndex = make([]int, len(rf.peers))
	for ind, _ := range rf.nextIndex {
		rf.nextIndex[ind] = nextInd
	}
	DPrintf("TEMP3:lastindex %d", rf.getLastLogIndex())
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
			//remember the pre appendentry
			logLen := rf.getLastLogIndex() + 1
			DPrintf("leader %d distributing AE| logLen %d, rf.lastIndexInsnapShot %d len(rf.log) %d", rf.me, logLen, rf.lastIndexInSnapshot, len(rf.log))
			if rf.nextIndex[rf.me] == logLen {
				DPrintf("leader %d command==nil", rf.me)
			} else {
				DPrintf("leader %d lastLog %d next %d truenext %d", rf.me, rf.getLastLogIndex(), rf.nextIndex[rf.me], rf.getTrueIndex(rf.nextIndex[rf.me]))
				DPrintf("leader %d has the command %v", rf.me, rf.log[rf.getTrueIndex(rf.nextIndex[rf.me]):])
				if _, ok := commIndtoState[logLen-1]; !ok {
					atomic.AddUint32(&state, 1)
					commIndtoState[logLen-1] = int(atomic.LoadUint32(&state))
				}
			}

			for ind := range rf.peers {
				if ind == rf.me {
					continue
				}
				appEn := &AppendEntry{}
				appEnTerm := rf.currentTerm
				appEn.Term = appEnTerm
				appEn.LeaderID = rf.me
				appEn.LeaderCommit = rf.commitIndex
				//DPrintf("SPTEST:leader %d  commit %d", rf.me, appEn.LeaderCommit)
				DPrintf("leader %d :node %d nextIndex %d len(log) %d logLen %d lastIndexSnapshot %d 2nameIndex %d trueindex %d", rf.me, ind, rf.nextIndex[ind], len(rf.log), logLen, rf.lastIndexInSnapshot, rf.nextIndex[ind], rf.getTrueIndex(rf.nextIndex[ind]))
				appEn.PrevLogIndex = rf.nextIndex[ind] - 1
				prevLogIndex := appEn.PrevLogIndex
				if rf.nextIndex[ind] <= rf.lastIndexInSnapshot {
					DPrintf("node %d: next %d should go to installsnapshot ", ind, rf.nextIndex[ind])
					rf.sendSnapShot(ind)
				}
				DPrintf("TEMP:node %d len(log) %d logLen %d lastIndexSnapshot %d 2nameIndex %d trueindex %d", ind, len(rf.log), logLen, rf.lastIndexInSnapshot, prevLogIndex+1, rf.getTrueIndex(prevLogIndex+1))
				if rf.nextIndex[ind] < logLen {
					rf.mu.Lock()
					if appEn.PrevLogIndex+1-rf.lastIndexInSnapshot > 0 {
						appEn.Entries = append(appEn.Entries, rf.log[rf.getTrueIndex(prevLogIndex+1):]...)
					}
					rf.mu.Unlock()
					DPrintf("TEST4: entries %v ", appEn.Entries)
				}
				//may out the lenght of log
				//DPrintf("TEST6: node %d  len(rf.log) %d, appEn.PrevLogIndex %d, truePreLogIndex %d", rf.me, len(rf.log), appEn.PrevLogIndex, rf.getTrueIndex(appEn.PrevLogIndex))
				if rf.getTrueIndex(appEn.PrevLogIndex) >= len(rf.log) {
					continue
				}
				appEn.PrevLogTerm = rf.log[rf.getTrueIndex(appEn.PrevLogIndex)].Term
				//DPrintf("DEBUG:2 node %d appEn.PrevlogIndex %d len rf.log %d", ind, appEn.PrevLogIndex, len(rf.log))
				go func(ind int, commState uint32, prevLogIndex int) {
					//defer func() {
					//	if e := recover(); e != nil {
					//		DPrintf("leader %d send AE to %d routines down :%v", rf.me, ind, e)
					//	}
					//}()
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
									} else {
										DPrintf("leader %d: node %d  gone cause index has been processed |AE prevlogindex %v at term %d matchindex %d  ", rf.me, appendEntryReply.Me, appEnPointer.PrevLogIndex, rf.currentTerm, rf.matchIndex[ind])
										DPrintf("rf.getTrueIndex(rf.matchIndex[ind]) %d len(rf.log) %d", rf.getTrueIndex(rf.matchIndex[ind]), len(rf.log))
										//DPrintf("rf.log[rf.getTrueIndex(rf.matchIndex[ind])].Term %d", rf.log[rf.getTrueIndex(rf.matchIndex[ind])].Term)
										return
									}
								} else {
									rf.matchIndex[ind] = appendEntryReply.CommitIndex
									rf.nextIndex[ind] = rf.matchIndex[ind] + 1
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
										appEnTmp.PrevLogIndex = appendEntryReply.CommitIndex
										if appEnTmp.PrevLogIndex >= rf.getLastLogIndex() {
											return
										}
										rf.mu.Lock()
										oneLogStartIndex := rf.getTrueIndex(appEnTmp.PrevLogIndex)
										appEnTmp.PrevLogTerm = rf.log[oneLogStartIndex].Term
										if oneLogStartIndex > 0 {
											appEnTmp.Entries = append(appEnTmp.Entries, rf.log[oneLogStartIndex+1:oneLogStartIndex+2]...)
										}
										rf.mu.Unlock()
									} else {
										appEnTmp.PrevLogIndex = currComInd
										oneLogStartIndex := rf.getTrueIndex(appEnTmp.PrevLogIndex)
										appEnTmp.PrevLogTerm = rf.log[oneLogStartIndex].Term
										appEnTmp.Entries = append(appEnTmp.Entries, rf.log[oneLogStartIndex+1:]...)
									}
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
									DPrintf("ERROR: currComInd<rf.commitIndex but don't affect the following codes")
								}
								if rf.leaderNum != rf.me {
									DPrintf("leader %d node %d is leader", rf.me, rf.leaderNum)
									return
								}
								rf.mu.Lock()
								for i := rf.commitIndex + 1; i <= currComInd; i++ {
									if rf.getTrueIndex(i) >= len(rf.log) || rf.getTrueIndex(i) < 1 {
										DPrintf("leader %d com nameIndex % true %d FAiled", rf.me, rf.getNameIndex(i), rf.getTrueIndex(i))
										break
									}
									var applyMsg ApplyMsg
									applyMsg.Command = rf.log[rf.getTrueIndex(i)].Command
									applyMsg.CommandIndex = i
									applyMsg.CommandValid = true
									if i > rf.commitIndex {
										rf.commitIndex = i
										applyCh <- applyMsg
										DPrintf("leader %d: commit com at %d of term %d command %v term %d", rf.me, i, rf.log[rf.getTrueIndex(i)].Term, rf.log[rf.getTrueIndex(i)].Command, rf.log[rf.getTrueIndex(i)].Term)
										//update this commit message in time
										if len(AEDistributedChan) == 0 {
											AEDistributedChan <- true
										}
									}
								}
								rf.mu.Unlock()
								if rf.commitIndex > rf.nextIndex[rf.me] {
									rf.nextIndex[rf.me] = rf.commitIndex
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
									if curLastSnapshotIndex != rf.lastIndexInSnapshot {
										termList = rf.makeTermIndex()
										DPrintf("leader %d update termlist %v", rf.me, termList)
									}
									for index < len(termList)-1 && termList[index].Term > lastTerm {
										index++
									}
									if termList[index].Term <= lastTerm && termList[index].LastIndexOfTerm < prevlog {
										prevlog = termList[index].LastIndexOfTerm
										DPrintf("leader %d to node %d prevlogIndex go to %d |in termList index %d termList[ind] %d", rf.me, ind, prevlog, index, termList[index].Term)
									}
								}
								rf.nextIndex[ind] = prevlog + 1
								if prevlog < 0 {
									DPrintf("leader %d prevlogindex becomes <0", rf.me)
									panic("leader:prevlog go to < zero")
								}
								if prevlog < rf.lastIndexInSnapshot {
									DPrintf("node %d: next %d should go to installsnapshot ", ind, rf.nextIndex[ind])
									rf.sendSnapShot(ind)
									prevlog = rf.matchIndex[ind]
								}
								if prevlog >= len(rf.log) {
									return
								}
								appEnTmp.PrevLogIndex = prevlog
								appEnTmp.Entries = append(appEnTmp.Entries, rf.log[rf.getTrueIndex(prevlog+1)])
								appEnTmp.Term = rf.currentTerm
								DPrintf("TEST6: node %d  len(rf.log) %d, prelog %d, truePreLogIndex %d", rf.me, len(rf.log), prevlog, rf.getTrueIndex(prevlog))
								appEnTmp.PrevLogTerm = rf.log[rf.getTrueIndex(prevlog)].Term
								appEnPointer = &appEnTmp
								DPrintf("leader %d go to decrement the %d's log to prelog %d  command Term:%d   count %d", rf.me, appendEntryReply.Me, prevlog, rf.log[rf.getTrueIndex(prevlog)].Term, count)
							}
						} else {
							DPrintf("leader: %d  send AE to %d failed the %d time", rf.me, ind, count)
							count++
							if count >= 8 {
								return
							}
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
	//	defer func() {
	//		if e := recover(); e != nil {
	//			DPrintf("node  %d main down %v", rf.me, e)
	//		}
	//		DPrintf("node %d mainProcess exit", rf.me)
	//	}()
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
	DPrintf("me %d: starting sizeof peers=%d", me, len(peers))

	// Your initialization code here (2A, 2B, 2C).
	rf.currentTerm = 0
	// initialize from state persisted before a crash
	rf.applyCh = applyCh
	rf.readSnapshotPersist(persister.ReadSnapshot())
	rf.readPersist(persister.ReadRaftState())

	if len(rf.log) < 1 {
		rf.log = append(rf.log, CommandTerm{nil, 0})
	}
	rf.commitIndex = rf.lastIndexInSnapshot
	rf.lastApplied = 0
	rf.AENotifyChan = make(chan bool, len(rf.peers))
	//most 256 command in the queue
	rf.commandToBeExec = make(chan bool, 256)

	rand.Seed(time.Now().UnixNano())
	//Election timeout set to 256-512ms
	elecTimeOut := rand.Int()
	elecTimeOut = (elecTimeOut&255 + 256)
	DPrintf("node %d elecTimeOut :%d commitindex %d lastSnapshot %d", rf.me, elecTimeOut, rf.commitIndex, rf.lastIndexInSnapshot)
	go mainProcess(rf, elecTimeOut, applyCh)
	return rf
}
