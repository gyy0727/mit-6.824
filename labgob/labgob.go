package labgob

//
// 尝试通过RPC发送未大写的字段会产生一系列的错误行为，包括神秘的不正确计算和直接崩溃。
// 因此，这个包装器是为了 Go 的 encoding/gob，能够警告未大写的字段名。
//

import (
	"encoding/gob"
	"fmt"
	"io"
	"reflect"
	"sync"
	"unicode"
	"unicode/utf8"
)

var mu sync.Mutex                 //*互斥锁
var errorCount int                // 用于 TestCapital
var checked map[reflect.Type]bool //标记某个类型是否已经被处理

// * LabEncoder 是对 Go 的 gob.Encoder 的包装，并添加了对未大写字段名的检查。
type LabEncoder struct {
	gob *gob.Encoder
}

// * NewEncoder 创建一个新的 LabEncoder，包装了一个现有的 io.Writer。
func NewEncoder(w io.Writer) *LabEncoder {
	enc := &LabEncoder{}
	enc.gob = gob.NewEncoder(w)
	return enc
}

// * Encode 使用包装的 gob 编码器对给定的值进行编码，并在编码前检查未大写的字段。
func (enc *LabEncoder) Encode(e interface{}) error {
	checkValue(e)
	return enc.gob.Encode(e)
}

// * EncodeValue 使用包装的 gob 编码器对给定的 reflect.Value 进行编码，并在编码前检查未大写的字段。
func (enc *LabEncoder) EncodeValue(value reflect.Value) error {
	checkValue(value.Interface())
	return enc.gob.EncodeValue(value)
}

// * LabDecoder 是对 Go 的 gob.Decoder 的包装，并添加了对未大写字段名的检查。
type LabDecoder struct {
	gob *gob.Decoder
}

// * NewDecoder 创建一个新的 LabDecoder，包装了一个现有的 io.Reader。
func NewDecoder(r io.Reader) *LabDecoder {
	dec := &LabDecoder{}
	dec.gob = gob.NewDecoder(r)
	return dec
}

// * Decode 从读取器中解码数据到给定的值，并检查未大写字段。
func (dec *LabDecoder) Decode(e interface{}) error {
	checkValue(e)
	checkDefault(e)
	return dec.gob.Decode(e)
}

// * Register 将一个值注册到 Go 的 gob 包中，并检查未大写的字段。
func Register(value interface{}) {
	checkValue(value)
	gob.Register(value)
}

// * RegisterName 使用自定义名称将一个值注册到 Go 的 gob 包中，并检查未大写的字段。
func RegisterName(name string, value interface{}) {
	checkValue(value)
	gob.RegisterName(name, value)
}

// * checkValue 检查提供的值的类型，确保它不包含未大写的字段。
func checkValue(value interface{}) {
	checkType(reflect.TypeOf(value))
}

// * checkType 检查类型及其字段，确保它不包含未大写的字段名。
func checkType(t reflect.Type) {
	k := t.Kind() //*t的类型
	mu.Lock()
	// 只检测一次，并避免递归。
	if checked == nil {
		checked = map[reflect.Type]bool{}
	}
	//*类型已经被检查过
	if checked[t] {
		mu.Unlock()
		//*直接返回,因为函数目的就是检查类型有没有未大写的字段
		return
	}
	//*标记当前类型正在被检查
	checked[t] = true
	//*操作完共享数据了,直接解锁
	mu.Unlock()
	switch k {
	case reflect.Struct:
		//* 如果是结构体类型，检查所有字段的名字。
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			rune, _ := utf8.DecodeRuneInString(f.Name)
			if unicode.IsUpper(rune) == false {
				// 如果字段名是小写的，输出错误信息
				fmt.Printf("labgob error: lower-case field %v of %v in RPC or persist/snapshot will break your Raft\n",
					f.Name, t.Name())
				mu.Lock()
				errorCount += 1
				mu.Unlock()
			}
			// 递归检查字段的类型
			checkType(f.Type)
		}
		return
	case reflect.Slice, reflect.Array, reflect.Ptr:
		//* 如果是切片、数组或指针，递归检查元素类型。
		checkType(t.Elem())
		return
	case reflect.Map:
		//* 如果是映射，递归检查键和值的类型。
		checkType(t.Elem())
		checkType(t.Key())
		return
	default:
		//* 默认情况下，不需要做任何检查。
		return
	}
}

// * checkDefault 检查值是否包含非默认值，如果包含，可能会导致 RPC 或持久化/快照问题。
func checkDefault(value interface{}) {
	if value == nil {
		return
	}
	checkDefault1(reflect.ValueOf(value), 1, "")
}

// * checkDefault1 递归检查值及其字段，判断是否包含非默认值。
func checkDefault1(value reflect.Value, depth int, name string) {
	if depth > 3 {
		return
	}

	t := value.Type()
	k := t.Kind()

	switch k {
	case reflect.Struct:
		//* 如果是结构体，递归检查字段。
		for i := 0; i < t.NumField(); i++ {
			vv := value.Field(i)
			name1 := t.Field(i).Name
			if name != "" {
				name1 = name + "." + name1
			}
			checkDefault1(vv, depth+1, name1)
		}
		return
	case reflect.Ptr:
		//* 如果是指针，检查指针指向的值。
		if value.IsNil() {
			return
		}
		checkDefault1(value.Elem(), depth+1, name)
		return
	case reflect.Bool,
		reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Uintptr, reflect.Float32, reflect.Float64,
		reflect.String:
		//* 如果是基本类型，检查是否与默认值相等。
		if reflect.DeepEqual(reflect.Zero(t).Interface(), value.Interface()) == false {
			mu.Lock()
			if errorCount < 1 {
				what := name
				if what == "" {
					what = t.Name()
				}
				// 输出警告信息
				fmt.Printf("labgob warning: Decoding into a non-default variable/field %v may not work\n",
					what)
			}
			errorCount += 1
			mu.Unlock()
		}
		return
	}
}
