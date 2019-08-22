### Zookeeper总结

#### MAIN：

- wait-free aspects of shared register with an event-driven mechanism ：wait-free是指慢的client不影响快的client执行？
- 给每一个client提供请求执行的FIFO队列保证，以及所有修改状态机的请求的线性性
- 实现了一个高性能的处理流水线with read request processed by local servers
- 支持用户定义primitives足够灵活。
- 流水线架构：高性能并且低延迟。pipeline支持client request 的FIFO处理，而FIFO又支持了client请求的异步。
- process会缓存当前leader的标识符。
- 主要的地方：
  - 协调内核
  - 协调的basic 原语
- 由zookeeper client lib支持与server的通信，这个地方应该是一个长连接？
- 有两种节点：regular与ephemeral，前者需要显式修改，后者可以被自动清除。临时节点不能拥有children。节点还有sequential属性。
- watch机制：watch只支持一次触发。watch机制避免了客户端的轮询。
- Data model：方便为不同的app分配子树并且设置访问权限
- ZK并不是设计来保存大容量数据的，但可以用来存储一些小型的数据如configuration meta-data，并且ZK将meta-data用timestamp和version number来标记。
- 分同步与异步版本的API。ZK client保证相应的callback to each op are invoked in order
- **ordering guarantee**：1. linearizable writes 2. FIFO client order。前者保证所有修改ZK状态的命令执行都是串行化的；后者保证client发出的清楚的处理过程与他们被从client发出的顺序一致。
- 所有满足线性化的对象都满足A-linearizable对象，每一个读都可以到replica上读达到scale linearly 
- 利用ready 来实现分布式锁。**存在这样一种临界的情况，一个client读到ready exist然后读configuration的时候 ready 被删掉了，Watch机制可以保证ready在被删除的时候，通知原来读configuration的client**
- 使用sync来同步最新的数据。ZK保证一旦update成功一定会持久化。
- 通过ZK提供的服务可以支持 configuration management、rendezvous、group membership、简单锁，读写锁，double barrier等原语。
- **ZK实现：**
  - 使用ZAB协议，in-memory DB，周期持久化。并且使用replay log(write-ahead) 来记录committed op和snapshot
  - client连到server上，read本地产生，write要转发到leader处理。
  - 事务是idempotent的。
  - Atomic Broadcast：make pipeline full to achieve high throughput。**ZAB协议还需要加强。这里的zab保证leader broadcast changes 会按照他们送达的顺序并且前一个leader所有的变化都会在新leader分发自己的变化之前分发给新的leader。**
  - zab依靠TCP和zab leader来简化。
  - 在recover的时候，可能会发生重复的deliver。
  - ZK的snapshot是动态的，不阻塞请求，在这个过程可能会出现snapshot以后拿到的状态image和snapshot结束的时的状态不一致的情况，因为apply 是idempotent的，所以只要记录下从snapshot命令开始时的执行的命令，recover时在snapshot的基础上，重新执行记录的命令，就可以达到恢复原来的状态的效果。
  - watch机制：使得leader在完成了写操作以后，立刻会执行发送消息然后清除消息的操作。
  - zxid：全局的状态机修改记录ID，global并且唯一。
  - 对于client发起的sync请求，client会将其发送给leader，然后leader放在请求队列中，这样sync前的命令就会获得最新的结果。这里关于leader是不是还是leader有个提交NULL transaction的方法，这里ZK使用timeout实现。
  - leader和server之间通过一般的HB来保持连接。HB中也会携带LastZxid来确保自己是不是最新的。

#### SOME KEY POINTS FROM  LECTURE

- ZK支持异步，**和pipeline？**
- Ordering guarantees：1.  all write operations are totally ordered 2.   all operations are FIFO-ordered per client。
- ZK不是一个End-to-End的解决方法
- table to detect duplication
- **CHALLENGE：**能否让replica当做leader：
  - 如何确保不收到stale data
  - 能不能在replica判断最后一次的值变化，如果值不变化，由replica提供，否则询问leader

#### FAQ

- Pipeline 使得client可以异步执行命令而不用阻塞，pipeline中的命令执行完了以后，leader会调用callback函数通知client，方便client进行下一步的操作。这个地方存在的问题是**指令重排**，也就是client发送的命令可能会被乱序执行。ZK为了避免这种情况发生，在client端实施了FIFO队列。
- Wait-free完全不等待，注意到在FAQ中提到即便是server处理完client请求以后的通知也同样是wait-free的，不管client是否收到消息。

