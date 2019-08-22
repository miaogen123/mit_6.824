### GFS总结

#### 概要：

- GFS的一个非常重要的特点在于他的低成本性，高可用性，简单性，高容错性。他以廉价机的成本，以简单的master-chunkserver的形式完成了一个分布式的文件存储系统，该系统中：设计原则基于：在一个大型的文件系统中，读取的情况要远远大于真正修改的情况，而且，修改的时候，文件的新加入写要远远高于写原文件，这样，可以通过优化record  Append 的方式，大大提高性能。

- GFS中的一致性模型不是非强的，也就是允许延迟一致性。

- 对单文件支持并发的追加

- 有效利用网络带宽，应用中，网络会成为系统的瓶颈所在 

- 对snapshot 和Record append 的支持

- **架构：single primary master –many shadow master，master存储metadata，管理所有的chunkserver**

- **系统中没有使用缓存，因为文件过大，用缓存容易发生抖动。**

- **对于client**来说，可能的策略包括一次请求多个chunkserver**，保存本地，直到文件expire** **或者reopen，但是这个过程中，文件是可能被更新的**。**个人觉得缓存的时候不仅会缓存位置，还会缓存相应的version number，会使用version number进行请求，发现stale cache，就return error code or redirect the request to master**

***

**2.5** Chunk size 采用比较大的块(64M)，有利于降低交互性负载，提高带宽利用率，并且降低master的存储负担。缺点在于：对于一些小文件，并发的读取这些块(may be the same one)会导致这 些块变成hot spot, 解决办法是：对于这些小文件给更高 的replica facotr，错开读。
**2.6**  **元信息：在master中：1. 文件和块命名空间， 2. 文件和块的映射，3. Chunkserver的location，在内存中，高速读写。启动时获取Metadata，避免同步，还有就是chunkserver对于chunk是决定性的，Master只是获取其信息，核心：operating log：log在一切行为之前，记录所有的操作，同时写入硬盘之后才能进行写，日志batches write and 以compact B-tree like 来存储。以checkpoint 为基本传输单元，当完成checkpoint以后，才转发到其他的master去，并且recovery也是恢复checkpoint所在的位置。**
**2.7** **consistency model：**由GFS中master保证。 文件命名空间修改是atomic，master中存在namespace lock。State：defined or consistent.  Master保证record append的原子性，通过master 调度replica 来保证其他replica的顺序与其一致，并使用checksum保证数据完整性，对于client 请求statle chunk，chunkserver返回premature end ，使client 重新与master交互。 Reader 同样会进行Checksum的校验，user 通过library进行通信，通信的细节与相关的跳过padding和replica 的操作都被隐藏在了library中。
**3.1 system interaction：**因为master作为核心，要尽可能的降低master 的workload，primary chunkserver 由master的lease指定，typically it will expire in 60s，如果chunkserver还在写文件，那么就可以向master请求延长
**3.2 data flow :** 避免网络成为系统的瓶颈，primary到 其他replica 的时候，采用单向转发，最近转发的机制，并且使用光纤的全双工通信，接收到数据立马转发。
**3.3 atomic record append : **采用append 的方式，类UNIX的O_append 模式，并发时不会发生竞争。 Gfs遇到不满足固定块大小的块会进行填充，GFS不保证每一个replica在位级别都是相等的，但他保证一旦报告写入成功，那么所有replica的块都有一个相同的offset？
**3.4 snapshot:** master接收到snapshot命令时，会首先取消所有相关块的lease，接着进行备份。
**4.  master opeartion**
**4.1 namespace management and locking：**允许master的多个操作并发，但是对于namespace 会有 相关的lock，GFS中不支持文件别名与链接。其命名空间=lookup table mapping full pathname to the metadata。每一个节点(文件或者目录)都有一个相关锁，这样多个并发对单文件的请求都会被序列化。
**4.2 replica placement：**我记得这个地方，是要求不同的replica放在不同的rack上，并且在replica base设置较高的情况下(视具体环境)，也应减少不同的replica跨网络的情况，因为这样会导致，网络带宽被replication之间的 操作占用，影响整体性能。
**4.3 create，re-replication，rebalancing：**三个要点：1. 平均化磁盘使用率 2. 限制一个chunserver的最近的磁盘写的次数，3. 平均负载， 对于阻塞了client的chunk，优先级会提高，同时也会对一些会造成负载增加，导致不平衡的操作进行限制。
**4.4 garbage collection：**使用lazy collection 策略。对于要删除的chunk，不立即删除，采用了先命名为隐藏文件名+时间戳的方式，在master 对namespace的regular scan 时发现并进行回收，master 与chunkserver之间通过Heart beat 包进行regularly exchange，这样lazy collection 的优点在于：1. Simple 2. 后台运行不会干扰，3. 延迟(lazy )清理，可以避免一些意外情况的deletion操作，对于重复对于同文件的删除操作，采取加快storage reclaimation的办法解决。
**4.5 tale replica detection：**使用chunk version number 进行检测，同时由于master负责所有的控制权，所有的操作都需要经过master，后会对CVN进行修改，chunkserver遇到higher CVN会认为自己出错，同时向master 更新最新的chunk，在应用层，master在在inform client时，会带上CVN，接着client 与 chunkserver 通信时会把 CVN带上，to promise 始终带着最新的文件。
**5.1 fult tolerance and diagnose：**文件存储使用的是容易出错的低可靠的磁盘，所以容错性需要被保证。三个地方：chunk replication。Master replication：chunk replication做足备份就好，对于master replication：chunk 的mutation 操作只有在写入disk并传输给所有的master replica以后，才被认为是完全commited。如果是master在本地重启，一般都是即时重开进程，如果是在不同的机器上重启，则会修改dns使得client永远都可以指向正常的master，这样的话，应该在primary master和shadow master之间有着比一般的master与chunkserver之间的HB更加频繁的HB，因为primary master的 down 状态代价是非常高的，必须保证primary master能够尽快的提供服务。在PM down 状态下shadow master提供的服务是存在lag的，只能提供read-only服务，一般只对于写不频繁，或者对于同步性要求不高情况。
**5.2 data integrity：**由于GFS的loose consistent，允许不同的replica 非bit-identical，所以需要使用checksum作为校验。Chunkserver在读和写(对first and last chunk 进行 check)的时候都会进行相应的校验工作，用来防止提供错误的数据，或者写操作隐藏之前损坏的数据，master在闲时会对inactive chunk进行扫描来检测corruption data，防止corruption data fooling master 以为有足够的replica of a chunk

 

#### 问题 unfinished：

- 如果一个replica 的data 发生 corrupttion，当其恢复的时候，采用的是append还是random write？如果采用append，块的偏移会与其他的replica 不一致，这样是否有影响又该如何解决。

- **对于replica 他保证一旦报告写入成功，那么所有replica的块都有一个相同的offset**？

- 对于很多个clinent来说，多个client可能同时写，在这时就会有针对不同的块多个primary chunkserver ，他们写的时候，不同的chunkserver的初始状态假定相同，写完以后，在进行传播的过程中，不同的replica就可能出现不同的块顺序，所以我觉得这里的**replica的块都有一个相同的offset**，可能并不是真的，应该会有不同的，relax consistent

- 没有给出在master重启的细节，PM与Shadow Master (SM)的关联性，是否存在在master 重启失败，导致等待时间过程，重启有时request 请求过多，