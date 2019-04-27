package shardmaster

import (
	"labgob"
	"labrpc"
	"raft"
	"sort"
	"sync"
	"time"
)

type ShardMaster struct {
	mu      sync.Mutex
	me      int
	rf      *raft.Raft
	applyCh chan raft.ApplyMsg
	// Your data here.

	curCommitIndex      int
	configs             []Config // indexed by config num
	ClientMaxRequestSeq map[int32]int
	resultMap           map[int64]Config
}

//op type
type OpType int

const (
	JoinType  OpType = 1
	LeaveType OpType = 2
	MoveType  OpType = 3
	QueryType OpType = 4
)

type Op struct {
	// Your data here.
	ClientID   int32
	ProposalID int
	//JOIN 1
	//LEAVE 2
	//MOVE 3
	//QUERY 4
	//the function of OPtype is to distinguish the requests that change config and not
	Optype  OpType
	Servers map[int][]string // new GID -> servers mappings
	GIDs    []int
	Shard   int
	GID     int
	Num     int // desired config number
}

func rebalance(pLastConfig *Config) {
	//sm.mu.Lock()
	groupCount := len((*pLastConfig).Groups)
	if groupCount <= 0 {
		return
	}
	var GroupsIDs []int
	//GroupsIDs := make([]int, 3)
	//GroupsIDs := []int{}
	for key, _ := range (*pLastConfig).Groups {
		GroupsIDs = append(GroupsIDs, key)
	}
	sort.Ints(GroupsIDs)
	shardPerGroup := int(NShards / groupCount)
	remainShards := NShards - shardPerGroup*groupCount
	var shardInd int
	for i := 0; i < groupCount; i++ {
		for j := 0; j < shardPerGroup; j++ {
			pLastConfig.Shards[shardInd] = GroupsIDs[i]
			shardInd++
		}
		if remainShards > 0 {
			pLastConfig.Shards[shardInd] = GroupsIDs[i]
			remainShards--
		}
	}
	//sm.mu.Unlock()
}

func (sm *ShardMaster) Join(args *JoinArgs, reply *JoinReply) {
	// Your code here.
	if sm.me != sm.rf.GetLeaderNum() {
		reply.WrongLeader = true
		return
	}
	DPrintf("SM %d: JOIN %v", sm.me, args.Servers)
	var comm Op
	comm.ClientID = args.ClientID
	comm.ProposalID = args.CommSeq
	comm.Optype = JoinType
	comm.Servers = args.Servers
	index, _, _ := sm.rf.Start(comm)

	for true {
		leaderNum := sm.rf.GetLeaderNum()
		if leaderNum != sm.me {
			reply.WrongLeader = true
			DPrintf("sm %d:JOIN is not leader (after start) (%d is)", sm.me, leaderNum)
			return
		}
		if sm.curCommitIndex >= index {
			break
		} else {
			DPrintf("sm %d:JOIN waitting 3ms clientiD %d seq %d index %d", sm.me, comm.ClientID, comm.ProposalID, index)
			time.Sleep(3 * time.Millisecond)
		}
	}
	DPrintf("sm %d: end Join ", sm.me)
	return
}

func (sm *ShardMaster) Leave(args *LeaveArgs, reply *LeaveReply) {
	// Your code here.
	if sm.me != sm.rf.GetLeaderNum() {
		reply.WrongLeader = true
		return
	}
	DPrintf("SM %d: LEAVE %v", sm.me, args.GIDs)
	var comm Op
	comm.ClientID = args.ClientID
	comm.ProposalID = args.CommSeq
	comm.Optype = LeaveType
	comm.GIDs = args.GIDs
	index, _, _ := sm.rf.Start(comm)

	for true {
		leaderNum := sm.rf.GetLeaderNum()
		if leaderNum != sm.me {
			reply.WrongLeader = true
			DPrintf("sm %d:LEAVE is not leader (after start) (%d is)", sm.me, leaderNum)
			return
		}
		if sm.curCommitIndex >= index {
			break
		} else {
			DPrintf("sm %d:LEAVE waitting 3ms clientiD %d seq %d index %d", sm.me, comm.ClientID, comm.ProposalID, index)
			time.Sleep(3 * time.Millisecond)
		}
	}
	DPrintf("SM %d:end LEAVE %v", sm.me, args.GIDs)
	return

}

func (sm *ShardMaster) Move(args *MoveArgs, reply *MoveReply) {
	// Your code here.
	if sm.me != sm.rf.GetLeaderNum() {
		reply.WrongLeader = true
		return
	}
	DPrintf("SM %d: MOVE shard %d to gid %d", sm.me, args.Shard, args.GID)
	var comm Op
	comm.ClientID = args.ClientID
	comm.ProposalID = args.CommSeq
	comm.Optype = MoveType
	comm.GID = args.GID
	comm.Shard = args.Shard
	index, _, _ := sm.rf.Start(comm)

	for true {
		leaderNum := sm.rf.GetLeaderNum()
		if leaderNum != sm.me {
			reply.WrongLeader = true
			DPrintf("sm %d:MOVE is not leader (after start) (%d is)", sm.me, leaderNum)
			return
		}
		if sm.curCommitIndex >= index {
			break
		} else {
			DPrintf("sm %d:MOVE waitting 3ms clientiD %d seq %d index %d", sm.me, comm.ClientID, comm.ProposalID, index)
			time.Sleep(3 * time.Millisecond)
		}
	}
	DPrintf("SM %d:end MOVE shard %d to gid %d", sm.me, args.Shard, args.GID)
	return
}

