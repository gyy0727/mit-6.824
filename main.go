package main

import (
	"bytes"
	"fmt"
	"github.com/gyy0727/mit-6.824/labgob"  
)

// 模拟 TestGOB 测试
func TestGOBManual() {
	type T1 struct {
		T1int0    int
		T1int1    int
		T1string0 string
		T1string1 string
	}

	type T2 struct {
		T2slice []T1
		T2map   map[int]*T1
		T2t3    interface{}
	}

	type T3 struct {
		T3int999 int
	}

	var errorCount int // 用于模拟 errorCount 变量
	e0 := errorCount

	w := new(bytes.Buffer)

	// 注册结构体
	labgob.Register(T3{})

	// 初始化测试数据
	x0 := 0
	x1 := 1
	t1 := T1{T1int1: 1, T1string1: "6.824"}
	t2 := T2{
		T2slice: []T1{{}, t1},
		T2map:   map[int]*T1{99: {1, 2, "x", "y"}},
		T2t3:    T3{999},
	}

	// 编码
	e := labgob.NewEncoder(w)
	e.Encode(x0)
	e.Encode(x1)
	e.Encode(t1)
	e.Encode(t2)

	data := w.Bytes()

	// 解码
	var d_x0, d_x1 int
	var d_t1 T1
	var d_t2 T2
	r := bytes.NewBuffer(data)
	d := labgob.NewDecoder(r)
	d.Decode(&d_x0)
	d.Decode(&d_x1)
	d.Decode(&d_t1)
	d.Decode(&d_t2)

	// 验证
	if d_x0 != 0 || d_x1 != 1 {
		fmt.Println("Failed: x0 or x1 mismatch")
		return
	}
	if d_t1.T1int1 != 1 || d_t1.T1string1 != "6.824" {
		fmt.Println("Failed: t1 values mismatch")
		return
	}
	if len(d_t2.T2slice) != 2 || d_t2.T2slice[1].T1int1 != 1 {
		fmt.Println("Failed: t2.T2slice mismatch")
		return
	}
	if len(d_t2.T2map) != 1 || d_t2.T2map[99].T1string1 != "y" {
		fmt.Println("Failed: t2.T2map mismatch")
		return
	}
	t3 := d_t2.T2t3.(T3)
	if t3.T3int999 != 999 {
		fmt.Println("Failed: t2.T2t3 mismatch")
		return
	}

	if errorCount != e0 {
		fmt.Println("Errors occurred during encoding/decoding")
		return
	}

	fmt.Println("TestGOB passed successfully!")
}

func main() {
	fmt.Println("Running TestGOB...")
	TestGOBManual()
}
