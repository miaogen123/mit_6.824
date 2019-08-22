### Spinnaker 总结

- 支持强弱一致性：strong consistency &timeline consistency
- sharding Data store：based on key range
- 3 replica：可以保证online upgrade& avoid panic mode
- CONTRIBUTION ：
  - 使用paxos协议并集成了log replication & recovery process
  - 相对于Cassandra，性能还可以，不输上下。虽然有些地方受限于Zookeeper，但是spinnaker本来就是设计的single datacenter用的。
  - High availablity and recover less in 1 second
- spinnker 是CA系统，dynamo 是AP系统。原因在于dynamo支持的是eventual consistency
- Dynamo (弱一致性)：使用vector clock resolve conflicts, (? timestamp)
- BigTable中没有Hot standby的概念，GFS并不适合 transactional workloads。
- spinnaker 的get支持强弱一致性flag，使用strong consistency的时候，从每一个cohort的leader那里读数据，如果一致性要求不高的话，使用timeline consistency的时候，从cohort的任意一个节点读取数据。put则是强制的SC 一致。条件删与条件put，这个条件是时间戳(version number)。
- 每一个操作都是一个single transaction。
- 每一个cohort为了方便不同的node之间进行replica过程，使用了相同的LSN，内部使用了类似commit queue来记录pending writes，committed writes is logged in memtable，然后会周期性的被写入到SStable。(与bigTable类似)。
- 利用了ZK来实现分布式锁，barriers，group membership。但是ZK不是整个spinnaker中的 critical path。
- pre-cohort 预先分组，以组为单位进行leaderElection：谁的f.lst最大，最后谁是leader。每一次的logreplication也是两个轮回的message，都是forcee log。
- commit period：leader的commit 周期。
- Follower Recovery ：分成两个阶段local recovery & catch up：Phase1 ：local recovery 用来reapply 他自己的一些log，来重建memtable。Phase2：follower发送自己的f.cmt给leader。leader把之后的log发给follower。follower得以跟leader同步，注意到leader 在catch up的最后阶段会block writes，来达到一个一致的状态。
- Logical truncation问题：对于f.cmt之前的可以rebuild，对于f.cmt之后的，可以直接进行overwrite吗？**不可以，因为一个node 的这部分log还会被其他的cohort share。因为不同的cohort的log被merge到了一个日志中去，如果不同的cohort的日志在不同的文件中，那么truncation就是可以的，并且按照appendix 这里可以使用SSD来进行优化 。**Solution：follower的从f.cmt到f.lst的LSN被记录在skipped-LSN list中，每一次进行recovery的时候都会从这里面找到LSN list。
- leader Takeover：新leadercommit 旧log的方法跟raft类似。leaderElection过程借助于ZK来实现：
  - 对于每一个Key range(cohort)都有一个在ZK的node (let's say r)， 然后该cohort的leader以及candidate都在这里。进行candidate的选举的时候，每个node都在/r/candidates/ 下面加上一个ephemeral znode with value 是自己的lst，然后设置watch on this node。当大多数的node 出现在candidates下面时，就会node会检查自己是不是leader，是的话，在/r/leader下面添加znode写上自己的 hostname，以便其他的人知晓；不是的话就到/r/leader下面找到leader。(每一个node都有event handler 作为ZK 的client)
- Design tradeoffs：一方面spinnaker更加灵活，但另一方面强一致性导致leader的负担重，并且容易造成瓶颈。
- 综测：相对于Cassandra和Dynamo：spinnaker在write方面会耗点时间，但是相差不大。由于C&D两者都是弱一致性，当node发生了failure以后，node不一定能被带到consistent的状态，需要额外的方法来消除conflict，dynamo使用了vector clock。
- spinnake设计用来作为singer-datacenter的。

#### Appendix

- spinnaker实际上是基于Cassandra的分支进行修改的。
- multi-paxos底层基于不可靠message layer 假设，而spinnaker则通过in-order message base-on TCP
- 在spinnaker的recovery过程中，先完成recovery 然后自增epoch，再开启write。
- 影响spinnaker性能的原因：CPU和network瓶颈(typical for transaction workload)、log force (also typical for transaction workload)。Cassandra的log 模块也限制了spinnaker，因为C的log 没有spinnaker频繁。可行的改进在于 async I/O，multiple log buffer， preallocated log files。
- SSD可以显著提升Spinnaker的性能，因为寻道的时间减少了很多。并且可以将不同cohort的log 进行分离到不同的file中进行优化，因为SSD随机访问的速度快很多。
- **spinnaker的leaderElection机制与raft的类似，因为spinnaker中同样有epoch的比较，类似于term的比较，在term的相同情况下在比较长度。**

#### questions

- 文章里讲的force log write 与non-force，区别在于落盘不落盘？
  - 是的



#### SOME KEY POINTS FROM  LECTURE

- Raft snapshots,  linearizability,  duplicate RPCs,  faster gets, 
-  service tells Raft it is snapshotted **through some log index**，这里通过特殊的log？
-  service will see repeats, **must detect repeated index, ignore**
- **？？？**Raft log may start before the snapshot (but definitely not after)
-  what does a follower do with InstallSnapshot? **follower受到snapshotRPC时候的操作**
  - ignore if term is old (not the current leader)
  - ignore if follower already has last included index/term
        it's an old/delayed RPC
  - if not ignored:
        empty the log, replace with fake "prev" entry
        set lastApplied to lastIncludedIndex
        replace service state (e.g. k/v table) with snapshot contents
- linearizability definition **线性化的定义**:从反面出发：如果在一个在W完成后的开始的R读到了旧值那么认为不满足一致性。
    an execution history is linearizable if 
      one can find a total order of all operations,
      that matches real-time (for non-overlapping ops), and
      in which each read sees the value from the
      write preceding it in the order.

- **对于并发的写，由service控制写的顺序**
- 重复RPC的问题：lecture建议给client设置一个ID，每一个RPC设置一个seq，然后如果遇到的seq大于当前的才执行，小于等于不执行。并且leader记录这样的table，每一个replica在收到leader消息的时候也及时的更新，这样当一个leader选出来之后，他就具有了之前了leader的table。**？？？这样在leader 完成request的 rpc_reply 丢失的时候并且leader随即宕机，如何保证旧leader不重复执行？**。我想到的办法是在log里除了command和term以外在加上一个新的ID字段，ID由clientID和rpc seq组成(为了避免悬挂了很长时间的rpc请求，可以再添加一个timestamp)，leader收到RPC的时候，首先检查ID，看是否已经出现在了log中。这样想是因为前后两个leader的共同信息只有log。
  - 优点：防重复apply
  - 缺点：log变大，recover时间受影响
- 优化Read-op：**使用Lease机制**：每一个leader收到majority AER的时候，认为在接下来的lease 时期：N ms内，自己是唯一的leader，从而对于接下来的read-op直接返回，不进行logRead。**并且新leader在上一个lease过期前不能执行put操作()**

#### FAQ

- Q: 文章中没有提到consistent read 在leader change以后如何进行的修改，换言之，如何知道自己还是不是leader。
  - A:可能是lease机制，也可能是zookeeper主动通知的。	
- 



#### 优化

- 一般的情况下都是leader负责回复请求，这里如果让leader进行转发，把收到的消息转发给follower让follower进行处理，client发送的时候，带上一个token，follower回复的时候也带上token。这样不就在提升了对follower的利用率？

