#!/bin/bash
# setup.sh — Bootstrap the dmsg project
# Run this after cloning the repo.
set -e

echo "📦 Resolving Go dependencies..."
go mod tidy

echo "🔨 Building..."
make build

echo ""
echo "✅ Build complete! Run with:"
echo "   ./bin/dmsg start"
echo ""
echo "   In a second terminal, connect to it:"
echo "   ./bin/dmsg start --listen /ip4/0.0.0.0/tcp/4002 --bootstrap /ip4/127.0.0.1/tcp/4001/p2p/<PEER_ID>"
echo ""
echo "   (Copy the <PEER_ID> from the first node's output)"
