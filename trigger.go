package trigger

import (
	"errors"
	"fmt"
	"os"
	"reflect"
	"sync"
)

// 事件默认最大监听数量
const defaultMaxListeners = 16

// 错误
var ErrNotFunction = errors.New("传入参数不是函数类型")
var ErrExceedMaxListeners = errors.New("此事件超过最大监听数量")

// 错误处理函数
type RecoveryFunc func(interface{}, interface{}, error)

// 默认错误处理函数
var defaultRecoveryFunc RecoveryFunc = func(event interface{}, listener interface{}, err error) {
	fmt.Fprintf(os.Stdout, "Error: 事件[%v]\n错误: %v.\n", event, err)
}

// 事件触发器
type Trigger struct {
	// 读写锁
	*sync.RWMutex
	// 存放事件与事件执行函数的反射数组
	events map[interface{}][]reflect.Value
	// 最大监听数量
	maxListeners int
	// 错误处理函数
	recoverer RecoveryFunc
}

//***************************************************
//Description : 添加事件
//param :       事件名称
//param :       回调函数
//return :      事件触发器
//***************************************************
func (trigger *Trigger) AddListener(event, listener interface{}) *Trigger {
	// 加锁
	trigger.Lock()
	defer trigger.Unlock()

	// 反射回调函数
	fn := reflect.ValueOf(listener)

	// 判断参数2是否是函数类型
	if reflect.Func != fn.Kind() {
		// 如果未对recoverer赋值, 则直接panic, 否则调用recoverer
		if nil == trigger.recoverer {
			panic(ErrNotFunction)
		} else {
			trigger.recoverer(event, listener, ErrNotFunction)
		}
	}

	// 判断此事件是否超过最大监听数量, 如果超过panic或者调用recoverer
	if trigger.maxListeners != -1 && trigger.maxListeners < len(trigger.events[event])+1 {
		if nil == trigger.recoverer {
			panic(ErrExceedMaxListeners)
		} else {
			trigger.recoverer(event, listener, ErrExceedMaxListeners)
		}
	}

	// 对此事件追加监听者
	trigger.events[event] = append(trigger.events[event], fn)

	// 返回本对象, 链式编程
	return trigger
}

//***************************************************
//Description : 调用的AddListener
//param :       事件名称
//param :       回调函数
//return :      事件触发器
//***************************************************
func (trigger *Trigger) On(event, listener interface{}) *Trigger {
	return trigger.AddListener(event, listener)
}

//***************************************************
//Description : 删除监听
//param :       事件类型
//param :       监听回调函数
//return :      事件触发器
//***************************************************
func (trigger *Trigger) RemoveListener(event, listener interface{}) *Trigger {
	trigger.Lock()
	defer trigger.Unlock()

	// 获取回调函数类型
	fn := reflect.ValueOf(listener)
	if reflect.Func != fn.Kind() {
		if nil == trigger.recoverer {
			panic(ErrNotFunction)
		} else {
			trigger.recoverer(event, listener, ErrNotFunction)
		}
	}

	// 从事件map中获取回调函数数组
	if events, ok := trigger.events[event]; ok {
		newEvents := []reflect.Value{}
		// 遍历数组,把其他回调函数放入新的数组中
		for _, listener := range events {
			if fn.Pointer() != listener.Pointer() {
				newEvents = append(newEvents, listener)
			}
		}
		// 从新赋值
		trigger.events[event] = newEvents
	}

	return trigger
}

//***************************************************
//Description : 调用的RemoveListener
//param :       事件名称
//param :       回调函数
//return :      事件触发器
//***************************************************
func (trigger *Trigger) Off(event, listener interface{}) *Trigger {
	return trigger.RemoveListener(event, listener)
}

//***************************************************
//Description : 添加只执行一次的监听事件
//param :       事件名称
//param :       回调函数
//return :      事件触发器
//***************************************************
func (trigger *Trigger) Once(event, listener interface{}) *Trigger {
	// 获取回调函数类型
	fn := reflect.ValueOf(listener)
	if reflect.Func != fn.Kind() {
		if nil == trigger.recoverer {
			panic(ErrNotFunction)
		} else {
			trigger.recoverer(event, listener, ErrNotFunction)
		}
	}

	// 包装回调函数, 在调用回调函数之后调用RemoveListener移除此监听
	var run func(...interface{})
	run = func(arguments ...interface{}) {
		defer trigger.RemoveListener(event, run)

		var values []reflect.Value

		for i := 0; i < len(arguments); i++ {
			values = append(values, reflect.ValueOf(arguments[i]))
		}

		fn.Call(values)
	}

	// 添加监听, 函数为包装后的函数
	trigger.AddListener(event, run)
	return trigger
}

