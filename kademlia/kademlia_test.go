package kademliadfs

import (
	"net"
	"testing"
)

func BenchmarkXorDistance(b *testing.B) {
	targetID := NewNodeId("target-id")
	inputID := NewNodeId("input-id")

	for b.Loop() {
		XorDistance(inputID, targetID)
	}
}

// Helper for generating a contact in the specified bucket for testing
func CreateContactForBucket(selfID NodeId, bucketIndex int, contactIndex int) Contact {
	byteIndex := idLength - 1 - bucketIndex/8
	bitPosition := bucketIndex % 8

	createdContactID := selfID

	createdContactID[byteIndex] ^= 1 << bitPosition
	createdContactID[idLength-1] += byte(contactIndex)

	return Contact{ID: createdContactID, IP: net.IPv4zero, Port: 10000 + contactIndex}
}
