package kademliadfs

type NodeState int

const (
	StateJoining NodeState = iota
	StateActive
)

func (ns NodeState) String() string {
	return [...]string{"JOINING", "ACTIVE"}[ns]
}

func (node *Node) SetState(target NodeState) error {
	node.mu.Lock()
	defer node.mu.Unlock()

	node.state = target

	return nil
}