func (sm *ShardMaster) Query(args *QueryArgs, reply *QueryReply) {
	// Your code here.
	if sm.me != sm.rf.GetLeaderNum() {
		reply.WrongLeader = true
		return
	}
	var comm Op
	comm.ClientID = args.ClientID
	comm.ProposalID = args.CommSeq
	comm.Optype = QueryType
	comm.Num = args.Num
	index, _, _ := sm.rf.Start(comm)
	for true {
		leaderNum := sm.rf.GetLeaderNum()
		if leaderNum != sm.me {
			reply.WrongLeader = true
			DPrintf("sm %d:QUERY is not leader (after start) (%d is)", sm.me, leaderNum)
			return
		}
		if sm.curCommitIndex >= index {
			break
		} else {
			DPrintf("sm %d:QUERY waitting 3ms clientiD %d seq %d index %d", sm.me, comm.ClientID, comm.ProposalID, index)
			time.Sleep(3 * time.Millisecond)
		}
	}
	commID := int64(comm.ClientID)<<32 + int64(comm.ProposalID)
	sm.mu.Lock()
	reply.Config = sm.resultMap[commID]
	delete(sm.resultMap, commID)
	sm.mu.Unlock()
	DPrintf("SM %d:len(config):%v end QUERY %v return %v", sm.me, len(sm.configs), args.Num, reply.Config)
	return
}

func mainProcess(sm *ShardMaster) {
	var applyMsg raft.ApplyMsg
	for true {
		select {
		case applyMsg = <-sm.applyCh:
			commitIndex := applyMsg.CommandIndex
			commOp := applyMsg.Command.(Op)
			commClientID := commOp.ClientID
			if commitIndex > sm.curCommitIndex {
				sm.curCommitIndex = commitIndex
			}
			sm.mu.Lock()
			if commOp.Optype == QueryType {
				commID := int64(commOp.ClientID)<<32 + int64(commOp.ProposalID)
				if commOp.Num >= 0 && commOp.Num < len(sm.configs) {
					sm.resultMap[commID] = sm.configs[commOp.Num]
				} else {
					sm.resultMap[commID] = sm.configs[len(sm.configs)-1]
				}
				sm.mu.Unlock()
				break
			}
			//DPrintf("sm commIndex %d cur %d", commitIndex, sm.curCommitIndex)
			if _, ok := sm.ClientMaxRequestSeq[commClientID]; ok {
				if sm.ClientMaxRequestSeq[commClientID] >= commOp.ProposalID {
					sm.mu.Unlock()
					break
				}
			}
			sm.ClientMaxRequestSeq[commClientID] = commOp.ProposalID
			newConfig := &Config{}
			newConfig.Num = sm.configs[len(sm.configs)-1].Num + 1
			newConfig.Groups = make(map[int][]string)
			for key, val := range sm.configs[len(sm.configs)-1].Groups {
				newConfig.Groups[key] = val
			}
			for i := 0; i < NShards; i++ {
				newConfig.Shards[i] = sm.configs[len(sm.configs)-1].Shards[i]
			}

			switch commOp.Optype {
			case JoinType:
				for key, val := range commOp.Servers {
					_, ok := newConfig.Groups[key]
					if !ok {
						newConfig.Groups[key] = val
					} else {
						newConfig.Groups[key] = append(newConfig.Groups[key], val...)
					}
				}
			case LeaveType:
				for _, key := range commOp.GIDs {
					delete(newConfig.Groups, key)
					pShards := &(newConfig.Shards)
					for i := 0; i < len(*pShards); i++ {
						if key == pShards[i] {
							pShards[i] = -1
						}
					}
				}
			case MoveType:
				newConfig.Shards[commOp.Shard] = commOp.GID
			}
			DPrintf("sm %d: get config %v", sm.me, *newConfig)
			rebalance(newConfig)
			DPrintf("sm %d: after balance config %v", sm.me, *newConfig)
			sm.configs = append(sm.configs, *newConfig)
			sm.mu.Unlock()
		}
	}
}

// the tester calls Kill() when a ShardMaster instance won't
// be needed again. you are not required to do anything
// in Kill(), but it might be convenient to (for example)
// turn off debug output from this instance.

func (sm *ShardMaster) Kill() {
	sm.rf.Kill()
	DPrintf("SM %d: killed ", sm.me)
	// Your code here, if desired.
}

// needed by shardkv tester
func (sm *ShardMaster) Raft() *raft.Raft {
	return sm.rf
}

//
// servers[] contains the ports of the set of
// servers that will cooperate via Paxos to
// form the fault-tolerant shardmaster service.
// me is the index of the current server in servers[].
//

func StartServer(servers []*labrpc.ClientEnd, me int, persister *raft.Persister) *ShardMaster {
	sm := new(ShardMaster)
	sm.me = me

	sm.configs = make([]Config, 1)
	sm.configs[0].Groups = map[int][]string{}

	sm.ClientMaxRequestSeq = make(map[int32]int)
	labgob.Register(Op{})
	sm.resultMap = make(map[int64]Config)
	sm.applyCh = make(chan raft.ApplyMsg)
	sm.rf = raft.Make(servers, me, persister, sm.applyCh)
	// Your code here.
	go mainProcess(sm)
	return sm
}
