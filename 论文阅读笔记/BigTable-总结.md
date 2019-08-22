### BigTable阅读总结
Abstract：
主要是用来管理结构化数据的分布式存储系统，之前GFS是分布式文件系统。可以理解为是一个数据库, 但不完全是一个数据库，不支持full relational data model .使用行列值(任意string)以及timestamp 作为index查找数据

1. Introduction:
    实现目标：wide applicability, scalability, high perfromance, high availability. 允许client 决定数据的存储位置MEM or client

2. Data Model

   - Sparse，distributed ,persistent multi-dimensional sorted map. 注意是sorted，BT中数据存储是有序的，
   - Row:对每一个row key下数据的读写都是atomic的(与该row 中正在被读或者被写的数据无关), 由一个row key range 组成的 多个row 组成tablet，tablet是load balance 和distributed的基本单元。通过将相近key进行局部存储，优化locality
   - Column families:由column集组成的，是访问控制的基本单元。最多几百个family，列命名：family:qualifier 。
   - TimeStamp：用64位整数代表毫秒值，代表时间戳。支持基于timestamp的策略设定

3. API
    支持普通的操作以外，还支持单行的事务操作。支持client script 操作。

4. Building Blocks

    - BT内部使用Google sstable：persistent， ordered immutable map.支持通过索引进行二分查找，并且Sstable可以完全放在Mem中。

    - 依赖chubby，进行文件修改的lock，Chubby：使用Paxos共识协议，five replicas使用session 访问chubby，允许client 注册文件修改通知。
      Chubby作为一个中心的master，起的作用类似于GFS的master，发现与终结tablet  server;存储BT schema info( column family info)；存储访问控制列表

5. Implement
    Talbet server对于本地的tablet的增长到一定程度以后，会进行split(默认 100-200MB)，不需要master干预。在BT实现中，client几乎与master进行通信，所以master 的负载是很低的，

    - Tablet Location：
      使用类似B+树的三层机制存储location info：第一层：root表，存储所有表的位置在一个METADATA 中，并且never split。Client library 缓存Metadata。细节：在cache为空时三次round-trip，miss时需要六次round-trip。为了提高效率，使用了prefetch。

    - Tablet Assignment：
      BT使用chubby 通过监视文件目录（server目录）来观察新的server接入，server  的操作在于取得lock与释放lock，当chubby中的文件（server中自己的文件）不在存在时，相应的server会认为自己已经被master removed ，会自己kill 自己。Server目录下的相应文件的锁是server存在与否的一个flag，如果master在无法到达server的时候，会尝试获取相应的server/的文件，一旦获取，就删除原来server的文件和相应的tablet assign，并重新分配。**为了保证BT cluster不会因为master与chubby之间的网络问题，而降低性能，一旦master的session过期，master self-killed。**

      Master重启后的操作：1.获得master lock in the chubby，2.扫描server 目录发现live server，3.与live server communicate 发现tablet 分配情况4. 扫描metadata表，发现unassign table，并记录，之后重分配

      对已经存在的existing tablet的改变，发生在 table 创建或删除，两个tablet合并或者单个分割，对于split操作(由server完成),server 会进行在metadata表中进行commit the new tablet, after commiting ，server notify the master，in case the notification is lost， 会在master通知 load 被切割的tablet的时候通知master of the split.

    - Tablet Serving:                        

      Memtable:用来存储最近commit的change，方便进行快速的restore。写操作需要预先授权，授权有chubby 的权限文件决定。读类似，并且文件读写可以在tablet在split和merge时候进行。

    - Compactions：
      随着memtable的渐渐增加，需要对memtable进行压缩，作用：1.减少内存占用2.减少restore过程中需要执行的操作。每一个minor compaction 会创建新的Sstable。。。**？？中间有段不明白的数据。**

    - Major compaction可以用来回收资源。但是它rewrite all the sstable into only one？？？
      那major compaction以后产生的table大小岂不是要炸？？

6. Refinement

    - Locality: key排序相对较近的放在相同的位置。用户可以指定存储的位置。
      Compress: 使用两种算法 ：bentley/macIlroy  压缩算法，以及针对小文件块(16K)的压缩算法：就近存储相似性高，导致存储的压缩率很好。
    - Caching for read performance：使用二级缓存的机制：一级缓存metadata，二级缓存实际 块。
    - Bloom filter：使用布隆过滤器完成读的加速。
    - Commit-log implement：针对tablet server 进行每一次的log。Instead make - - - log for every tablet，同时为了避免读取不需要的文件，在commit log内部使用了有序的排列。并且为了防止GFS造成的性能hiccup，需要使用两个线程对各自的log进行读写。Recovery在使用两个log file的时候，采用去重。
    - ** Speeding up tablet recovery : move的时候使用二级压缩的算法，一次压缩在move的时候，压缩完了把文件传给对方，然后stop serving this tablet . 在 unload this totally 之前，再做一次compaction。然后把log传给另外一个server。：这个过程不是很细，然后，不会很明确。**
    - Exploiting immutability：Sstable是不可修改的，而memtable则是可以修改的。也正是这样才无需加锁，更加高效，子表可以与parent表共享Sstable
      总结：中心化master，多server 是一种简单的架构，而且容易实现。但master故障会造成整个系统的宕机，所以GFS一个设计原则就是降低master的负载。在BT中，master 的宕机则并不是大的问题，master并不是以个center
#### 问题
1.  Client如 何得到相应的tablet的位置？
8. 当tablet server的状态发生变化如何(如tablet split)，master如何更新信息
9. 对每一个row key下数据的读写都是atomic的(与该row 中正在被读或者被写的数据无关)为什么这样设计？原则是什么？
10. 权限管理以family 为基础单位，应该是基于user的权限？
11. Tablet unassigned是什么东西？tablet不应该是由server进行 管理呢。
12. Root metadata 的初始是如何创建的？
13. Tablet 的load 操作是为什么要进行呢？
14. Library中的cache是从metadata中创建的？
15. 压缩过程的sstable产生若为unchecked，会使得读数据？？？5.4第二段
