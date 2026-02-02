package scheduler

import kademliadfs "github.com/hasocobo/kademlia-dfs/kademlia"

type Event interface{ isEvent() }

type (
	EventTick               struct{}
	EventJobSubmitted       struct{ job JobDescription }
	EventTaskDispatchFailed struct {
		taskID  TaskID
		contact kademliadfs.Contact
		err     error
	}
	EventTaskDone struct{ taskID TaskID }
	EventJobDone  struct{ jobID JobID }
	//	EventTaskDispatched struct{ taskID TaskID } no need for now
)

func (EventTick) isEvent()               {}
func (EventJobSubmitted) isEvent()       {}
func (EventJobDone) isEvent()            {}
func (EventTaskDone) isEvent()           {}
func (EventTaskDispatchFailed) isEvent() {}

// func (EventTaskDispatched) isEvent() {}
