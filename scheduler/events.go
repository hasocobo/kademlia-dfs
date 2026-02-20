package scheduler

import kademliadfs "github.com/hasocobo/kademlia-dfs/kademlia"

type Event interface{ isEvent() }

type (
	EventTick         struct{}
	EventJobSubmitted struct {
		job   JobDescription
		tasks []*TaskDescription
	}
	EventTaskDispatchFailed struct {
		taskID  TaskID
		contact kademliadfs.Contact
		err     error
	}
	EventTaskDone struct {
		taskID TaskID
		result []byte
	}
	EventJobDone      struct{ jobID JobID }
	EventLeaseRequest struct{ request leaseRequest }
	//	EventTaskDispatched struct{ taskID TaskID } no need for now
)

func (EventTick) isEvent()               {}
func (EventJobSubmitted) isEvent()       {}
func (EventJobDone) isEvent()            {}
func (EventTaskDone) isEvent()           {}
func (EventTaskDispatchFailed) isEvent() {}
func (EventLeaseRequest) isEvent()       {}

// func (EventTaskDispatched) isEvent() {}
