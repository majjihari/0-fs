package p2p

import (
	"bufio"
	"context"

	msgpackCodec "gx/ipfs/QmRDePEiL4Yupq5EkcK3L3ko3iMgYaqUdLu7xc1kqs7dnV/go-multicodec/msgpack"
	inet "gx/ipfs/QmXoz9o2PT3tEzf7hicegwex5UgVP54n3k82K7jrWFyN86/go-libp2p-net"
)

// pattern: /protocol-name/request-or-response-message/version
const (
	shareReq  = "/0fs/sharerReq/0.0.1"
	shareResp = "/0fs/shareResp/0.0.1"
)

var msgpackHandle = msgpackCodec.DefaultMsgpackHandle()

// ShareStorageProtocol type
type ShareStorageProtocol struct {
	node *Node // local host
}

type ShareReq struct {
	Name string //absolute path of the file requested
}

type ShareResp struct {
	Name  string //absolute path of the file requested
	Found bool   // if the file is found or not
	Data  []byte // content of the file requested, nil if Found is false
}

func NewShareStorageProtocol(node *Node) *ShareStorageProtocol {
	p := &ShareStorageProtocol{node: node}
	node.host.SetStreamHandler(shareReq, p.onShareReq)
	node.host.SetStreamHandler(shareResp, p.onShareResp)
	return p
}

func (p *ShareStorageProtocol) RequestFile(name string) {

	for _, hostID := range p.node.host.Peerstore().Peers() {

		if hostID == p.node.host.ID() {
			// don't send to ourself
			continue
		}

		log.Infof("%s: Sending share request to: %s for file %s", p.node.host.ID(), hostID, name)

		// create share request
		req := &ShareReq{Name: name}

		s, err := p.node.host.NewStream(context.Background(), hostID, shareReq)
		if err != nil {
			log.Error(err)
			continue
		}

		if err := sendMsgPackMessage(req, s); err != nil {
			log.Error(err)
			continue
		}
	}
}

func (p *ShareStorageProtocol) onShareReq(s inet.Stream) {

	// get request data
	req := &ShareReq{}
	decoder := msgpackCodec.Multicodec(msgpackHandle).Decoder(bufio.NewReader(s))
	err := decoder.Decode(req)
	if err != nil {
		log.Error(err)
		return
	}

	log.Infof("%s: Received share request from %s. path:%s", s.Conn().LocalPeer(), s.Conn().RemotePeer(), req.Name)

	found, data := p.node.getFromCache(req.Name)

	resp := &ShareResp{
		Name:  req.Name,
		Found: found,
		Data:  data,
	}

	log.Infof("%s: path:%s found:%v", s.Conn().LocalPeer(), req.Name, found)

	// send the response
	s, err = p.node.host.NewStream(context.Background(), s.Conn().RemotePeer(), shareResp)
	if err != nil {
		log.Error(err)
		return
	}

	if err := sendMsgPackMessage(resp, s); err != nil {
		log.Error(err)
	}
}

func (p *ShareStorageProtocol) onShareResp(s inet.Stream) {

	resp := &ShareResp{}
	decoder := msgpackCodec.Multicodec(msgpackHandle).Decoder(bufio.NewReader(s))
	err := decoder.Decode(resp)
	if err != nil {
		return
	}

	if !resp.Found {
		log.Errorf("%s: Share response received from %s: data not found", s.Conn().LocalPeer().String(), s.Conn().RemotePeer().String())
		return
	}

	log.Infof("%s: Share response received from %s: data found", s.Conn().LocalPeer().String(), s.Conn().RemotePeer().String())
	out, exists := p.node.requestsChan[resp.Name]
	if !exists {
		log.Errorf("%v: output channel for %s does not exist, file has already been fetch from another node", p.node.host.ID().String(), resp.Name)
		return
	}

	// send the data over the channel, the caller that initiated the requests should select on the channel
	out <- resp.Data
	close(out)
	delete(p.node.requestsChan, resp.Name)
}

func sendMsgPackMessage(data interface{}, s inet.Stream) error {
	writer := bufio.NewWriter(s)
	enc := msgpackCodec.Multicodec(msgpackHandle).Encoder(writer)
	err := enc.Encode(data)
	if err != nil {
		log.Error(err)
		return err
	}
	writer.Flush()
	return nil
}
