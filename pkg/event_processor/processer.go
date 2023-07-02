package event_processor

import (
	"fmt"
	"log"
	"stackplz/user/event"
	"sync"
	"time"
)

const (
	MAX_INCOMING_CHAN_LEN = 1024
	MAX_PARSER_QUEUE_LEN  = 1024
)

type EventProcessor struct {
	sync.Mutex
	// 收包，来自调用者发来的新事件
	incoming chan event.IEventStruct

	// key为 PID+UID+COMMON等确定唯一的信息
	workerQueue map[string]IWorker

	logger *log.Logger
}

func (this *EventProcessor) GetLogger() *log.Logger {
	return this.logger
}

func (this *EventProcessor) init() {
	this.incoming = make(chan event.IEventStruct, MAX_INCOMING_CHAN_LEN)
	this.workerQueue = make(map[string]IWorker, MAX_PARSER_QUEUE_LEN)
}

// Write event 处理器读取事件
func (this *EventProcessor) Serve() {
	for {
		select {
		case e := <-this.incoming:
			this.dispatch(e)
		}
	}
}

func (this *EventProcessor) dispatch(e event.IEventStruct) {
	//this.logger.Printf("event ID:%s", e.GetUUID())
	err := e.ParseContext()
	if err != nil {
		this.logger.Printf("ParseContext failed:%d err:%v", e.EventType(), err)
		return
	}
	// this.logger.Printf("event_context:%s", e.GetEventContext().String())
	// 在做完初步解析之后 这里应该根据 eventid 转换为更明确的 event
	e = e.ToChildEvent()
	if e.GetEventContext().EventId == 0 {
		this.logger.Printf("ParseContext skip EventId:%d", e.GetEventContext().EventId)
		return
	}
	var uuid string = e.GetUUID()
	found, eWorker := this.getWorkerByUUID(uuid)
	if !found {
		// ADD a new eventWorker into queue
		eWorker = NewEventWorker(e.GetUUID(), this)
		this.addWorkerByUUID(eWorker)
	}

	err = eWorker.Write(e)
	if err != nil {
		//...
		this.GetLogger().Fatalf("write event failed , error:%v", err)
	}
}

//func (this *EventProcessor) Incoming() chan user.IEventStruct {
//	return this.incoming
//}

func (this *EventProcessor) getWorkerByUUID(uuid string) (bool, IWorker) {
	this.Lock()
	defer this.Unlock()
	var eWorker IWorker
	var found bool
	eWorker, found = this.workerQueue[uuid]
	if !found {
		return false, eWorker
	}
	return true, eWorker
}

func (this *EventProcessor) addWorkerByUUID(worker IWorker) {
	this.Lock()
	defer this.Unlock()
	this.workerQueue[worker.GetUUID()] = worker
}

// 每个worker调用该方法，从处理器中删除自己
func (this *EventProcessor) delWorkerByUUID(worker IWorker) {
	this.Lock()
	defer this.Unlock()
	delete(this.workerQueue, worker.GetUUID())
}

// Write event
// 外部调用者调用该方法
func (this *EventProcessor) Write(e event.IEventStruct) {
	select {
	case this.incoming <- e:
		return
	}
}

func (this *EventProcessor) Close() error {
	// 关闭模块的时候 变更 worker 状态 让它自己退出
	for _, worker := range this.workerQueue {
		worker.(*eventWorker).NeedExit = true
	}
	// 等待 0.3s
	time.Sleep(3 * 100 * time.Millisecond)
	if len(this.workerQueue) > 0 {
		return fmt.Errorf("EventProcessor.Close(): workerQueue is not empty:%d", len(this.workerQueue))
	}
	return nil
}

func NewEventProcessor(logger *log.Logger) *EventProcessor {
	var ep *EventProcessor
	ep = &EventProcessor{}
	ep.logger = logger
	ep.init()
	return ep
}
