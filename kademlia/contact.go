package kademliadfs

import "net"

type Contact struct {
	ID   NodeId
	IP   net.IP
	Port int
}

type ContactSorter struct {
	TargetID NodeId
	Contacts []Contact
}

// sort.Interface implementation
func (cs ContactSorter) Len() int { return len(cs.Contacts) }
func (cs ContactSorter) Swap(i, j int) {
	cs.Contacts[i], cs.Contacts[j] = cs.Contacts[j], cs.Contacts[i]
}

func (cs ContactSorter) Less(i, j int) bool {
	distI := xorDistance(cs.Contacts[i].ID, cs.TargetID)
	distJ := xorDistance(cs.Contacts[j].ID, cs.TargetID)
	return distI.Cmp(distJ) == -1
}
