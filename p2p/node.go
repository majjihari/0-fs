package p2p

import (
	"context"
	"crypto/rand"
	"fmt"
	"io"
	"io/ioutil"
	mrand "math/rand"
	"os"
	"path"
	"time"

	ma "gx/ipfs/QmWWQ2Txc2c6tqjsBpzg5Ar652cHPGNsQQp2SejkNmkUMb/go-multiaddr"
	crypto "gx/ipfs/Qme1knMqwt1hKZbc1BmQFmnm9f36nyQGwXxPGVpVJ9rMK5/go-libp2p-crypto"
	host "gx/ipfs/QmfZTdmunzKzAGJrSvXXQbQ5kLLUiEMX5vdwux7iXkdk7D/go-libp2p-host"

	libp2p "github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/p2p/discovery"
)

// Node type - a p2p host implementing one or more p2p protocols
type Node struct {
	host      host.Host // lib-p2p host
	cache     string    //root path of the cache directory
	discovery discovery.Service
	// Map of file name requests and channels on which the data is supposed to come back
	// when requesting a file, we can pool on the channel until we get a response
	requestsChan map[string]chan []byte //TODO: protect concurrent access to the map
	*ShareStorageProtocol
}

// NewNode creates a new node with its implemented protocols
func NewNode(host host.Host, cache string) *Node {
	node := &Node{
		host:         host,
		requestsChan: make(map[string]chan []byte),
		cache:        cache,
	}
	node.ShareStorageProtocol = NewShareStorageProtocol(node)
	discovery, err := discovery.NewMdnsService(context.Background(), host, time.Second*60, "")
	if err != nil {
		panic("error creating discovery service")
	}
	node.discovery = discovery
	node.discovery.RegisterNotifee(node)
	return node
}

func (n *Node) Close() error {
	if err := n.discovery.Close(); err != nil {
		return err
	}
	return n.host.Close()
}

func (n *Node) CheckAndGet(name string) <-chan []byte {
	out := make(chan []byte)

	if _, exists := n.requestsChan[name]; !exists {
		n.requestsChan[name] = out
	}
	n.ShareStorageProtocol.RequestFile(name)
	return out
}

func (n *Node) getFromCache(name string) (bool, []byte) {
	log.Infof("%s: getFromCache, path: %v", n.host.ID().String(), name)

	name = path.Join(n.cache, name)

	f, err := os.Open(name)
	if err != nil {
		log.Errorf("%s: error trying to read cached file: %v", n.host.ID().String(), err)
		return false, nil
	}

	// TODO: return a io.Reader to stream the data instead of a block of bytes
	b, err := ioutil.ReadAll(f)
	if err != nil {
		log.Errorf("%s: error trying to read cached file: %v", n.host.ID().String(), err)
		return false, nil
	}

	return true, b
}

// MakeBasicHost creates a LibP2P host with a random peer ID listening on the
// given multiaddress
func MakeBasicHost(listenPort int, randseed int64) (host.Host, error) {

	// If the seed is zero, use real cryptographic randomness. Otherwise, use a
	// deterministic randomness source to make generated keys stay the same
	// across multiple runs
	var r io.Reader
	if randseed == 0 {
		r = rand.Reader
	} else {
		r = mrand.New(mrand.NewSource(randseed))
	}

	// Generate a key pair for this host. We will use it at least
	// to obtain a valid host ID.
	priv, _, err := crypto.GenerateKeyPairWithReader(crypto.RSA, 2048, r)
	if err != nil {
		return nil, err
	}

	opts := []libp2p.Option{
		libp2p.ListenAddrStrings(fmt.Sprintf("/ip4/127.0.0.1/tcp/%d", listenPort)),
		libp2p.Identity(priv),
	}

	basicHost, err := libp2p.New(context.Background(), opts...)
	if err != nil {
		return nil, err
	}

	// Build host multiaddress
	hostAddr, _ := ma.NewMultiaddr(fmt.Sprintf("/ipfs/%s", basicHost.ID().Pretty()))

	// Now we can build a full multiaddress to reach this host
	// by encapsulating both addresses:
	addr := basicHost.Addrs()[0]
	fullAddr := addr.Encapsulate(hostAddr)
	log.Infof("I am %s %s\n", fullAddr, basicHost.ID().String())

	return basicHost, nil
}
