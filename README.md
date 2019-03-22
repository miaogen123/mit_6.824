### 6.824 课程学习 coding
#### 欢迎STAR与交流 miaogen156@gamil.com。QQ群：575983415
***
---
### Lab1-MapReduce:

- 在map端写入的时候，要想让在reduce进行读取的时候，一次性的读入到一个数组里面，需要在map端写入的内容前面和后面加上表示数组的符号'['和']'

- 第一个task在读题的时候题意给搞错了，还以为是程序的问题，浪费了一些时间

- 事实证明，地基要铺好，在做task1的时候，根据key进行分散的那一步搞错了，没有搞清楚key的分布，就卡在task2蛮久，要死了要死了。。。

- **task3如果允许使用队列( 添加/修改 schedule以外的部分):同时队列应该是可并发访问的, 会使实现方便很多**， channel也可以简化操作，但是channel本身有易失性，以及性能有问题，并不是很合适。所以我还是使用了slice(狗头)

- **task4还有待改进的部分，性能和设计两个方面：原因在于对于go本身的特性不甚了解**

***
---

### Lab2-RAFT：

#### task1 LeaderElection：

- **一个raft节点，要有计时的装置，另外一个是要有处理外部的请求。对于RequestVote，需要是并行分发，变成leader以后，进行新的HB维持关系。**

- candidate在进行等待vote的时候，如果收到来自leader 的AE（在进行判断了以后），需要退到follower或者继续candidate，这个问题在于如何同时的做这两件事，两个goroutines 如何做到沟通。**我在这里的实现是通过raft内部设置一个AENotification channel来解决的。当这个channel中有消息时，就意味着收到了其他node的AE消息，然后判断自己还是不是leader，不是leader就退出leader过程**

- **选举超时的选择：256-512ms的区间，考虑到选举超时应该远远大于RTT，这里因为labrpc的限制 1s 内最多10个HB，模拟的是长延时环境下的election 过程。但也要太大，5s要选出来。**

- **整体的设计思路：在mainProcess中，三个角色循环变化。每一个角色都会监听时间变化，通过select进行多操作的同时进行。candidate并行分发RV，同时监听AE的channel，并且开始时设置选举超时，这个地方使用SELECT进行监听，两个情况：1. AE先来，然后退到follower阶段。2. RV true大于(n-1)/2先来，然后就成为LEADER。如果RV没有接收到来follower的RVR，就需要重新发送，这个时候，如果已经变成leader，那么就不在发送；leader 进行AE(HB)的操作，与follower进行沟通；follower没有明显的动作，超时会进入下一次的选举。**论文中要求刚开始的时候等待一个Election timeout时间，然后才开始进行ELECtion 选举。这样相对于我的一开始就进行选举类似。

**问题：**

1. **Term的超时与计时：对于follower计时是从受到AE的时候开始，超时也同样，所以收到AE就要跳出选举阶段的Select，对于follower没有别的过多的动作，只需要等到TermTimeOut就行，相应的操作由来自leader的RPC调用来完成。**

2. 选举阶段收到AE以后：candidate的动作：判断是否符合要求，符合条件关闭接收RVR的chan，然后把leaderNum设置一下为candidate的ID或者-1，退到follower的状态。每次leader的AE来了就把选举超时的计时重置。

3. **在成为Leader以后，设置一个Term长度的定时器，等到Term时间结束就放弃身份。由leader主动放弃，follower在ElectionTimeout时间内没有收到的情况下就转到candidate状态。**

4. **文章第4页的图中，RPC返回消息中，都没有添加自己的ID，我觉得需要添加一个ID，因为需要一个来使leader更新自己对于follower状态的一个消息。(文章只是给了很基础的结构，很多东西都需要自己去完善)**

5. **一个term内最多只能vote one node。如果本身是candidate并且对方的log比自己的新，这时候是可以在vote给别人的。**

6. **AE中在对preLogIndex进行查询的时候，是一个O(n)的操作，考虑到RAFT是时间敏感的，这里应该使用快速的算法查找存在性。**

