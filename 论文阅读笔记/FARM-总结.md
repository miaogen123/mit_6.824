### FaRM 总结   

《No compromises: distributed transactions with consistency, availability, and performance》

#### 概要：

- 依赖比较新的硬件：RDMA和non-volatile DRAM，实现一个高性能低延迟的分布式事务系统，with strict serializability
  - 因为RDMA，不经过remote cpu的特性，像以前的事务实现机制，在这里都会遇到来自网卡的延迟问题。
- 从a failure 恢复只需要50ms
- 相对于dynamo和memcached(不支持事务或者支持弱的一致性保证)，FaRM提供了SS，HA，HT，LL：Strict serializability， High Availability，High throughput(吞吐量) and Low Latency
- FaRM 遵循三个原则来解除CPU bottleneck：减少消息数量(可你的事务协议可是需要4-phase)，使用one-side RDMA read write而不是 使用消息(RPC as it's cpu-related operation)，充分利用并行(???利用并行会降低CPU调用)
- 允许事务涉及到一个数据中心的any count machines，没有用paxos，而是使用了vertical-paxos 的primary-backup结构，来做replication。
- 使用了 optimistic concurrency control  4-phase commit protocol. 
- 事务执行的四个过程：Lock， validation，commit backup，commit primary
- 由于RDMA的特性，这里没有办法使用以往的lease机制。这里使用了precise membership来管理membership关系。
- failure-recovery：允许事务正在执行时进行recovery，被failure影响到的transaction需要几十ms来进行恢复，没有被影响到的就可以直接运行。
- failure-detection：主要是利用快速的心跳包来做的。
- 核心在于事务的实现，以及failure recovery，说来也是挺复杂的。

***

**2 ** 硬件趋势：讲的主要是non-volatile dram消耗的电量很小和RDMA网络

**3** 编程模型和架构：

- 在事务执行的时候，thread可以执行任意的操作，而不必被阻塞。
- 事务执行的时候，对于单个对象的read是atomic操作。（多个对象不保证），（**It does not guarantee atomicity across reads of dif ferent objects but, in this case, it guarantees that the trans action does not commit ensuring committed transactions are strictly serializable.**）可以enable defer 一致性检查 直到被commit以后，而不是在每一个对象读取的时候进行检查。这样要求FaRM应用自己解决可能出现的不一致情况。
- FaRM提供lock-free read：但是只对read-only transaction，locality hints。which enable programmers to co-locate related objects on the same set of machines.
- 每一个FaRM实例与内核线程相绑定，lease manager一直是的。
- FaRM的配置信息：<i, S,F,CM>, i 全局递增的配置ID，S是机器的集合，F是映射从机器到failure domin，CM是configuration manager。CM依赖于ZK来做agree，但是相关的lease management，detect failurecoordinate recovery，
- global adress space 由2GB的region组成，每一个machine 都有一些regions。由CM管理primary中region到backup的映射。
- 地址空间在f+1 个node上进行replicate，总是从primary上读对象，每一个对象有64-bit的versionID。
- 允许application 指定相关的 region locality。
- CM的协调过程是2PC的。所有的都认同了新的configuration，然后发送commit 消息。
- 中心化的CM要比一致性哈希要好用。

**4** Distributed transaction and replication

- primary有backup做冗余，但是coordinator 则没有冗余。

- 在事务的execute phase，事务通过one-side RDMA读object然后写操作都buff在本地

- LOCK：主要是针对要写的对象。coordinator 发送writelog给每一个machine 与被写对象相关的，然后primary 根据version 锁住对象，如果version不匹配，就lock不成功。所有的primary都返回为true的时候，才lock阶段成功。

- VALIDATION：主要是针对要读的对象，coordinator 从所有的primaries中找到相关的read object的version，然后如果有一个version发生了改变，validation失败。

- COMMIT BACKUP：coordinator 发送commit backup给所有的backup，所有都ack以后进入下一个阶段。**（?如果在这个地方，会出错吗？）**

- COMMIT PRIMARY：写primary，有点疑惑的是，coordinator在收到一个primary的ack以后或者在write locally以后就返回给客户端。

- TRUNCATE：在收到所有的primary的ack以后，coordinator才发送truncate命令。
  - 正确性：对于写对象，在lock阶段收到所有的ACK以后的那个点是一个serialization point。因为所有的writen对象都锁住了，所以相关的事务执行都是串行化的。为了确保在failure存在的情况下的serializability，必须确保coordinator收到所有的commit backup的ACK，不然，先发出去commit primary，primary可能会在未收到之前和coordinator一起挂掉，然后，其他的backup又没有收到commit backup消息，这样这一个组的primary-backup就没有一个收到信息了。
  - coordinator首先会在commit之前的阶段，预先reserve内存

- 性能：FaRM需要f+1 个节点来完成f fault tolerance，但是paxos，raft为了达到F fault tolerance 就得2F+1 个节点。并且RDMA网络的使用，降低了对于对方CPU的使用。

**5** Failure recovery

-  依赖bounded clock drift for safety and eventually bounded message delays for liveness. 
-  因为non-volatile DRAM的缘故，即便是lose power的情况，也能恢复数据。在大多数节点都能相互连接并且可以连到ZK service的时候，并且分区内至少包含每一个object的replica的时候，整个系统仍然可以正常工作。
-  总共过程分为5个phase：Failure Detection， Reconfiguration，Transaction state recovery，Recovering data，Recovering allocator state。

5.1 Failure detection：每一个机器都hold 一个lease at CM(双向的)，lease的获取采用类似TCP三次握手的方式。

-  FaRM 的 HA 来源于**极短的lease time**
-  FaRM 使用的是UDP数据来更新lease，这里说了使用TCP会有性能损耗，但单看那一段其实看不出来为什么，参考文献中[16]中应该有解释。Lease renewal 每1/5 expiry time 发生一次。
-  FaRM的lease renewal 由一个高优先级的线程周期性来做，并且不绑定到任何一个kernel thread上，使用了interrupt而不是poll避免饥饿关键的OS tasks，（虽然增加了几个ms延迟）
-  lease manager 需要的内存一开始就分配了。

5.2 Reconfiguration：由于RDMA NIC不支持lease机制，所以FaRM使用了precise membership：发生了failure 以后，所有的node都需要同意新的membership，然后才能允许object mutation。

- Suspect: 当发生lease超时的时候：CM端检测到lease 超时，然后会suspect并开始reconfiguration，阻塞外部请求，如果说是，node 端检测到lease超时，会发送reconfiguration的请求给CM backup(利用一致性hash的backup)，当一段时间后，发现配置没有发生改变，会尝试reconfiguration 自己。两种情况下，先发起reconfiguration的machine会try to 变成新的node。
- Probe：新的CM会probe每一个node(通过one-side RDMA read，除了被怀疑的)，如果一个read没有得到ACK，那对应的机器也将被suspect，只有当收到大多数是ACK时才继续下一个阶段。
- Update configuration：利用ZK的znode sequence numbers （CAS）来做配置的更新
- Remap regions：新的CM把之前的failed machine上的region重新映射到别的machines，并且考虑app的locality等相关要求。如果failed machine 是primary 有surviving backups，那就把backup promoting to a new primary
- Send new Configuration：将新的配置发送给所有的机器，如果原来的CM已经失效，这里的消息相当于一个lease 变更的请求，如果没有失效，原来的lease还会正常工作。
- Apply new Configuration: 判断新的配置的合法性，在machine更新完自己的配置ID以后，不再向不再新配置中的文件发送消息，并且，block外部请求。最后回复ACK
- Commit new Configuration: CM在收到来自**所有的 **机器的ACK以后，发送new-config-commit操作，其他machine收到以后会继续block的操作，

5.3 Transaction state recovery：FaRM通过被修改对象的replicas‘s log 来做recovery。步骤蛮多，好在每一个都是可以并行的。

- Step1 : Block access to recovering regions：对于一个region直到所有的transaction 修改都apply，通过block request for local pointer和RDMA 引用to that region 直到step4。
- Step2 : drain logs：在配置变更的时候，也会有一些Commit-backup或者commit-primary，一般可以采用直接拒绝的方式，但是对于RDMA则不能，FaRM会保证相关的records会在recovery过程中被处理。最后一个drainlog被记录在LastDrained(每一个transaction都由一个四元组表示唯一的ID：<c,m,t,l> : 配置的ID，coordinator的ID，coordinator的线程标识，线程局部的唯一标识)
- Step3: Find recovering transaction:  recovering transaction : 包括但不限于：commit phase spans configuration，或者其他的不知道说的是撒的情况。只有recovering transaction go through the recovery phase. 所有的机器都需要同意是否一个transaction 是不是recovering transaction。CM通过在原来的message上加extra metadate来做这件事。一个transaction的recover 与否是需要条件的。具体条件见：61页。backup需要向primary 发送need-recovery消息
- Step4：Lock recovery：primary收到来自backup的need-recovery消息并且log都已经被drain以后，将收到的recoveing transaction按照ID分发到各个线程。当这一步完成了以后，原来阻塞的read 请求和事务就可以与剩下的replicate log 并行执行了。
- Step5: Replicate log records: thread in  primary replicate log record by sending backups the replicate-TX-State message.
- PERFORMANCE：通过 identify recovering transaction来优化，因为对于单机失效的情况来说，真正需要recover的其实会很少，每个recovery过程都是单独的在运行于各自的region，thread，machine，避免卡性能

5.4 Recovering data：

- FaRM需要recover at new backup，但这个并非是第一位的，所以可以推迟。当一个primary上的所有region都已经处于一种active状态时，会向CM发送region-active消息，CM收到所有的region-active消息时，会发送all-region-active消息，然后FaRM才会开始data recovery。
- 通过小的块读，8KB block，来降低正常操作的影响，通过随机读时间间隔，来降低影响
- 并且每一个对象在读之前要先做检查，看是否version number已经stale，是的话，更新

5.5 recovering allocator state：

- FaRM allocator把region分成1MB的block，作为slab，保存这两个meta-data：block headers：包含了对象大小，slab free list。前者在backup上有replica，用来当primary失效的时候做恢复，并且当primary收到new-config-commit的时候，会立刻把block header进行分发，避免不一致。
- slab free List只在primary上，object header中有一个bit被用来记录，当primary failure以后，可以通过扫描对象来做重建，并且为了不影响前台进程，会限制回收的速率

#### 问题 unfinished：
-  FaRM中似乎，一个object可以放在多个不同的primaries上？
-  文章好几次提到了false positive，然后，是个莫子意思？
-  Failure Recovery的过程还需要细细的品味。
-  感觉整个系统过于繁琐。
#### 来自[preview](https://pdos.csail.mit.edu/6.824/notes/l-farm.txt)
- 90 million **replicated， persistent，transaction** per second， 9000万的事务每秒太强了
- 高性能的原因：1. 全内存 2. non-volatile ram 3. one-sided RDMA 4. fast user-level access to nic 5. 事务和replication protocol 都利用了one-sided RDMA
- 一般的网络通信需要经过网卡, 网卡驱动，tcp(内核)，用户。包含了系统调用，消息拷贝，中断等代价，如果RPC double
- RDMA: app 访问流水线化的，直接与NIC通信(没有系统调用)，共享内存映射，
- 只有primary处理read，剩下的f+1 see commits+writes
- version number有一个lock bit 放在version number的高位置。
- TC(Transaction coordinator) 的commit primary 只等待硬件的回复。当发生commit-primary 的时候，copy new valueto object's memory 然后increment object‘s version, clear ojbect's lock flag.
- 要求是收到人一个commit-primary的ack以后，是为了实现f+1个region。达到了f fault tolerance。
#### FROM [FAQ](https://pdos.csail.mit.edu/6.824/papers/farm-faq.txt)
- 是说这个系统是只能用在single data center instead of geographically distributed data.
- FAQ中指出了FaRM可能的缺点。
- polling：轮询
- FaRM的高性能虽然很大程度上依赖于硬件，但是硬件已经存在了 很久，FaRM还是第一个把这些硬件给连接在一起的
- 我们说RAFT是一个纯软件的实现，但其实是可以利用硬件比如说非易失性的硬件来做的，这样就可以对于RAFT进行一些修改。
- FaRM 使用ram存储所有的数据，只在power off 时把数据写入到SSD，然后，power on时恢复
- FaRM中的三个角色，primary，backup，CM，有点类似GFS
#### 有空在读一遍，还有蛮多的细节需要考虑。