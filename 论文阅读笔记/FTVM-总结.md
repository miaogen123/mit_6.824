FTVM总结：
用于要求很高的backup system中。
1.   **Introduction：**
     FTVM特点：带宽占用低，允许远距离传输。
     采用状态机来实现P/B，因为完全的主从备份会使用相当的带宽。
     目前只实现了单处理器的P/B，因为多处理器在使用的过程中，每一次的共享内存的访问都会造成不确定性，造成非确定性的操作。
2.   **Basic FT design**
     两个Virtual disk  都在同一个shared-disk上, Virtual lock-step disk收到primary的读请求会在一定时间以后(lag)发送给backup，同时也保证了两者一个时间只有一个工作。使用logging channel进行通信，对logging channel流量的监听可以判断两者之间的连接状态，
     **Deterministic Reply Implementation：	RSM is the kernel. **非确定性的操作virtual interrupt and the read of clock cycle counter of processor, 对于P/B的三点要求：1. 所有的input including(deterministic or non-deterministic )都要被捕获。2. 并且要求正常的apply到backup主机上3.这个过程不能消耗过多的性能。backup通过relay log  file来重新执行。相较于其他的在epoch的末尾进行转发的办法，FTVM对于中断采取了立即转发的办法，因为这样效率最高。
     **FT protocol: ** 
     基础要求：Output requirement：当backup VM接管了执行的时候，对外，backup的输出与primary没有failure之前是一样的。且：PVM直到BVM收到了output to external world 并且commit以后，PVM才输出这些信息。
     只要满足上述要求，一定的状态不一致是可以存在的。这样的要求可以保证即时PVM在failure时BVM对外是一致的状态，每次的commit都可以看做是一个checking point.
     **Detecting and responding to failure：**保持BVM的log-consume 与之前的primary保持一致。BVM上位以后，要advertise 自己的mac地址以及解决IO的一些问题。通过共享磁盘的test-and-set防止split-brain，
3.   **Practical implement of FT**
     3.1** Starting and restarting FTVMs：**最初启动的VM状态肯定是不确定的，如何跟上PVM的执行，Vmware使用了Vmotion 的移植版：clone a VM and set logging  channel, 这种backup的时间少于一秒。
     3.2 **Managing the Logging channel: **有一个大的buffer有hypervisor进行维护，P和B哪一方执行的慢了就增加其cpu性能，加速IO，使其执行的快一点。由于PVM和BVM都在虚拟机上进行执行，是存在其他虚拟机占用过多的带宽或者CPU的情况的，当发生这种情况以至于影响了系统的执行的时候，可以通过hypervisor的动态调度来调整。来保证execution lag在1 sec之内。
     3.3 **Operation on FT VMs :** 特殊指令的限制，如power off，这时PVM主动的power off那对于BVM就不需要仅需 go live了。P/B之间的操作只有Vmotion 是独立的。Vmotion 不会把P/B 移动到同一个machine上面。在移动P/BVM时要保证所有的IO都是quiesced，对PVM好解决，只要物理IO停了就行了。但对于BVM，BVM对于物理IO本身是不进行写操作的，Vmotion 通过使BVM在执行到switching point的时候，VMotion向PVM发送静默请求，之后PVM会进入到静默IO的状态。
     3.4 **implement issues for disk IOs：**1. Disk read is non-blocking , P/B就可能同时发起读请求，这个时候通过强制的序列化执行来保证不会发生race。关于内存和disk 同时读一块memory block的时候的race，这里是指通过磁盘通过DMA直接访问内存导致竞争，使用在disk operation时对加上页保护，VM访问will trap，MMU设置页保护，成本很高，Vmware采取了bounce buffer的实现。其实就是一个缓冲区，用来缓冲磁盘读写。Re-issue 在go-live过程中，出现的IO请求，防止漏掉，即时多做问题也不大。
     3.5 **implement issues for network Ios：**网络流进入的时候，对于PVM和BVM的状态容易造成不一致，对于异步网路设备的更新，采用了group delay的方法，对于incoming 与outcoming flow都是这样。Tasklet是linux中的一种软中断，用来处理允许延时处理的外部中断请求。Vmware vSphere 允许在tcp栈上注册函数来减少中断。
4.   **Design alternative**
     4.1 **Shared vs Non-shared Disk**: 优点:忽略，缺点：需要额外的同步机制，并且复杂，并且为了防止split-brain的发生还要求另外的第三方的service
     4.2 **Executing disk reads on the backup VM**：默认的disk read是通过 logging channel来发送了，但现在如果使用真正的disk read，会有backup读取disk失败的情况，并且两者都使用shared disk就肯定会存在锁的问题
5.   **Performance evaluation **
6.   **Related works **
     FAQ:
     Bounce buffer: 用来防止读写内存时的竞争现象，bounce buffer用来存储当process正在读写内存时，要写入内存的数据，在执行结束以后，将bounce buffer 的值重新写入内存并通知应用程序，这样来保证两方都是相同的。
     对共享存储的单独使用，来防止split-brain的发生。
Others:
     Hardware performance counter, 用来计数cpu层的相关信息的register，如L1 cache missing rate.