7. **AE的判断与处理是一个相对来说比较复杂的部分，多个条件需要判断**
>   If the logs have last entries with different terms, then   the log with the later term is more up-to-date. If the logs  end with the same term, then whichever log is longer is   more up-to-date.      文章第8页
---
#### task2 Log Replication：
**主要任务: 完成keep a consistent replicated log of operation**

- **election restriction:** 也就是大多数机制，通过大多数机制阻止没有获得上一个commitLog的node的选举。
- **时间上的建议：real time 不超过1min，cpu time 不超过5s**
- 对于log的commit：**通过对rf.matchIndex进行排序，取len(rf.peers)/2索引处将要commit的log**
- 实现的log replication的时候，考虑到测试中command的命令都很短，不在关键路径上面，在node收到AE时没有进行preLogInd和preLogTerm的比较，而是直接进行replace，如果command比较大或者说操作比较耗时，就需要比较然后replace
- 这里AE在被reject以后，是存在回滚的机制：返回到没有conflict的那一个阶段的
    -   easy way: leader 对被reject的AE依次递减在发出直到leader中的记录的follower状态与follower相同步，然后在对那个一致的状态节点开始。
    -   **optimized way: follower 返回不一致节点的Term信息，以使leader跳过那个term，同时leader在初始时设置一个term的最后一个index的缓存，这样方便在进行log backup 的时候快速查找。**
    -   **这里有一个实现上的问题，leader在繁忙时一个请求接一个请求的时候，如果follower的状态发生比较大的不一致，这时会进行会滚。这时会滚的操作是耗时的，下一个请求再来(slow node 不影响请求的处理)，会新开另外一个go routine，也同样会进行会滚，前一个goroutine 会快一些，最终会趋于一致，但是如果会滚的速度没有请求过来处理的速度快，可能会造成goroutine不断的增加。这个过程同样会因为导致一些数据同步的问题**
