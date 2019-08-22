### MapReduce 总结

- 整篇文章从轮廓到细节及性能，讲述了MapReduce这一distributed programming model 方法, 虽然文章中说，google内部在那时(2003)年就已经实现了这一lib，但重要的还是这样的programming model.

- MapReduce模型将分布式的计算通过map和reduce两个操作合作实现，map对应的是task assignment, 从map生成的intermediate data 流入reduce ，reduce 对应的是类似merge的操作，注意：这里是类似于merge，但与merge有不同，一般而言merge操作最后结果是1个，reduce的操作结果则不限于一个。

- 就实现细节来讲，要考虑到master failure， worker failure， semantic in  the presence of failure, locality(单机局部性), (task granularity)任务粒度, 

- 142的backup task 其实是在整个mapreduce程序快要结束的时候，backup所有的in-progress task ，在task primary(主要)结束了以后，或者backup结束了，就把没有完成的任务marked as complete , **然后就可以重新开始该任务了？文章里没有细说。**

- 坏记录处理/跳过(bad record),   本地执行，状态信息(log)， 计数

- Related work中讲述了一些技术(active disk和eager  schedule 等相关的内容)

 #### 问题 unfinished：

141页有一个semantic in  the presence of failure

 