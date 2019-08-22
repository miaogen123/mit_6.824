#### RAFT总结：
**CA系统，不允许P**
**Two main part:**
**1.electing a new leader **

**2. ensuring identical logs despite failures**

**选举的过程中：old leader may still be alive and think it is the leader**

1. **Introduction**
    简单，保证leader的唯一性，减少了状态空间，来增加确定性，与ViewStamped Replication相似，强一致性算法，使用随机化选举超时，允许热的配置更改（使用joint consensus）算法
1. **Replicated state machine**
    状态机完全相同，log与其他系统类似，都是一系列的命令；对于client端来讲，其所见到的只是一个，raft对外表现的状态是一致的；允许小于一半的机器宕掉；不依赖于时间
1. **Disadvantages of paxos**
    简单来说：太复杂；还有一个原因是没有提供针对于multi-paxos明确的算法；paxos的节点时symmetry的，其提供了weak 一致性
1. **Design for understandability**
    可理解，可修改，可深入。Raft通过解耦得到4个问题，election，log replication，safety, member ship changes. 还有一部分是非确定性的引入但简化了问题的解决。
1. **Raft consensus algorithm**
    **RAFT的五个属性：election safety， leader append-only, log matching, leader completeness, state machine safety.**
    **BASIC: leader handle on all the request from client(leader不累吗。。。)**
      以TERM驱动的leader的存活期
      只有两种RPC ，requestVote与Append-only。 最开始在Term的值应该是最开始存放在配置文件中的。并且在通信的时候一直都带着。
    **Leader election :** 核心是随机化选举超时时间。三个状态，leader，candidate，follower，利用没有内容的rpc作为HB 消息。选举过程以：成功，不成功，超时三个状态。Term最大的是作为leader。
    **Log replication :** AppRPC中的term number是用来确保一致性。Log以后当大多数replica都变成commit状态以后，leader commit it to state machine。Leader中保存了follower的状态，follower的下一个log，下一个commit 应该是的index。（Log Matching property）:通过append 命令确保（leader 发的app请求中包含前一个commit的ind和term ）。当follower状态与leader不一致的时候，leader强制follower的日志与leader同步，当app请求的一致性检查失败的时候，leader中维护的关于follower的状态会回退。 **对于leader和follower状态不一致的情况，可以让follower发送给leader 当前term的第一个log和矛盾的log entry。然后，可以快速的回退？ 使用的方法：leader对于failed 请求，把所有的log converge 聚合在一起。**
    **Safety：** 状态机安全：（大多数同意的机制保证当前Term commited 状态的log在下一个Term一定是存在的）
    **Election restriction ：** 在ViewStamp中，允许没有收到所有的commit的数据的节点来当选leader，这样需要很多额外的措施才能保证系统的一致性（ **HOW** ）。Raft中由于超半数的确认机制，可以保证绝大多数的同意，这样可以抑制commit不是最新的人当选leader（这样看来他跟vr的机制还是有蛮大的差距的）。通过比较index与term来确认是不是处于最新的状态。
    **Committing entries from previous term：（分叉的处理）** 前一个leader的commit请求没有及时的扩散，如果它已经成功的扩散到了大多数的nodes中，那么后一个leader一定会收到该entry，但是如果前一个leader的entry没有扩散到大多数的节点，那么前一个entry就有可能被废弃掉。
    **Follower and candidate crashes：** 不用担心，通过HB可以发现。
    **Timing and availablity：** RAFT协议不依赖于时间相关的操作，但是其对外的可用性(availablity)却是有关的。广播时间（到每一个server的来回往返时间取均值）需要远远小于选举时间，避免leader选举过程过于波折。选举时间要远远Leader的MTBF ，来使得failure的所占时间远远小于正常工作的时间。
1. **Cluster membership changes**
    采用了一种逐步过渡的办法。使用了一种叫做joint consensus 的机制。
    两个要点：1. Nodes总是使用最新的configuration去做判断， **不管是否最新的log entries 已经commit。** 2. Config 是以log的形式存起来的。
    存在新旧节点可能相互选举产生一个新的两个不同配置下的leader，raft把新旧的config放在一个log里，接受到的都以新的config作为配置。
    等到大多数的nodes都接受了Cnew 就可以认为Cold 已经不起作用了。
    **三个问题：**1. 新加入的节点会有一段时间的延迟选举期，这段时间内其不能参加选举，直到所有的数据都同步完成的时候。2. 当当前leader不是newcongfig中的leader中的时候，其在commit了新的config的时候，自动退出leader身份 3 .  一些旧的已经移去的nodes可能发送选举请求影响到当前的leader，这里使用了两次选举之间有一个最小的选举间隔，在间隔之内的不进行操作。 **（旧的node在发现自己已经不在config里面的时候应该会自动退出，不然会影响正常的运行）**
1. **Log compaction**

    使用了snapShot机制，把当前的所有的内容全部都写入到日志中，不同nodes之间的snap是互不干扰的（可以自行进行快照操作），对于有一些非常慢的节点由leader发送整个快照给follower。对于leader进行snapshot然后转发给followers 一方面占据带宽，另一方面leader既要实现replica 的功能，又要负责snapshot，实现会复杂。

1. **Client interaction**
    **Linearizable semantics：在clients发送请求与收到回复的时候，系统对外表现为执行了一次only。**   补充：在读spinnaker，线性语义更加清楚了：**不读到旧值，实现寄存器语义**
    Clients 如果联系非leader的 follower，其请求会被转发到leader那里去。通过命令添加序号，来防止重复执行并且应用到状态机。对于read 操作，要防止返回stale data，leader是知道最新的数据的(在第一次commit之后)，为了获取最新的commit数据，leader首先进行一次no-op entry，为了防止在处理读请求的时候，leader本人已经被推翻，leader每次处理之前都要进行一次check by HB。
1. **Implement and evaluation**
    **Performance：像其他大多数的consensus 一样，性能的关键在于leader何时进行log的replica。其他的共识协议有了很多的优化算法，也可以应用于raft。**

***
**Question:**

    1. Replica在把数据写入到log以后，什么时候执行呢，等下一次的log请求发过来吗？ **对，等下一次的来自rpc的commit字段**
    2. 在leader执行完commit了以后，如果带有commit的消息没有到达多数节点，而且安照已有的协议，没有进一步的确定，那么通过怎样的方式来弥补呢？ **重发？**
    3. 疑问，如果客户端只跟leader交互，那岂不是在raft系统中，多个主机对外只表现一个，这样其他的主机仅仅是起到冗余的作用吗，性能不是被浪费了？ **仅仅起到冗余的作用，分布式高性能是并行计算要考虑的东西。**
    4. 选举到一个新leader的时候，对于follower的状态的如何采集？ **在第一次HB的时候，然后follower将自己的状态返回？**
    5. 丢弃前一个没有完成commit 的entry会不会有什么不好后果？：**没有，客户端不会收到确认信息，破天也就是增加了延迟。**
    6. 在新旧的配置文件替换的时候，这个新旧配置文件的替换的命令发起者是谁？特权用户？这个地方似乎有点东西。
    7. 相互之间的通信应该是有安全验证机制的，因为在对外的时候，如果有恶意的机器发送请求，会崩溃掉（确实会，paper中的方法是强调了可信环境下的），不可信的情况下，验证一定是必要的。
    8. Follower在收到APPEND ENTRY的时候需要更新自己的TERM吗？如何更新？**当然需要，收到就更新**
    9. **ViewStamp在进行允许非一致的当选leader的情况下，是如何修复的？**
       10.参考：https://www.jianshu.com/p/92c42018c4f0