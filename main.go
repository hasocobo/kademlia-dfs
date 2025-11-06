package main

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"math/big"
	"net"
	"sort"
	"strconv"
)

type NodeId [20]byte

func NewNodeId(name string) NodeId {
	return sha1.Sum([]byte(name)) // Returns 20 byte hash
}

func (nodeId NodeId) String() string {
	return hex.EncodeToString(nodeId[:])
}

func xorDistance(a, b NodeId) *big.Int {
	ai := new(big.Int).SetBytes(a[:])
	bi := new(big.Int).SetBytes(b[:])
	return new(big.Int).Xor(ai, bi)
}

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

func (cs ContactSorter) Print() {
	for _, v := range cs.Contacts {
		fmt.Println("Contact: ", v, "Distance: ", xorDistance(v.ID, cs.TargetID))
	}
}

func main() {
	var contacts []Contact

	for i := range 10 {
		contacts = append(contacts,
			Contact{ID: NewNodeId("test " + strconv.Itoa(i)), IP: net.IPv4zero, Port: 10000 + i})
	}

	contactTable := &ContactSorter{
		TargetID: NewNodeId("test"),
		Contacts: contacts,
	}

	contactTable.Print()

	fmt.Println()

	sort.Sort(contactTable)

	contactTable.Print()
}
