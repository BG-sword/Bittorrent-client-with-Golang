package connection

import (
	"bytes"
	"fmt"
	"go-torrent-client/src/bitfield"
	"go-torrent-client/src/message"
	"go-torrent-client/src/peers"
	"io"
	"net"
	"time"
)

type Client struct {
	Conn     net.Conn
	Choked   bool
	Bitfield bitfield.Bitfield
	Peer     peers.Peer
	InfoHash [20]byte
	OurID   [20]byte
}

type Handshake struct {
	Pstr     string
	InfoHash [20]byte
	OurID   [20]byte
}

func newHandshake(infoHash, ourID [20]byte) *Handshake {
	return &Handshake{
		Pstr:     "BitTorrent protocol",
		InfoHash: infoHash,
		OurID:   ourID,
	}
}

func (h *Handshake) serialize() []byte {
	/*
		format of handshake is like this
		19(in hex -> 0x13) + pstr("BitTorrent protocol") + 8 unset bits(indicates no extension support) + infoHash of our torrent(20bytes) + our peerId
		1 + 19 + 8 + 20 + 20 => 68
	*/
	buf := make([]byte, len(h.Pstr)+49) //pstr + all remaining fields length
	buf[0] = byte(len(h.Pstr)) //set to 0x13
	curr := 1
	curr += copy(buf[curr:], h.Pstr) //set to "BitTorrent protocol"
	curr += copy(buf[curr:], make([]byte, 8)) //8 unset bits
	curr += copy(buf[curr:], h.InfoHash[:]) //torrent infoHash
	curr += copy(buf[curr:], h.OurID[:]) //our unique ID
	return buf
}

func readHandshake(r io.Reader) (*Handshake, error) {
	lengthBuf := make([]byte, 1)
	_, err := io.ReadFull(r, lengthBuf)
	if err != nil {
		return nil, err
	}
	pstrlen := int(lengthBuf[0])

	if pstrlen == 0 {
		err := fmt.Errorf("pstrlen cannot be 0")
		return nil, err
	}

	handshakeBuf := make([]byte, 48+pstrlen)
	_, err = io.ReadFull(r, handshakeBuf)
	if err != nil {
		return nil, err
	}

	var infoHash, ourID [20]byte

	copy(infoHash[:], handshakeBuf[pstrlen+8:pstrlen+8+20])
	copy(ourID[:], handshakeBuf[pstrlen+8+20:])

	h := Handshake{
		Pstr:     string(handshakeBuf[0:pstrlen]),
		InfoHash: infoHash,
		OurID:   ourID,
	}

	return &h, nil
}

func makeHandShake(conn net.Conn, infoHash [20]byte, ourID [20]byte)(*Handshake, error){
	conn.SetDeadline(time.Now().Add(3*time.Second))
	defer conn.SetDeadline(time.Time{})

	// make an handshake, our request
	req := newHandshake(infoHash, ourID)
	_, err := conn.Write(req.serialize())
	if err != nil {
		return nil, err
	}

	//receive handshake
	res, err := readHandshake(conn)
	if err != nil {
		return nil, err
	}
	if !bytes.Equal(res.InfoHash[:], infoHash[:]) {
		return nil, fmt.Errorf("Infohash mismatched!")
	}
	return res, nil

}

func getBitfield(conn net.Conn) (bitfield.Bitfield, error) {
	conn.SetDeadline(time.Now().Add(5 * time.Second))
	defer conn.SetDeadline(time.Time{}) // Disable the deadline

	msg, err := message.Read(conn)
	if err != nil {
		return nil, err
	}
	if msg.ID != message.MsgBitfield {
		err := fmt.Errorf("Wrong bitfield received")
		return nil, err
	}

	return msg.Payload, nil
}

func NewClient(peer peers.Peer, ourID [20]byte, infoHash [20]byte) (*Client, error) {
	conn, err := net.DialTimeout("tcp", peer.String(), 3*time.Second)
	if err != nil {
		return nil, err
	}

	_, err = makeHandShake(conn, infoHash, ourID)
	if err != nil {
		conn.Close()
		return nil, err
	}

	bf, err := getBitfield(conn)
	if err != nil {
		conn.Close()
		return nil, err
	}

	return &Client{
		Conn:     conn,
		Choked:   true,
		Bitfield: bf,
		Peer:     peer,
		InfoHash: infoHash,
		OurID: ourID,
	}, nil
}

func (c *Client) Read() (*message.Message, error) {
	msg, err := message.Read(c.Conn)
	return msg, err
}

// SendRequest sends a Request message to the peer
func (c *Client) SendRequest(index, begin, length int) error {
	req := message.FormatRequest(index, begin, length)
	_, err := c.Conn.Write(req.Serialize())
	return err
}

// SendInterested sends an Interested message to the peer
func (c *Client) SendInterested() error {
	msg := message.Message{ID: message.MsgInterested}
	_, err := c.Conn.Write(msg.Serialize())
	return err
}

// SendNotInterested sends a NotInterested message to the peer
func (c *Client) SendNotInterested() error {
	msg := message.Message{ID: message.MsgNotInterested}
	_, err := c.Conn.Write(msg.Serialize())
	return err
}

// SendUnchoke sends an Unchoke message to the peer
func (c *Client) SendUnchoke() error {
	msg := message.Message{ID: message.MsgUnchoke}
	_, err := c.Conn.Write(msg.Serialize())
	return err
}

// SendHave sends a Have message to the peer
func (c *Client) SendHave(index int) error {
	msg := message.FormatHave(index)
	_, err := c.Conn.Write(msg.Serialize())
	return err
}

