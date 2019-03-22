package raftkv

import (
	"crypto/rand"
	"labrpc"
	"math/big"
	"sync"
	"time"
)

type Clerk struct {
	servers []*labrpc.ClientEnd
	// You will have to modify this struct.
	mu          sync.Mutex
	ID          int32
	commSeq     int
	LeaderIndex int
	int
}

func nrand() int64 {
	max := big.NewInt(int64(1) << 62)
	bigx, _ := rand.Int(rand.Reader, max)
	x := bigx.Int64()
	return x
}

func MakeClerk(servers []*labrpc.ClientEnd) *Clerk {
	ck := new(Clerk)
	ck.servers = servers
	ck.LeaderIndex = -1
	ck.commSeq = 1
	ck.ID = int32(nrand())
	// You'll have to add code here.
	return ck
}

// returns "" if the key does not exist.
// keeps trying forever in the face of all other errors.
//
// you can send an RPC with code like this:
// ok := ck.servers[i].Call("KVServer.Get", &args, &reply)
//
// the types of args and reply (including whether they are pointers)
// must match the declared types of the RPC handler function's
// arguments. and reply must be passed as a pointer.
//
//TODO:get and putappend has same infra, there should be extracted a func
func (ck *Clerk) Get(key string) string {

	// You will have to modify this function.
	getArgs := &GetArgs{}
	getArgs.Key = key
	//get leader first
	DPrintf("client: %v reqeust get key:%s", ck.ID, key)
	var leaderNum int
	var ret string
	ck.mu.Lock()
	getArgs.CommSeq = ck.commSeq
	ck.commSeq++
	ck.mu.Unlock()
	getArgs.ClientID = ck.ID
	for true {
		getReply := &GetReply{}
		if ck.LeaderIndex != -1 {
			leaderNum = ck.LeaderIndex
		} else {
			leaderNum = (int(nrand()) % len(ck.servers))
		}
		DPrintf("client %v: get leaderNum %d", ck.ID, leaderNum)
		ok := ck.servers[leaderNum].Call("KVServer.Get", getArgs, getReply)
		if getReply.WrongLeader || !ok {
			DPrintf("client: %v ask %d but wrong leader", ck.ID, leaderNum)
			//TODO::optimization needed
			ck.LeaderIndex = -1
			time.Sleep(10 * time.Millisecond)
		} else if getReply.Err != Err("") {
			DPrintf("kv %s: get meets err", string(getReply.Err))
		} else {
			ret = getReply.Value
			ck.LeaderIndex = leaderNum
			break
		}
	}
	DPrintf("client: %v end reqeust get key:%s value:%s", ck.ID, key, ret)
	return ret
}

//
// shared by Put and Append.
//
// you can send an RPC with code like this:
// ok := ck.servers[i].Call("KVServer.PutAppend", &args, &reply)
//
// the types of args and reply (including whether they are pointers)
// must match the declared types of the RPC handler function's
// arguments. and reply must be passed as a pointer.
//

func (ck *Clerk) PutAppend(key string, value string, op string) {
	// You will have to modify this function.
	paArgs := &PutAppendArgs{}
	paArgs.Key = key
	paArgs.Value = value
	paArgs.Op = op
	paArgs.ClientID = ck.ID
	ck.mu.Lock()
	paArgs.CommSeq = ck.commSeq
	ck.commSeq++
	ck.mu.Unlock()
	DPrintf("client: %v reqeust pa ID %d  %s key:%s value:%s", ck.ID, paArgs.CommSeq, op, key, value)
	var leaderNum int
	for true {
		paReply := &PutAppendReply{}
		if ck.LeaderIndex != -1 {
			leaderNum = ck.LeaderIndex
		} else {
			leaderNum = (int(nrand()) % len(ck.servers))
		}
		DPrintf("client %v: get leaderNum %d", ck.ID, leaderNum)
		//ok := ck.servers[(leaderNum+len(ck.servers)-1)%(len(ck.servers))].Call("KVServer.PutAppend", paArgs, paReply)
		ok := ck.servers[leaderNum].Call("KVServer.PutAppend", paArgs, paReply)
		if paReply.WrongLeader || !ok {
			DPrintf("client: %v ask %d but wrong leader", ck.ID, leaderNum)
			ck.LeaderIndex = -1
			time.Sleep(10 * time.Millisecond)
			//TODO::optimization needed
		} else if paReply.Err != Err("") {
			DPrintf("kv %s: put meets err", string(paReply.Err))
		} else {
			ck.LeaderIndex = leaderNum
			break
		}
	}
	DPrintf("client: %v end reqeust pa ID %d  %s key:%s ", ck.ID, paArgs.CommSeq, op, key)
}

func (ck *Clerk) Put(key string, value string) {
	ck.PutAppend(key, value, "Put")
}
func (ck *Clerk) Append(key string, value string) {
	ck.PutAppend(key, value, "Append")
}

//buggy, debug needed
func (ck *Clerk) GetLeader() int {
	for i := 0; i < len(ck.servers); i++ {
		reply := &GetReply{}
		reply.WrongLeader = true
		ok := ck.servers[i].Call("KVServer.IsLeader", reply, reply)
		if ok && !reply.WrongLeader {
			DPrintf("serverend i %d is true leader", i)
			return i
		}
	}
	return -1
}
