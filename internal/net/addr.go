package net

import (
	"github.com/multiformats/go-multiaddr"
)

func parseMultiaddr(s string) (multiaddr.Multiaddr, error) {
	return multiaddr.NewMultiaddr(s)
}
