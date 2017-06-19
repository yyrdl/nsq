## nsqlookupd

`nsqlookupd` is the daemon that manages topology metadata and serves client requests to
discover the location of topics at runtime.

Read the [docs](http://nsq.io/components/nsqlookupd.html)

`nsqlookupd` 是一个管理拓扑信息的守护进程，对外提供两种接入协议，分别是http 和 tcp,路由部分见 `nsq/nsqlookup/http.go` 和 `nsq/nsqlookup/tcp.go`


http 部分：

`/lookup`  返回某个话题的生产者列表

`/topics` 返回某个话题

`/channels` 返回某个topic的channel列表

`/nodes` 返回所有已知的nsqd列表

`/delete_topic`  删除一个已经存在的话题

`/delete_channel` 删除已知话题的一个channel 

`/tombstone_topic_producer` 逻辑删除某topic的生产者

`/ping` 健康监测，返回ok

`/info` 返回版本信息

