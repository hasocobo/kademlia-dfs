package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math/big"
	"net"
	"sort"
	"strconv"
)

/*
We use idLength = 32 to match the 32 bytes produced by SHA-256 hashes.
SHA-256 is preferred over SHA-1 for new Kademlia implementations because SHA-1 is now considered cryptographically broken—
practical collision attacks have been demonstrated against SHA-1, meaning two different inputs can be crafted to produce the same SHA-1 hash.
This makes the identifier space vulnerable to attacks and undermines the security of the DHT.
SHA-256 is much stronger, with no known practical collision or preimage attacks, it is a safer choice for node identifiers.
*/
const (
	idLength = 32
)

type NodeId [idLength]byte

func NewNodeId(name string) NodeId {
	return sha256.Sum256([]byte(name))
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

// To determine what bucket the target node belongs to, we need to get the index of the distance (value we get after applying xorDistance),
//
// i.e, 010001 means it belongs to the 5th bucket(read from right to left, least significant bit is at the most right)
//
// So if we calculate it with log_2, we get the index.
//
// But since sha1 is 160-bit length, we can't use math.log2 because it accepts numbers with 64 bit,
//
// So we have to use big.Int.BitLen which does the same thing on larger numbers
func getBucketIndex(selfID, otherID NodeId) int {
	dist := xorDistance(selfID, otherID)

	return dist.BitLen() - 1
}

func (cs ContactSorter) Print() {
	for _, v := range cs.Contacts {
		fmt.Println("Contact: ", v, "Distance: ", xorDistance(v.ID, cs.TargetID))
		fmt.Println("Target belongs to bucket ", getBucketIndex(v.ID, cs.TargetID))
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
