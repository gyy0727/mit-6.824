package labrpc

import (
	"bytes"
	"github.com/gyy0727/mit-6.824/labgob"
	"log"
	"math/rand"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// *请求消息
type reqMsg struct {
	endname  interface{}   //*请求的终端
	svcMeth  string        //*e.g. "Raft.AppendEntries"
	argsType reflect.Type  //*参数类型
	args     []byte        //*参数
	replyCh  chan replyMsg //*回复的消息
}

// *响应消息
type replyMsg struct {
	ok    bool   //*响应的状态码
	reply []byte //*响应内容
}

// *客户端终端
type ClientEnd struct {
	endname interface{}   //*终端的名字
	ch      chan reqMsg   //*要发送的请求消息的通道
	done    chan struct{} //*清理网络时关闭
}

// *具有可通过RPC调用的方法的对象。
// *单个服务器可以具有多个服务。
type Service struct {
	name    string                    //*服务的名称
	rcvr    reflect.Value             //*接收方法调用的对象,服务对应的函数实例
	typ     reflect.Type              //*接收方法调用的对象的类型
	methods map[string]reflect.Method //*注册的方法
}

// *rpc服务器。
type Server struct {
	mu       sync.Mutex          //* 保护服务的互斥锁。
	services map[string]*Service //* 注册的服务，按名称映射。
	count    int                 //* 注册的服务数量。
}

// *维持客户端和服务端的通信
type Network struct {
	mu             sync.Mutex                  //* 用于保护共享数据的互斥锁
	reliable       bool                        //* 是否是可靠网络
	longDelays     bool                        //* 在禁用连接时，发送时是否会暂停很长时间
	longReordering bool                        //* 是否有时会延迟很长时间来回复
	ends           map[interface{}]*ClientEnd  //* 存储各个端点（ClientEnd），按名称映射
	enabled        map[interface{}]bool        //* 各端点是否启用，按端点名称映射
	servers        map[interface{}]*Server     //* 存储各个服务器（Server），按名称映射
	connections    map[interface{}]interface{} //* 端点名称到服务器名称的映射
	endCh          chan reqMsg                 //* 用于接收请求消息的通道
	done           chan struct{}               //* 在网络清理时关闭的通道
	count          int32                       //* 总的 RPC 调用次数，用于统计
	bytes          int64                       //* 发送的总字节数，用于统计
}

// *发送RPC，等待回复。
// *返回值表示成功；false意味着
// *未收到服务器的回复。
func (e *ClientEnd) Call(svcMeth string, args interface{}, reply interface{}) bool {
	req := reqMsg{}
	req.endname = e.endname
	req.svcMeth = svcMeth
	req.argsType = reflect.TypeOf(args)
	req.replyCh = make(chan replyMsg)
	qb := new(bytes.Buffer)
	qe := labgob.NewEncoder(qb) //*新建一个编码器
	qe.Encode(args)             //*编码参数
	req.args = qb.Bytes()       //*将编码后的参数赋值给req.args

	//*发送rpc请求
	select {
	case e.ch <- req:
		//*发送成功，等待回复。
	case <-e.done:
		//* 网络已关闭，无法发送请求。
		return false
	}

	//*等待回复
	rep := <-req.replyCh
	if rep.ok {
		rb := bytes.NewBuffer(rep.reply)
		rd := labgob.NewDecoder(rb)
		if err := rd.Decode(reply); err != nil {
			log.Fatalf("ClientEnd.Call(): decode reply: %v\n", err)
		}
		return true
	} else {
		return false
	}
}

// *创建服务
func MakeService(rcvr interface{}) *Service {
	svc := &Service{}
	svc.typ = reflect.TypeOf(rcvr)
	svc.rcvr = reflect.ValueOf(rcvr)
	//*返回指针指向的值的类型名
	svc.name = reflect.Indirect(svc.rcvr).Type().Name()
	svc.methods = map[string]reflect.Method{}

	for m := 0; m < svc.typ.NumMethod(); m++ {
		method := svc.typ.Method(m)
		mtype := method.Type
		mname := method.Name

		//* 检查方法的包路径是否为空，包路径为空表示方法名是大写的（公开方法）
		//* 如果方法的输入参数数量不等于 3，或者第三个参数不是指针类型，或者方法有返回值，表示该方法不适合作为处理函数
		//* 第二个参数必须是指针类型，表示它是 `reply`（返回值）
		if method.PkgPath != "" || //* 方法是否是大写的?
			mtype.NumIn() != 3 || //* 输入参数数量是否是 3
			//*mtype.In(1).Kind() != reflect.Ptr ||  // 第二个参数是否是指针类型
			mtype.In(2).Kind() != reflect.Ptr || //* 第三个参数是否是指针类型
			mtype.NumOut() != 0 { //* 输出参数数量是否是 0
			//* 该方法不适合用作处理函数
			//*fmt.Printf("bad method: %v\n", mname)
		} else {
			//* 该方法符合处理函数的要求
			svc.methods[mname] = method
		}
	}

	return svc
}

// *执行对应服务
func (svc *Service) dispatch(methname string, req reqMsg) replyMsg {
	if method, ok := svc.methods[methname]; ok {
		//* 准备读取参数的空间。
		//* args 的类型将是 req.argsType 的指针类型。
		args := reflect.New(req.argsType)

		//* 解码参数。
		ab := bytes.NewBuffer(req.args)
		ad := labgob.NewDecoder(ab)
		ad.Decode(args.Interface())

		//* 为返回值分配空间。
		replyType := method.Type.In(2)
		replyType = replyType.Elem()
		replyv := reflect.New(replyType)

		//* 调用方法。
		function := method.Func
		function.Call([]reflect.Value{svc.rcvr, args.Elem(), replyv})

		//* 编码返回值。
		rb := new(bytes.Buffer)
		re := labgob.NewEncoder(rb)
		re.EncodeValue(replyv)

		//* 返回响应消息。
		return replyMsg{true, rb.Bytes()}
	} else {
		//* 如果方法没有找到，列出所有可用的方法并打印错误。
		choices := []string{}
		for k, _ := range svc.methods {
			choices = append(choices, k)
		}
		log.Fatalf("labrpc.Service.dispatch(): unknown method %v in %v; expecting one of %v\n",
			methname, req.svcMeth, choices)

		//* 返回失败的响应消息。
		return replyMsg{false, nil}
	}
}

func (rs *Server) GetCount() int {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	return rs.count
}

func (rs *Server) dispatch(req reqMsg) replyMsg {
	rs.mu.Lock()

	rs.count += 1

	//* 将 Raft.AppendEntries 拆分成服务（service）和方法（method）
	dot := strings.LastIndex(req.svcMeth, ".")
	serviceName := req.svcMeth[:dot]
	methodName := req.svcMeth[dot+1:]

	service, ok := rs.services[serviceName]

	rs.mu.Unlock()

	if ok {
		return service.dispatch(methodName, req)
	} else {
		choices := []string{}
		for k, _ := range rs.services {
			choices = append(choices, k)
		}
		log.Fatalf("labrpc.Server.dispatch(): unknown service %v in %v.%v; expecting one of %v\n",
			serviceName, serviceName, methodName, choices)
		return replyMsg{false, nil}
	}
}

// *添加服务
func (rs *Server) AddService(svc *Service) {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	rs.services[svc.name] = svc
}

// *创建rpc服务器
func MakeServer() *Server {
	rs := &Server{}
	rs.services = map[string]*Service{}
	return rs
}

// * 将一个 ClientEnd 连接到一个服务器。
// * 一个 ClientEnd 在其生命周期内只能连接一次。
func (rn *Network) Connect(endname interface{}, servername interface{}) {
	rn.mu.Lock()
	defer rn.mu.Unlock()

	rn.connections[endname] = servername
}

// * 启用/禁用一个 ClientEnd。
func (rn *Network) Enable(endname interface{}, enabled bool) {
	rn.mu.Lock()
	defer rn.mu.Unlock()

	rn.enabled[endname] = enabled
}

// * 获取一个服务器的传入 RPC 请求数。
func (rn *Network) GetCount(servername interface{}) int {
	rn.mu.Lock()
	defer rn.mu.Unlock()

	svr := rn.servers[servername]
	return svr.GetCount()
}

// * 获取网络的总 RPC 请求数。
func (rn *Network) GetTotalCount() int {
	x := atomic.LoadInt32(&rn.count)
	return int(x)
}

// * 获取网络的总字节数。
func (rn *Network) GetTotalBytes() int64 {
	x := atomic.LoadInt64(&rn.bytes)
	return x
}

// * 创建一个新的网络实例。
func MakeNetwork() *Network {
	rn := &Network{}
	rn.reliable = true
	rn.ends = map[interface{}]*ClientEnd{}
	rn.enabled = map[interface{}]bool{}
	rn.servers = map[interface{}]*Server{}
	rn.connections = map[interface{}](interface{}){}
	rn.endCh = make(chan reqMsg)
	rn.done = make(chan struct{})

	//* 启动一个 goroutine 处理所有 ClientEnd.Call() 请求
	go func() {
		for {
			select {
			case xreq := <-rn.endCh:
				atomic.AddInt32(&rn.count, 1)
				atomic.AddInt64(&rn.bytes, int64(len(xreq.args)))
				go rn.processReq(xreq)
			case <-rn.done:
				return
			}
		}
	}()

	return rn
}

// * 清理网络，关闭相关 goroutine。
func (rn *Network) Cleanup() {
	close(rn.done)
}

// * 设置网络是否可靠。
func (rn *Network) Reliable(yes bool) {
	rn.mu.Lock()
	defer rn.mu.Unlock()

	rn.reliable = yes
}

// * 设置是否启用长时间乱序。
func (rn *Network) LongReordering(yes bool) {
	rn.mu.Lock()
	defer rn.mu.Unlock()

	rn.longReordering = yes
}

// * 设置是否启用长时间延迟。
func (rn *Network) LongDelays(yes bool) {
	rn.mu.Lock()
	defer rn.mu.Unlock()

	rn.longDelays = yes
}

// * 读取 ClientEnd 的信息。
func (rn *Network) readEndnameInfo(endname interface{}) (enabled bool,
	servername interface{}, server *Server, reliable bool, longreordering bool,
) {
	rn.mu.Lock()
	defer rn.mu.Unlock()

	enabled = rn.enabled[endname]
	servername = rn.connections[endname]
	if servername != nil {
		server = rn.servers[servername]
	}
	reliable = rn.reliable
	longreordering = rn.longReordering
	return
}

// * 检查服务器是否已死亡。
func (rn *Network) isServerDead(endname interface{}, servername interface{}, server *Server) bool {
	rn.mu.Lock()
	defer rn.mu.Unlock()

	if rn.enabled[endname] == false || rn.servers[servername] != server {
		return true
	}
	return false
}

// * 处理请求。
func (rn *Network) processReq(req reqMsg) {
	enabled, servername, server, reliable, longreordering := rn.readEndnameInfo(req.endname)

	if enabled && servername != nil && server != nil {
		if reliable == false {
			//* 短延迟
			ms := (rand.Int() % 27)
			time.Sleep(time.Duration(ms) * time.Millisecond)
		}

		if reliable == false && (rand.Int()%1000) < 100 {
			//* 丢弃请求，返回超时
			req.replyCh <- replyMsg{false, nil}
			return
		}

		//* 执行请求（调用 RPC 处理函数）。
		//* 在一个单独的线程中执行，以便我们可以定期检查
		//* 服务器是否已死亡，如果是，则 RPC 需要返回错误。
		ech := make(chan replyMsg)
		go func() {
			r := server.dispatch(req)
			ech <- r
		}()

		//* 等待处理函数返回，
		//* 但如果 DeleteServer() 被调用，停止等待并返回错误。
		var reply replyMsg
		replyOK := false
		serverDead := false
		for replyOK == false && serverDead == false {
			select {
			case reply = <-ech:
				replyOK = true
			case <-time.After(100 * time.Millisecond):
				serverDead = rn.isServerDead(req.endname, servername, server)
				if serverDead {
					go func() {
						<-ech //* 清空 channel 以让之前创建的 goroutine 终止
					}()
				}
			}
		}

		//* 如果服务器已经被删除，则不回复。
		serverDead = rn.isServerDead(req.endname, servername, server)

		if replyOK == false || serverDead == true {
			//* 服务器在等待期间已死亡，返回错误。
			req.replyCh <- replyMsg{false, nil}
		} else if reliable == false && (rand.Int()%1000) < 100 {
			//* 丢弃回复，返回超时
			req.replyCh <- replyMsg{false, nil}
		} else if longreordering == true && rand.Intn(900) < 600 {
			//* 延迟回复
			ms := 200 + rand.Intn(1+rand.Intn(2000))
			time.AfterFunc(time.Duration(ms)*time.Millisecond, func() {
				atomic.AddInt64(&rn.bytes, int64(len(reply.reply)))
				req.replyCh <- reply
			})
		} else {
			atomic.AddInt64(&rn.bytes, int64(len(reply.reply)))
			req.replyCh <- reply
		}
	} else {
		//* 模拟无回复并最终超时。
		ms := 0
		if rn.longDelays {
			//* 让 Raft 测试检查领导者是否没有同步发送
			//* RPC。
			ms = (rand.Int() % 7000)
		} else {
			//* 许多 kv 测试要求客户端快速尝试每个
			//* 服务器。
			ms = (rand.Int() % 100)
		}
		time.AfterFunc(time.Duration(ms)*time.Millisecond, func() {
			req.replyCh <- replyMsg{false, nil}
		})
	}

}

// * 创建一个客户端端点。
// * 启动一个线程来监听和处理。
func (rn *Network) MakeEnd(endname interface{}) *ClientEnd {
	rn.mu.Lock()
	defer rn.mu.Unlock()

	if _, ok := rn.ends[endname]; ok {
		log.Fatalf("MakeEnd: %v 已经存在\n", endname)
	}

	e := &ClientEnd{}
	e.endname = endname
	e.ch = rn.endCh
	e.done = rn.done
	rn.ends[endname] = e
	rn.enabled[endname] = false
	rn.connections[endname] = nil

	return e
}

// * 添加服务器到网络。
func (rn *Network) AddServer(servername interface{}, rs *Server) {
	rn.mu.Lock()
	defer rn.mu.Unlock()

	rn.servers[servername] = rs
}

// * 从网络中删除服务器。
func (rn *Network) DeleteServer(servername interface{}) {
	rn.mu.Lock()
	defer rn.mu.Unlock()

	rn.servers[servername] = nil
}