- 一直没有注意，make传过来的applyCh 是用来进行验证的，成功的消息要发送到这个通道里面才能被config进行了解并验证，所以在每一个RAFT节点内部都内置一个channel，把传进来的applyCh赋给该内置channel。 这样当消息commit以后就把applyMsg发送到那个channel中去(这样需要考虑persist以后readpersist时的channel关闭的问题)
- 当leader收不到其他的消息时而超时时，应该关闭其所产生的AE分发的goroutine(通过指定的尝试次数，尝试了一定时间以后就选择结束goroutine或者通过在RPC调用前面判断自己是不是leader，不是就退出过程)  
- 对于Leadercommit的更新应该在HB和AE分发的过程中都有，这就要求在HB的entries判断为空并返回之前进行。(设置如果prevlogindex和prevlogterm都匹配，就一直commit到最新的commitIndex的位置):。**为了避免一次commit 出错，这次的添加数据需要由下一次(更新leaderCommit以后)的HB或者AE进行更新**
- **leader对于command，应该是马上就附加到log上的**，之前是在完全commit以后才进行的，哇，蛮多的东西要改，因为直接加在末尾的话，是可以不需要那个Raft中未执行command的channel的。
- 原来的Raft定义的commandTerm是小写的，不能够被外部所访问，改成大写的方便访问。
- follower应该对HB/AE中的leaderCommit进行判断，并且适时的commit自己的logs
- 对于卡着一直不能被commit的命令，后面的命令也能来进行AE，但是LeaderCommit没办法向后更新。
- 在leader和candidate函数中，如果因为收到更高的leader的消息然后退出，那么所有的goroutine也会因为自己不是leader而推出。
- 并发导致的问题：对于node，rpc调用和 node主要执行函数(leaderProcess)及goroutine是同步进行的，这样在rpc中修改的东西，可能会立刻反应在goroutine中导致程序失败。还有node主要执行函数需要在关键的操作之前如AE分发，判断当前的身份，避免已经不是leader了，还在执行leader的动作。
- 这个过程中的并发操作是真的不好操作，多数情况下，AE和RV的分发程序，以及AE和RV的响应RPC，有些操作是不需要特别的加锁的(取到旧值不影响操作)，有些是需要加锁避免出错，特别是在Log replication的过程中。
- 并发操作的过程中，由于调度器的调度，AE_RPC可能出现prevLogIndex已经被commit过的情况，这时在AER为false时应该在leader中进行判断。
- 在leader被隔离的时候，可能会收到别的命令但是无法commit，当leader恢复以后，自身的命令更长，但term小，所以无法被选举，而如果该leader的ElectionTimeout 也比较小，更容易变到下一个term，这个时候，就可能进入一个死锁的状态，该leader先进入下个term并进行RV，其他的node不会vote this leader，但是会update term and go to follower，leader 么有收到vote，首先进入超时，并重启下一次的选举。term又比其他人的大，但是同样不会得到vote，就这样死循环。   **解决办法：在RV_RPC中，当term相等并且当前自己是candidate的时候，如果对方的更新(如下),则vote true and turn to follower**
> If the logs have last entries with different terms, then the log with the later term is more up-to-date. If the logs end with the same term, then whichever log is longer is more up-to-date. 
- 在testRejoin2B中，出现了恢复的节点的electionTimeout过小，由于本身缺失了一部分的log，所以在RV_RPC中无法获得vote，(同上一个描述), **可以采用与论文中描述的不一致的方法：在RV收到的term比自己的大的时候，不是任何时候都去变成follower，只有在voting true的时候才更新。这也是我采用的方法，完美破解这种锁情况**
- 之前没有做好Kill的善后操作导致在开始下一个 test的时候会出现问题。**可以在kill被调用的时候，设置leaderNum为-1，并且阻塞主routine mainProcess的执行（我这里阻塞了很久，足够之后的Test都执行完)**
- **说真的，并发程序真的不好写，一不小心就漏掉了同步**
- **因为并发的存在，在对raft node本身的属性进行修改时，比如commitIndex赋为a，需要先判断commitIndex是否比a更大，如果更大就不更新，注意：判断的过程也应该在锁作用域里**
- raft中命令执行是串行的，前面没有commit，后面也不能commit。前面的如果不commit那么说明没有得到超过半数的同意
#####   Other problems
- [ ] leader在主动退让以后，follower会等待最后的一个electionTimeout结束，leader已经开始了下一轮的election，这样会使follower在竞争中处于不利地位，一直无法获取到leader身份，同时leader也可能由于大量的工作而宕机
- [ ] 有个细节：RV_RPC和AE_RPC两个对term的修改是否需要对相应的计时器进行修改。

---
#### task3 Persist：

