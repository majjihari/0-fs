package p2p

import (
	"context"
	pstore "gx/ipfs/QmdeiKhUy1TVGBaKxt7y1QmBDLBdisSrLJ1x58Eoj4PXUh/go-libp2p-peerstore"
)

func (n *Node) HandlePeerFound(pi pstore.PeerInfo) {
	log.Infof("%s: peer discovered %s %v", n.host.ID().String(), pi.ID, pi.Addrs)
	if err := n.host.Connect(context.Background(), pi); err != nil {
		log.Errorf("%s error connecting to peer %s: %v", n.host.ID(), pi.ID, err)
	}
}
