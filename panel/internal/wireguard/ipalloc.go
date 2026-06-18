package wireguard

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math/big"
	"net"
)

var (
	ErrInvalidCIDR    = errors.New("invalid CIDR notation")
	ErrPoolExhausted  = errors.New("ip_pool_exhausted")
	ErrSubnetTooSmall = errors.New("subnet too small for allocation")
)

// ParseSubnetRange returns the first usable IP, last usable IP, prefix bits,
// and any error for the given CIDR. For IPv4, network and broadcast addresses
// are excluded. The gateway address (.1 for IPv4, ::1 for IPv6) is also excluded
// from the usable range — first returned is the first allocatable address (.2 / ::2).
func ParseSubnetRange(networkCIDR string) (first, last net.IP, bits int, err error) {
	_, ipNet, err := net.ParseCIDR(networkCIDR)
	if err != nil {
		return nil, nil, 0, ErrInvalidCIDR
	}

	ones, totalBits := ipNet.Mask.Size()
	bits = ones

	if isIPv4(ipNet.IP) {
		return parseIPv4Range(ipNet, ones, totalBits)
	}
	return parseIPv6Range(ipNet, ones, totalBits)
}

// AllocateNextIP finds the next available IP in the given CIDR, excluding
// network address, broadcast (for IPv4), gateway (.1 / ::1), and any IPs
// in the usedIPs slice. Returns the allocated IP as a string or an error.
func AllocateNextIP(networkCIDR string, usedIPs []string) (string, error) {
	first, last, _, err := ParseSubnetRange(networkCIDR)
	if err != nil {
		return "", err
	}

	usedSet := make(map[string]struct{}, len(usedIPs))
	for _, ip := range usedIPs {
		// Normalize: parse and convert back to string to handle formatting differences
		parsed := net.ParseIP(ip)
		if parsed != nil {
			usedSet[parsed.String()] = struct{}{}
		}
	}

	if isIPv4(first) {
		return allocateIPv4(first, last, usedSet)
	}
	return allocateIPv6(first, last, usedSet)
}

func parseIPv4Range(ipNet *net.IPNet, ones, totalBits int) (net.IP, net.IP, int, error) {
	hostBits := totalBits - ones
	// Need at least /30 to have usable addresses (network, gateway, 1 host, broadcast)
	if hostBits < 2 {
		return nil, nil, 0, ErrSubnetTooSmall
	}

	networkIP := ipNet.IP.To4()
	networkInt := binary.BigEndian.Uint32(networkIP)

	// Broadcast = network | ^mask
	maskInt := binary.BigEndian.Uint32(net.IP(ipNet.Mask).To4())
	broadcastInt := networkInt | ^maskInt

	// First allocatable = network + 2 (skip network address and gateway .1)
	firstInt := networkInt + 2
	// Last allocatable = broadcast - 1
	lastInt := broadcastInt - 1

	if firstInt > lastInt {
		return nil, nil, 0, ErrSubnetTooSmall
	}

	firstIP := make(net.IP, 4)
	binary.BigEndian.PutUint32(firstIP, firstInt)
	lastIP := make(net.IP, 4)
	binary.BigEndian.PutUint32(lastIP, lastInt)

	return firstIP, lastIP, ones, nil
}

func parseIPv6Range(ipNet *net.IPNet, ones, totalBits int) (net.IP, net.IP, int, error) {
	hostBits := totalBits - ones
	if hostBits < 2 {
		return nil, nil, 0, ErrSubnetTooSmall
	}

	networkIP := ipNet.IP.To16()

	// First allocatable = network + 2 (skip network address and ::1 gateway)
	firstBig := ipToBigInt(networkIP)
	firstBig.Add(firstBig, big.NewInt(2))

	// Last allocatable: network | ^mask - but for IPv6 we don't subtract broadcast
	// IPv6 doesn't have broadcast, but last address is often reserved by convention
	// We'll use all addresses up to the last in the range
	maskBig := maskToBigInt(ipNet.Mask)
	notMask := new(big.Int).Not(maskBig)
	// Limit to 128 bits
	allOnes := new(big.Int).Lsh(big.NewInt(1), 128)
	allOnes.Sub(allOnes, big.NewInt(1))
	notMask.And(notMask, allOnes)

	lastBig := new(big.Int).Or(ipToBigInt(networkIP), notMask)

	firstIP := bigIntToIP(firstBig)
	lastIP := bigIntToIP(lastBig)

	return firstIP, lastIP, ones, nil
}

func allocateIPv4(first, last net.IP, usedSet map[string]struct{}) (string, error) {
	firstInt := binary.BigEndian.Uint32(first.To4())
	lastInt := binary.BigEndian.Uint32(last.To4())

	for i := firstInt; i <= lastInt; i++ {
		candidate := make(net.IP, 4)
		binary.BigEndian.PutUint32(candidate, i)
		if _, used := usedSet[candidate.String()]; !used {
			return candidate.String(), nil
		}
	}
	return "", ErrPoolExhausted
}

func allocateIPv6(first, last net.IP, usedSet map[string]struct{}) (string, error) {
	current := ipToBigInt(first.To16())
	lastBig := ipToBigInt(last.To16())
	one := big.NewInt(1)

	for current.Cmp(lastBig) <= 0 {
		candidate := bigIntToIP(current)
		if _, used := usedSet[candidate.String()]; !used {
			return candidate.String(), nil
		}
		current.Add(current, one)
	}
	return "", ErrPoolExhausted
}

func isIPv4(ip net.IP) bool {
	return ip.To4() != nil
}

func ipToBigInt(ip net.IP) *big.Int {
	ip = ip.To16()
	return new(big.Int).SetBytes(ip)
}

func maskToBigInt(mask net.IPMask) *big.Int {
	return new(big.Int).SetBytes(mask)
}

func bigIntToIP(n *big.Int) net.IP {
	b := n.Bytes()
	// Pad to 16 bytes for IPv6
	ip := make(net.IP, 16)
	copy(ip[16-len(b):], b)
	return ip
}

// FormatWithPrefix returns the IP with the CIDR prefix length appended (e.g., "10.66.66.2/24").
func FormatWithPrefix(ip string, networkCIDR string) (string, error) {
	_, ipNet, err := net.ParseCIDR(networkCIDR)
	if err != nil {
		return "", fmt.Errorf("invalid CIDR: %w", err)
	}
	ones, _ := ipNet.Mask.Size()
	return fmt.Sprintf("%s/%d", ip, ones), nil
}
