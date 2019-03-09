package raftkv

import (
	"fmt"
	"labgob"
	"labrpc"
	"log"
	"raft"
	"sync"
	"time"
)

const Debug = 0

func init() {
	log.SetFlags(log.Lmicroseconds)
}

func DPrintf(format string, a ...interface{}) (n int, err error) {
	if Debug > 0 {
		//log.Printf(format, a...)
		fmt.Printf("%v", time.Now())
		fmt.Printf(" ")
		fmt.Printf(format, a...)
		fmt.Printf("\n")
	}
	return
}

type Op struct {
	// Your definitions here.
	// Field names must start with capital letters,
	// otherwise RPC will break.
	ClientID   int64
	ProposalID int
	Optype     OpType
	Key        string
	Value      string
}

type KVServer struct {
	mu           sync.Mutex
	me           int
	rf           *raft.Raft
	applyCh      chan raft.ApplyMsg
	maxraftstate int // snapshot if log grows this big

	// Your definitions here.
	rsm                 map[string]string
	curCommitIndex      int
	ClientMaxRequestSeq map[int64]int
}

func (kv *KVServer) Get(args *GetArgs, reply *GetReply) {
	// Your code here.
	//TODO::getstate is not necessary, but GetLeaderNum is
	leaderNum := kv.rf.GetLeaderNum()
	if leaderNum != kv.me {
		reply.WrongLeader = true
		DPrintf("kv %d: is not leader (get) (%d is)", kv.me, leaderNum)
		return
	}
	var comm Op
	//comm.ClientID = 1
	//comm.ProposalID = 1
	comm.Optype = GetType
	comm.Key = args.Key
	//wait util the success
	index, _, _ := kv.rf.Start(comm)
	for true {
		//need a timeout handler
		leaderNum := kv.rf.GetLeaderNum()
		if leaderNum != kv.me {
			reply.WrongLeader = true
			DPrintf("kv %d: is not leader (get) (%d is)", kv.me, leaderNum)
			return
		}
		if kv.curCommitIndex >= index {
			kv.mu.Lock()
			reply.Value = kv.rsm[args.Key]
			kv.mu.Unlock()
			DPrintf("server %d: reply to get key %v value %v", kv.me, args.Key, reply.Value)
			break
		} else {
			time.Sleep(5 * time.Millisecond)
		}
	}
	DPrintf("server %d get: return value will be seen in client output", kv.me)
}

func (kv *KVServer) IsLeader(reply *GetReply, reply2 *GetReply) {
	leaderNum := kv.rf.GetLeaderNum()
	if leaderNum == kv.me {
		reply.WrongLeader = false
		return
	}
	reply.WrongLeader = true
}

func (kv *KVServer) PutAppend(args *PutAppendArgs, reply *PutAppendReply) {
	// Your code here.
	var comm Op
	if args.Op == "Put" {
		comm.Optype = PutType
		DPrintf("server %d get PUT", kv.me)
	} else {
		comm.Optype = AppendType
		DPrintf("server %d get APPEND", kv.me)
	}
	leaderNum := kv.rf.GetLeaderNum()
	comm.ClientID = args.ClientID
	comm.ProposalID = args.CommSeq
	comm.Key = args.Key
	comm.Value = args.Value
	if _, ok := kv.ClientMaxRequestSeq[args.ClientID]; !ok {
		DPrintf("server %d add new client %d", kv.me, args.ClientID)
		kv.mu.Lock()
		kv.ClientMaxRequestSeq[args.ClientID] = 0
		kv.mu.Unlock()
	}
	if leaderNum != kv.me {
		reply.WrongLeader = true
		DPrintf("kv %d: is not leader (pa) (%d is)", kv.me, leaderNum)
		return
	}
	//wait util the success
	DPrintf("server %d starting key %s value %v", kv.me, comm.Key, comm.Value)
	index, _, _ := kv.rf.Start(comm)
	for true {
		leaderNum := kv.rf.GetLeaderNum()
		if leaderNum != kv.me {
			reply.WrongLeader = true
			DPrintf("kv %d: is not leader (after start) (%d is)", kv.me, leaderNum)
			return
		}
		if kv.curCommitIndex >= index {
			break
		} else {
			DPrintf("server %d waitting 5ms ", kv.me)
			time.Sleep(5 * time.Millisecond)
		}
	}
	DPrintf("server %d after PA ", kv.me)
}