- **commitIndex是volatile的，不需要persist。OMG**，这一点会影响之后的一些操作
- 需要考虑那些状态需要persist，一个node恢复以后，只恢复必要的自身相关的状态就好，然后重新进入循环
-  既然需要重新进入循环，那样的话对于leader，就不需要没有commit的多余log了。    **这样不行啊，前一个term的log可能已经完成replicate的过程了，但是还么有进行commit操作，这时需要保留到下一个term，然后通过新命令或者no-op command进行commit**
- AER中可以添加一些额外的字段帮助leader快速的back  up，比如lastLogIndex，然后leader直接从lastLogIndex进行back up(仍然需要使用跨term的方法进行再优化)。commitIndex、TermOfArgsPrevLogIndex，后者由于log backup 的过程中的优化。
- **对于leader刚开始时，commitIndex当有新的命令过来时再进行更新，这也是figure8所要求的：不通过count replica 的方式对上一个term的log进行commit。**对于follower只有在leader的AE过来时，才更新commit。
- 论文对follower的log缺失(AER return false)back up 给的优化办法是一次跳到前一个term的log，为什么不直接让AER携带这些信息呢，应该会更快。**在AER中返回lastLogIndex，当lastLogIndex小于prevlogIndex的时候，设置prevLogIndex为lastLogIndex**
- 对于AE的prevLogIndex大于当前的commitIndex这种情况允许发生，会设置AER结构的commitIndex。AER会返回true，在AER_handler中会处理这种情况，会把prevLogIndex设置为commitIndex重新backup。
- **voteFor也需要persist？？？。实际上并没有用**
- leader对于term的快速定位，当AER为因目标index处的term不匹配而返回false的时候，可以让follower返回对应的prevlogindex位置term（TermOfArgsPrevLogIndex）以便leader快速定位follower的日志的准确位置。**可以在leader启动的时候，设置索引，定位每一个term的开始位置。当AER因term不匹配的问题返回不匹配位置的log时，可以定位到最近的logindex。**
- 同时对于commitIndex的处理，一个节点commitIndex以前的log是不允许修改的，但也不是说AE的PrevLogIndex到了小于commitIndex的位置时就一定要求leader turn to follower。尽量不去碰commitIndex，避免修改以前的东西。
- **commitIndex如果不进行持久化，会出现重复apply的问题。如果恢复的时候带上commitIndex就不会出现这样的情况**
- 注释了ble2C中的延迟RPC返回代码，结果也一样会出错，原因在于leader查找一致点的过程太慢了，整个过程一点都不高效，当follower收到AE但无法成功时，可以返回不匹配的上一个term与index，这样leader内部相当于又执行了一下。
- 在命令返回的计数上，由于并发的存在需要一定的方法来合理的计数，一种方法是将命令的index和leader内部的一个slice对应，当slice的一个index的计数大于majority的时候，把leaderCommit到该index的log全部commit。也不好。**大道至简，每次AERreturn true就做一次检查呗，干嘛搞那么麻烦，最后的解决方案是采取对 matchIndex 的copy进行sort，然后找到len(rf.matchIndex)/2+1位置的index，比原来的commitIndex大的，就选择commit。**
- rf.nextIndex和matchIndex的区别？？？**在我的实现中两者的差别并不大**
- 并发的时候，并不是简单的加锁就可以解决所有问题，**在合适的时间加合适的锁才能得到正确的结果**。再次吐槽，**并发编程真的不是简简单单就能完成的，这东西需要经验的积累**。
- 编写过程中，一些并发访问的地方，let's say 连续两行需要使用同一个值，而该值被并发修改，这时就存在前一行和后一行使用的值不一致。这时就应该用变量记录下当前值，在之后的开发中使用变量，而不是两行都直接访问。
- follower接受到AE的时候，**应该在收到AE的log以后进行判断，如果本地最后一个log的term小于该AE的log的term值的话，就及时的删掉之后的log。**避免出现index:13-20 的term是9，而21-45的term是5这样的导致出错的情况的发生。

**TODO**

