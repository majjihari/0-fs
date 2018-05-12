package p2p

import (
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"testing"
	"time"

	ps "gx/ipfs/QmdeiKhUy1TVGBaKxt7y1QmBDLBdisSrLJ1x58Eoj4PXUh/go-libp2p-peerstore"

	"github.com/stretchr/testify/require"
)

func makeNode(listenPort int) (*Node, error) {
	h, err := MakeBasicHost(listenPort, 0)
	if err != nil {
		return nil, err
	}

	cache, err := ioutil.TempDir("", "")
	if err != nil {
		return nil, err
	}

	return NewNode(h, cache), nil
}

func TestShareFile(t *testing.T) {
	require := require.New(t)

	node1, err := makeNode(9900)
	require.NoError(err)
	node2, err := makeNode(9901)
	require.NoError(err)

	defer func() {
		if err := node1.Close(); err != nil {
			fmt.Println(err)
		}
		if err := node2.Close(); err != nil {
			fmt.Println(err)
		}
		os.RemoveAll(node1.cache)
		os.RemoveAll(node2.cache)
	}()

	node2.host.Peerstore().AddAddrs(node1.host.ID(), node1.host.Addrs(), ps.PermanentAddrTTL)
	node1.host.Peerstore().AddAddrs(node2.host.ID(), node2.host.Addrs(), ps.PermanentAddrTTL)

	const name = "foo"
	data1 := []byte("hello world")

	err = ioutil.WriteFile(path.Join(node1.cache, name), data1, 0660)
	require.NoError(err)

	c := node2.CheckAndGet(name)
	data2 := <-c
	require.Equal(data1, data2)
}

func TestShareFileMultipleNodes(t *testing.T) {
	require := require.New(t)

	node1, err := makeNode(9900)
	require.NoError(err)
	node2, err := makeNode(9901)
	require.NoError(err)
	node3, err := makeNode(9903)
	require.NoError(err)

	defer func() {
		node1.Close()
		node2.Close()
		node3.Close()
		os.RemoveAll(node1.cache)
		os.RemoveAll(node2.cache)
		os.RemoveAll(node3.cache)
	}()

	node1.host.Peerstore().AddAddrs(node2.host.ID(), node2.host.Addrs(), ps.PermanentAddrTTL)
	node1.host.Peerstore().AddAddrs(node3.host.ID(), node3.host.Addrs(), ps.PermanentAddrTTL)

	node2.host.Peerstore().AddAddrs(node1.host.ID(), node1.host.Addrs(), ps.PermanentAddrTTL)
	node2.host.Peerstore().AddAddrs(node3.host.ID(), node3.host.Addrs(), ps.PermanentAddrTTL)

	node3.host.Peerstore().AddAddrs(node1.host.ID(), node1.host.Addrs(), ps.PermanentAddrTTL)
	node3.host.Peerstore().AddAddrs(node2.host.ID(), node2.host.Addrs(), ps.PermanentAddrTTL)

	time.Sleep(1)

	const name = "foo"
	data1 := []byte("hello world")

	err = ioutil.WriteFile(path.Join(node1.cache, name), data1, 0660)
	require.NoError(err)

	t.Run("only 1 node has the file", func(t *testing.T) {
		c := node2.CheckAndGet(name)
		select {
		case data2 := <-c:
			require.Equal(data1, data2)
			err = ioutil.WriteFile(path.Join(node2.cache, name), data2, 0660)
			require.NoError(err)
		case <-time.After(time.Second * 5):
			t.Error("file not found")
		}
	})

	t.Run("2 nodes has the file now", func(t *testing.T) {
		c := node3.CheckAndGet(name)
		select {
		case data3 := <-c:
			require.Equal(data1, data3)
		case <-time.After(time.Second * 5):
			t.Error("file not found")
		}
	})
}

func TestDiscovery(t *testing.T) {
	require := require.New(t)

	node1, err := makeNode(9900)
	require.NoError(err)
	node2, err := makeNode(9901)
	require.NoError(err)

	defer func() {
		node1.Close()
		node2.Close()
		os.RemoveAll(node1.cache)
		os.RemoveAll(node2.cache)
	}()

	time.Sleep(2 * time.Second)

	require.Equal(2, len(node2.host.Peerstore().Peers()))
	require.Equal(2, len(node1.host.Peerstore().Peers()))
}
