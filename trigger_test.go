package trigger

import (
	"fmt"
	"testing"
)

var (
	happy = func(arg string) { fmt.Println("happy event:" + arg) }
	sad   = func(arg string) { fmt.Println("sad event:" + arg) }
	once  = func(arg string) { fmt.Println("once event:" + arg) }
)

func TestTrigger(t *testing.T) {
	var arr []int = nil
	fmt.Println(len(arr))
	trigger := NewTrigger()
	t.Log("测试on")
	trigger.
		On("happy", happy).
		On("sad", sad).
		Emit("happy", "哈哈").Emit("sad", "嘤嘤嘤")

	// 测试Once
	t.Log("测试once")
	trigger.
		Once("once", once).
		Emit("once", "调用两次只执行一次").
		Emit("once", "调用两次只执行一次")

	t.Log("测试off")
	trigger.
		Emit("happy", "先触发happy").
		Off("happy", happy).
		Emit("happy", "调用off后再次触发happy 此时不会触发")

	t.Log("测试recover")
	trigger.
		Emit("sad", 1)
}