- 优化整个过程中的RPC调用：相比于课程页面上给出的例子，我的Pass显得RPC的数量高很多，优化思路，待思考。
- preVote机制实现：[Raft的PreVote实现机制](https://www.jianshu.com/p/1496228df9a9)
- term索引通过二分的方法而不是通过遍历的方法进行建立。一万条log采用遍历的方法需要1ms左右，相对于Leader的周期1000ms来说还是有点耗时的。

#### other Problems
1. 当leader收到其他大多数node的AppendEntryReply(AER)的成功时，也就是大多数成功replicate以后，标记当前log(s)为commit状态，更新commitIndex，然后apply到状态机中，我的实现是在下一次的HeartBeat(HB)或者AE分发的时候，follower才更新刚才的log的状态为commit，如果在leader更新完commitIndex但没有完成向follower发送确认时，leader failed(意外down掉或者收到更高的term 然后turn to follower)，这和figure8的情况有点类似，这次的问题是：原来的leader已经把那个log commit掉了，后来的leader又修改了那个index对应的log，因为commit以后不能在修改，这样原来的leader 恢复以后，没有办法接受新leader的log，也没有办法在成为leader，就卡在这了，怎么破？ **仔细考虑一下，可以发现上有情况在leaderElection正常运行时(即只有一个leader)并且避免了figure8的情况的时候不可能出现**
2. 发现最后的测试过程中收到了nil命令？？：**因为rf.log索引访问越了界，而golang的slice对这样的情况并不会报index out of range**，如下：
```
lenArr=len(Arr);    //say lenArr=10
tmpSlice=Arr[10:12]    //as usual there should be a runtime error
fmt.Printf("%v", tmpSlice)    //actually it runs without warnings or errors. print [0, 0],  heartbreaking
```
3. node down掉以后，在重新变成leader的时候，因为leaderCommit为0，所以会将commit过的log再次commit。**这样会不会有什么问题？？？：不会，状态机来记录commit的内容，如果发生重复就不apply**
**lab2C中的并发同步是最让我难受的了，难怪网上有人的做法直接在RV和AE相关的里面全部加上锁，最后离开时释放，直接将游戏难度从abnormal拉到了easy。**
并发编程真的好蛋疼啊。。。但完成以后----
### 真香

---
---

### Lab3 KVservice

####  Task1 简单的KV 	
- data race需要解决 go test -race 
- 关键问题：
    1. 请求正确的leader server
    2. 状态机正确应用，(no duplicate request should be applied)
- 这样几种出错的情况：
    1. 对方不是leader
    2. reply超时丢失
    3. reply的err不为空，出现错误
    4. 对于一个kvserver，由于分区的存在，可以误以为自己是leader，此KV可以接受请求，但因为无法获取majority，而请求超时。
- 实现笔记：
    1. 两方通信：get和put
    2. 然后简单状态机：由rf 发送的commit消息来修改或者删除KV，可以使用DLC(double lock check)来检查applyCh中的内容避免出现重复的发送。(比较明显的是append，这里如果append发生了重传那么一个key对应的值就会出现多个)
    3. 但Raft的leader为-1时，这时处于一种中间状态。应该等待一些ms然后换一个进行问询。
    4. 对于消息的监听，这里应该是一个while循环，while循环里进行判断，在一定的时间内重复的监听的，如果commit的index大于调用start返回的index，就认为请求已经成功，返回结果。
- KV service 这里就用简单的map来实现？先不考虑LSMtree结构
- **log内如果加上command+ID拼接的字符串就可以避免状态机重复的执行。每一个client的ID由leader进行分配。**
- 这里就要求每一个client在给kvserver发消息之前先获取一个ID。**这里像是TCP的窗口，一定时间内只有特定窗口的值是有效的。**
- 要求是同步API，所以这里server端对于每一个请求都要等待commit结束才返回。并且每一个请求都应该有一个ID，由server记录，映射到相应的clientID上。确保一个重发的命令不会执行两次。**使用map映射clientID和对应的请求**  
- 3A的描述中提到**最好从开始就加锁**，言虽如此，但是**自己加锁也更加锻炼自己，何况，之前的自己也做过了蛮久，也可以把之前做的时候遇到的一些经验放在这里，聊以锻炼**
- kvserver应该在后台有一个线程用来listen change of log。
- 有时间的话，RAFT的大论文可以拜读一下，相关的优化以及细节作者都已经想好了。
- 大论文里提到优化：**client的ID是通过RegisterRPC来实现的，**，关于何时expire一个session&recycle clientID，1. 设置一个LRU策略，当达到最大值的时候，取消最远的那个；2.把过期时间写在log里，然后由大多数来同意。(live client在空闲时为了避免被清理，也会用keep-alive request来更新时间)
- 对于client已经expired的request，leader会拒绝处理，返回错误，并要求client申请session。
- **优化的部分等到lab3完成了以后再来做吧。**
    -  如果强制要求所有的client都有一个不重复的clientID的话，那么一个注册分发服务肯定是必须的。 
- ~~这里使用registerRPC的方式来注册ClientID~~，ClientID使用随机字符串来表示自己的唯一身份，虽然可能会有可能重复。(**使用RegisterRpc，可以避免ID的重复，待做**)
### Optimization TODO：
- Lease 机制实现
- Read 不落盘
### 问题
- 如何清理clientId呢？根据什么清理呢？超时？ID值？大论文给的LRU策略和使用时间戳的方法，来维持session，对于short session，就可以清理掉。对于keep-alive的session，会使用request来维持session

---

####  TASK 2 实现支持snapshot的KV
- KVserver检测state的大小然后适时地saveSaveAndSnapshot, raft在persister中保存这些东西。
- installSnapShot在follower请求leader已经discard掉的数据的时候，就需要发起installSnapShot请求。follower收到该请求以后，拿到锁然后install  snapshot。
- 利用applyCh来传递installSnapshot的原因应该在于慢。raft收到installSnapShot的时候，是直接修改raft的log的，也可以向applyCh中一个个的进行修改，但这样应该会比较慢。这样还得把相应的clientID给传进去。
- snapshot有raft层来完成，service层还有排冗余的map需要传给raft层
- 还有一点要考虑的是，当你的旧log已经除去的时候，你就需要在原来log计算lastLogIndex的位置上加一个offset，并且在读的时候也要加上判断。所有的logIndex相关的地方都要做。
- saveStateAndSnapShot的时候也需要注意并发的情况，如果在保存的时候还有其他的在访问，需要确保进行truncate的时候，没有别的在读。**关键的获取log的地方加锁来保护**
- KV service每来一个applyMsg就判断一次大小，然后如果超限的时候，就发起snapshot，并把自己的clientID和request的map，和rsm传给rf 层。**这个地方，在我的实现里kv 通过协程来完成snapshot并且一次只能有一个进入snapshot阶段**
- 需要在raft启动之前readSnapshot
- 处于性能考虑的话，很多地方最好加读写锁，而我没有加。
- getLastLogIndex的地方，可能会发生正好在snapshot的时候，造成读数据出错。
- **立个flag：这部分大概率会因为添加getLastLogIndex而DEBUG很久**
- 这里有不兼容的地方：默认没有snapshot的时候firstindex是1，如果snap以后log从第一个开始算的话，会有比较麻烦的地方，因为前后不一致而不好算。**所以每一个log都默认第一个不是有效的，这样在合并的时候也需要考虑清楚，在合并的时候，返回新的log的时候，会把第一个设置成nil。**
- **读的时候需要加上lastIndexInSnapshot，写的时候又需要把这部分给减去，而log又是操作的核心。有点难受啊**。网上有人的实现是通过这样，在log中记录相应的logIndex来实现。
- 在getTrueIndex中，因为第一个log是上一次snapshot的最后一个，所以可以考虑在getTrueIndex为的值为负的时候返回0。这样不至于出现panic。就是得考虑正不正确。
- 这里限制的snapshot的大小是每一次的snapshot的大小？如果是这样，那为了back up之前缺失的很远的部分(前一个snapshot中么有)，该怎样解决？**傻了傻了，snapshot是针对kvserver来说的，而不是针对raft的，snapshot的主要作用是删掉旧的log，保存kvserver的状态，installsnapshot的时候也是这样的，主要是传递kvserver的状态，而不是log的，旧的log已经没有用了。**
- 读raftpersist和snapshot的顺序也需要注意，因为snapshot是完全的重写，所以要是先读的raftpersist，前面的状态就丢失了。
- 在返回命令处理结果的时候，需要注意在状态机内部直接返回而不能等待一段时间再去查询，因为状态机执行的很快，可能后面的命令又会覆盖掉这条命令的结果。**采用32位clientID和32位sequID合起来一起做命令的映射，并等待结果。**
- rf.log 与snapshotindex相关：内部写动作用trueIndex，读动作用nameIndex。
- 原来的这部分需要在进行back up的时候，是需要用到termList的做优化的，但是这里因为snapshot以后，log已经发生了改变。这里就需要重新来做一次索引。
- 发现在锁的前后比较容易发生调度，也可能是这个地方的调度容易看到。
- apply这里发送有点麻烦，因为apply发送给kv，kv中会调用snapshot，snapshot中会加锁，如果applyMsg在锁进行，可能会发生死锁，但是不加锁的话，又需要保证顺序，这部分需要改一下。kv service使用go routine来进行分发了。
- 对于非leader来说不用保证commit的连续顺序，只要保证状态机是一致的。
- **有时候会遇到commit的消息是nil的情况，按道理来说是不应该出现的。这里应该是snapshot的问题，commit是nil，还是从leader端进来的时候的问题，leader访问index的时候遇到了0那个index，这个时候应该sendsnapshot进行更新follower的状态的，这个地方问题应该是这样的**
- 有时会频繁的发生snapshot。因为这里按照kv收到的commitIndex来判断大小进行snapshot，如果commindex占的整个log的比较小，就可能频繁的发生。(遇到过2个log就发生一次commit的情况)
- **小概率会发生snapshot导致的问题：比如说正在读使用getTrueIndex(i)读log，然后发生了snapshot，这个时候，i的值可能会比rf.lastSnapshotIndex要小，然后getTrueIndex返回了0值，读log就从头开始读，这样就返回了不该返回的值，而这个时候本来是应该进行sendSnapshot操作的。需要彻底解决。**
- 果真前面立的flag没有倒
- **这里应该考虑一种方便的解法来解决，index越界的问题，特别是getTrueIndex的位置。如何设计一种方式能以一种比较稳妥的方式来做，我实现的是对于小于0索引返回0，但是这样其实是有问题的。**
- installSnapshot的时候需要把snapshot的状态保存起来。避免下次crash以后读到的snapshot不是最新的。
### 遇到的并发的问题
- 执行顺序或者说同步导致的取到旧值的处理问题。有的地方取到旧值没有影响，有的就会直接宕掉。
- 共享数据的访问变量存在时间太长，使用该变量再去访问时，数据已经发生了变化。
- 这个实验中没有要求实现snapshot offset，当然是用分片是一个非常好的选择。
- 在truncate的时候，第一次测试20个log左右会进行一次snapshot，我之前的实现是在一次AE impied的所有该commit数据都commit以后才更新commitIndex，因为kvserver收到 applyMsg就会判断是否调用snapshot，所以会先调用snapshot，两者之间先后顺序不同导致的问题。
- 死锁的问题，三个部分：raft follower node commit的时候会获得锁，在锁占有期间同时会向channel发消息，发完消息，另一部分进行处理，另一部分处理的时候也会去争锁，然而锁被commit 部分占有，而commit部分又在等待消息发送(由于另一部分的阻塞，消息处理已经阻塞)，就这样形成了死锁。
- 锁内进行二次判断也是有必要的**DCL**
- 锁内需要用到的变量最好在锁内获取，避免读到旧值。
- 前后需要保持一致性的即同一个versiond的对象，最好加锁。
- 读取数据的时候为了保持一致，该加锁的地方不要吝啬加锁，正确性保证了才能有性能的说法。**FLAG：下次再因为这样的问题出错，直播吃xiang**
- 在leader发送AE的时候，可能发生snapshot导致log的长度发生变化，并且由于getTrueIndex在超过下限的时候，会返回0值，导致发送给follower的log变成了并不应该拿到的。
- 并发存在的地方：
    - snapshot，RVrpc，AErpc，sendAERPC(有，(sendRVrpc是只读的，所以没有问题)。
### others 
- golang 有move语义吗？(没必要，map和slice都是引用)
- go test -c 编译的文件里面没有符号表的信息
- termList(索引)的部分在进行snapshot 以后需要进行在一次的重建。
- **getTrueIndex在发生下越界的时候，考虑直接返回，这样在索引的时候会报错，如果发生错误，在recover的部分发送snapshot**


### 重复client的请求：
- 防重放：时间戳超时无效，nonce作为标记。两个结合用来防止重放的攻击。
- 那和这里的client请求的区别在一个client重复的请求，上面的请求是可以避免这样的方式的，问题是一个请求，如果已经执行成功，但是client误以为没有成功，那么client会重新发送，然后，client的请求一直没有成功呢？
- 这个地方其实需要一个安全级别的定义，要想达到安全级别需要怎样的处理方式。强的安全级别一定会出更加复杂的方式。

### 小概率3A和3B的线性化测试过不去