//
// the tester calls Kill() when a KVServer instance won't
// be needed again. you are not required to do anything
// in Kill(), but it might be convenient to (for example)
// turn off debug output from this instance.
//
func (kv *KVServer) Kill() {
	kv.rf.Kill()
	// Your code here, if desired.
}

//listen the log
func mainProcess(kv *KVServer) {
	var applyMsg raft.ApplyMsg
	for true {
		select {
		case applyMsg = <-kv.applyCh:
			commIndex := applyMsg.CommandIndex
			commOp := applyMsg.Command.(Op)
			kv.mu.Lock()
			switch commOp.Optype {
			case GetType:
				DPrintf("kv %d: get Type:read %v", kv.me, commOp.Key)
			case PutType:
				if commOp.ProposalID <= kv.ClientMaxRequestSeq[commOp.ClientID] {
					DPrintf("kv %d: get REPEAT Type:put %v, clientID:%v ProposalID:%v,IGNORED", kv.me, commOp.ClientID, commOp.ProposalID, commOp.Key)
					break
				}
				kv.ClientMaxRequestSeq[commOp.ClientID] = commOp.ProposalID
				kv.rsm[commOp.Key] = commOp.Value
				DPrintf("kv %d: get Type:put %v after value %v", kv.me, commOp.Key, kv.rsm[commOp.Key])
				//DPrintf("kv %d: get Type:put %s value %s", kv.me, commOp.Key, commOp.value)
				//DPrintf("kv %d: get %v value %v", kv.me, commOp.clientID, commOp.proposalID)

			case AppendType:
				if commOp.ProposalID <= kv.ClientMaxRequestSeq[commOp.ClientID] {
					DPrintf("kv %d: get REPEAT Type:APPEND %v, clientID:%v ProposalID:%v,IGNORED", kv.me, commOp.ClientID, commOp.ProposalID, commOp.Key)
					break
				}
				kv.ClientMaxRequestSeq[commOp.ClientID] = commOp.ProposalID
				if _, ok := kv.rsm[commOp.Key]; ok {
					kv.rsm[commOp.Key] = kv.rsm[commOp.Key] + commOp.Value
				} else {
					kv.rsm[commOp.Key] = commOp.Value
				}
				DPrintf("kv %d: get Type:app %v after value %v", kv.me, commOp.Key, kv.rsm[commOp.Key])
			}
			if kv.curCommitIndex < commIndex {
				kv.curCommitIndex = commIndex
			}
			kv.mu.Unlock()
		}
	}
}

//
// servers[] contains the ports of the set of
// servers that will cooperate via Raft to
// form the fault-tolerant key/value service.
// me is the index of the current server in servers[].
// the k/v server should store snapshots through the underlying Raft
// implementation, which should call persister.SaveStateAndSnapshot() to
// atomically save the Raft state along with the snapshot.
// the k/v server should snapshot when Raft's saved state exceeds maxraftstate bytes,
// in order to allow Raft to garbage-collect its log. if maxraftstate is -1,
// you don't need to snapshot.
// StartKVServer() must return quickly, so it should start goroutines
// for any long-running work.
//

func StartKVServer(servers []*labrpc.ClientEnd, me int, persister *raft.Persister, maxraftstate int) *KVServer {
	// call labgob.Register on structures you want
	// Go's RPC library to marshall/unmarshall.
	labgob.Register(Op{})

	kv := new(KVServer)
	kv.me = me
	kv.maxraftstate = maxraftstate

	// You may need initialization code here.
	kv.rsm = make(map[string]string)
	kv.applyCh = make(chan raft.ApplyMsg)
	kv.rf = raft.Make(servers, me, persister, kv.applyCh)
	kv.ClientMaxRequestSeq = make(map[int64]int)

	// You may need initialization code here.
	go mainProcess(kv)
	return kv
}