//***************************************************
//Description : 触发事件
//param :       事件类型
//param :       回调函数中的参数, 按照回调函数的参数列表顺序传入
//return :      事件触发器
//***************************************************
func (trigger *Trigger) Emit(event interface{}, arguments ...interface{}) *Trigger {
	// 获取此事件的监听回调函数数组,如果为空则直接返回
	listeners := trigger.GetListenersByEvent(event)
	if nil == listeners {
		return trigger
	}

	var wg sync.WaitGroup
	wg.Add(len(listeners))

	// 遍历监听函调函数
	for _, fn := range listeners {
		// 开启协程同步执行此事件的所有监听, 同时 WaitGroup - 1
		go func(fn reflect.Value) {
			defer wg.Done()

			// 拦截监听回调函数中的panic
			if nil != trigger.recoverer {
				defer func() {
					if r := recover(); nil != r {
						err := fmt.Errorf("%v", r)
						trigger.recoverer(event, fn.Interface(), err)
					}
				}()
			}

			// 传入参数数组
			var values []reflect.Value
			for i := 0; i < len(arguments); i++ {
				if arguments[i] == nil {
					values = append(values, reflect.New(fn.Type().In(i)).Elem())
				} else {
					values = append(values, reflect.ValueOf(arguments[i]))
				}
			}

			// 调用
			fn.Call(values)
		}(fn)
	}
	// 等待所有回调执行完毕
	wg.Wait()
	return trigger
}

//***************************************************
//Description : 同Emit, 不过会同步执行所有回调函数时
//param :       事件名称
//param :       回调函数中的参数, 按照回调函数的参数列表顺序传入
//return :      事件触发器
//***************************************************
func (trigger *Trigger) EmitSync(event interface{}, arguments ...interface{}) *Trigger {
	// 获取此事件的监听回调函数数组,如果为空则直接返回
	listeners := trigger.GetListenersByEvent(event)
	if nil == listeners {
		return trigger
	}

	for _, fn := range listeners {
		if nil != trigger.recoverer {
			defer func() {
				if r := recover(); nil != r {
					err := fmt.Errorf("%v", r)
					trigger.recoverer(event, fn.Interface(), err)
				}
			}()
		}

		var values []reflect.Value

		for i := 0; i < len(arguments); i++ {
			if arguments[i] == nil {
				values = append(values, reflect.New(fn.Type().In(i)).Elem())
			} else {
				values = append(values, reflect.ValueOf(arguments[i]))
			}
		}

		fn.Call(values)
	}

	return trigger
}

//***************************************************
//Description : 根据时间类型获取监听回调函数数组
//param :       时间类型
//return :      监听回调函数数组 或者 nil
//***************************************************
func (trigger *Trigger) GetListenersByEvent(event interface{}) []reflect.Value {
	trigger.RLock()
	defer trigger.RUnlock()
	listeners, _ := trigger.events[event]
	return listeners
}

//***************************************************
//Description : 设置错误处理函数
//param :       处理函数
//return :      事件触发器
//***************************************************
func (trigger *Trigger) RecoverWith(fn RecoveryFunc) *Trigger {
	trigger.Lock()
	defer trigger.Unlock()

	trigger.recoverer = fn
	return trigger
}

//***************************************************
//Description : 设置单事件最大监听数量
//param :       最大值
//return :      事件触发器
//***************************************************
func (trigger *Trigger) SetMaxListeners(max int) *Trigger {
	trigger.Lock()
	defer trigger.Unlock()

	trigger.maxListeners = max
	return trigger
}

//***************************************************
//Description : 获取某事件监听数量
//param :       事件类型
//return :      数量
//***************************************************
func (trigger *Trigger) GetListenerCount(event interface{}) int {
	trigger.RLock()
	defer trigger.RUnlock()
	listeners, _ := trigger.events[event]
	return len(listeners)
}

//***************************************************
//Description : 触发器构造函数
//return :      事件触发器
//***************************************************
func NewTrigger() (trigger *Trigger) {
	trigger = new(Trigger)
	trigger.RWMutex = new(sync.RWMutex)
	trigger.events = make(map[interface{}][]reflect.Value)
	trigger.maxListeners = defaultMaxListeners
	trigger.recoverer = defaultRecoveryFunc
	return
}
