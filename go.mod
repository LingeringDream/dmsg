module dmsg

go 1.22

// After cloning, run: go mod tidy
// This will resolve all dependencies automatically.

require (
	github.com/libp2p/go-libp2p v0.38.1
	github.com/libp2p/go-libp2p-pubsub v0.12.0
	github.com/libp2p/go-libp2p-kad-dht v0.28.2
	github.com/multiformats/go-multiaddr v0.14.0
	github.com/mattn/go-sqlite3 v1.14.24
	github.com/spf13/cobra v1.8.1
	google.golang.org/protobuf v1.36.3
)
