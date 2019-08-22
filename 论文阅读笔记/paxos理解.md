**From paxos made simple:**

1. PREPARE阶段：Proposer 选择一个proposal number n ,然后进行request分发给所有的acceptor，acceptor如果返回的话，就意味着1.promise：won’t accept any number less than n，2. 返回比n小的最大number。

2. ACCEPT阶段：如果大多数的acceptor response，然后就进入了accept阶段，proposer 选择n 与一个value( 所有response中 number最大的 所携带的值或者是他自己所选出来的)。继续进行分发。

- Acceptor只需要记录(persist)最高的number of proposal就行了。

- 对于Acceptor：只有当其未对更高的number  的proposal进行 responde的时候，才会accept一个 proposal。收到一个accept以后，如果没有对更高number的prepare进行回复的时候才会进行accept操作。

- Minor Optimization：acceptor 如果收到一个prepare request，但是已经有更高的prepare，acceptor应该选择回复proposer。

- Learner  how to know a chosen value：acceptor 向所有的learner进行分发已经chosen的 value。这个过程是一个O(N2)的通信过程。Paxos中使用Distinguished learner来优化这个过程，acceptor 首先发给DL，DL将收到的消息在分发给其他的learner（learner好像划水的）。通常会有了一个DL set来使robust。当部分accepor failed 剩下少于majority的node 导致Learner 无法知道一个proposal是否已经 chosen，他所选择的策略是要求Leader再issue a new proposal。

- 如果每一个proposer都可以任意的进行propose的时候，会造成none of proposal will be accept。所以需要一个leader来作为唯一的proposer。

- **Acceptor** **会把its intended response** **在发送之前就persist****。(but why)

- 像raft中要保证不会有相同index和相同term的log，这里paxos要保证不会有两个proposal使用相同的number。

- Implement a state machine：
  - 在内部设置了状态机。实现了一个sequence of separate instance of paxos consensus。一 个server plays all the roles。Paxos中的chosen value只会在phase2 出现。

- Gap的产生，这里的说起来感觉有点像自相矛盾的感觉。Proposal使用no-op来主动的填补上gap。

- Paxos 中server set 也是写在state machine中。