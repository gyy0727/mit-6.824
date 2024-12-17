package labrpc
import"github.com/gyy0727/labrpc"
import "testing"
import "strconv"
import "sync"
import "runtime"
import "time"
import "fmt"

//*参数
type JunkArgs struct {
	X int
}
//*响应体
type JunkReply struct {
	X string
}

//*服务器
type JunkServer struct {
	mu   sync.Mutex
	log1 []string
	log2 []int
}

//*处理函数1
func (js *JunkServer) Handler1(args string, reply *int) {
	js.mu.Lock()
	defer js.mu.Unlock()
	js.log1 = append(js.log1, args)
	*reply, _ = strconv.Atoi(args)
}

//*处理函数2
func (js *JunkServer) Handler2(args int, reply *string) {
	js.mu.Lock()
	defer js.mu.Unlock()
	js.log2 = append(js.log2, args)
	*reply = "handler2-" + strconv.Itoa(args)
}
//*处理函数3
func (js *JunkServer) Handler3(args int, reply *int) {
	js.mu.Lock()
	defer js.mu.Unlock()
	time.Sleep(20 * time.Second)
	*reply = -args
}

//*处理函数4
func (js *JunkServer) Handler4(args *JunkArgs, reply *JunkReply) {
	reply.X = "pointer"
}

//*处理函数5
func (js *JunkServer) Handler5(args JunkArgs, reply *JunkReply) {
	reply.X = "no pointer"
}

//*处理函数6
func (js *JunkServer) Handler6(args string, reply *int) {
	js.mu.Lock()
	defer js.mu.Unlock()
	*reply = len(args)
}

//*处理函数7
func (js *JunkServer) Handler7(args int, reply *string) {
	js.mu.Lock()
	defer js.mu.Unlock()
	*reply = ""
	for i := 0; i < args; i++ {
		*reply = *reply + "y"
	}
}


func main(){
  runtime.GOMAXPROCS(4)

	rn := MakeNetwork()
	defer rn.Cleanup()

	e := rn.MakeEnd("end1-99")

	js := &JunkServer{}
	svc := MakeService(js)

	rs := MakeServer()
	rs.AddService(svc)
	rn.AddServer("server99", rs)

	rn.Connect("end1-99", "server99")
	rn.Enable("end1-99", true)

	{
		reply := ""
		e.Call("JunkServer.Handler2", 111, &reply)
		if reply != "handler2-111" {
			t.Fatalf("wrong reply from Handler2")
		}
	}

	{
		reply := 0
		e.Call("JunkServer.Handler1", "9099", &reply)
		if reply != 9099 {
			t.Fatalf("wrong reply from Handler1")
		}
	}
}
