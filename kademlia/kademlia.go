package kademliadfs

import "math/bits"

/*
We use idLength = 32 to match the 32 bytes produced by SHA-256 hashes.
SHA-256 is preferred over SHA-1 for new Kademlia implementations because SHA-1 is now considered cryptographically broken because
practical collision attacks have been demonstrated against SHA-1, meaning two different inputs can be crafted to produce the same SHA-1 hash.
This makes the identifier space vulnerable to attacks and undermines the security of the DHT.
SHA-256 is much stronger, with no known practical collision or preimage attacks, it is a safer choice for node identifiers.
*/
const (
	idLength              = 32
	k                     = 32
	maxConcurrentRequests = 3 // Standard concurrency parameter (alpha) for Kademlia node lookup
)

// XorDistance ,starting from the most significant bit, returns the first encountered 1's bit index position of the xor result of a and b
func XorDistance(a, b NodeId) int {
	commonZeroBitCount := 0
	for i := range idLength {
		result := a[i] ^ b[i]

		if result == 0 {
			commonZeroBitCount += 8 // all of the byte is filled with 0 bits
		} else {
			commonZeroBitCount += bits.LeadingZeros8(result)
			// if they don't match at all, return bucketSize - 1, e.g. 255
			if commonZeroBitCount == 0 {
				return idLength*8 - 1
			}
			return idLength*8 - commonZeroBitCount - 1
		}
	}
	// if they are exactly the same
	return idLength*8 - commonZeroBitCount
}
