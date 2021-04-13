package download

import (
	"bytes"
	"crypto/sha1"
	"fmt"
	"go-torrent-client/src/connection"
	"go-torrent-client/src/message"
	"go-torrent-client/src/peers"
	"log"
	"runtime"
	"time"
)


// MaxBlockSize is the largest number of bytes a request can ask for
const MaxBlockSize = 16384

// MaxBacklog is the number of unfulfilled requests a client can have in its pipeline
const MaxBacklog = 5

type Torrent struct{
	Peers       []peers.Peer
	OurId       [20]byte
	InfoHash    [20]byte
	PieceHashes [][]byte
	PieceLength int
	Length      int
	Name        string
}

type onePiece struct{
	index int
	hash []byte
	length int
}

type outputData struct{
	index int
	out []byte
}

type pieceProgress struct {
	index      int
	client     *connection.Client
	buf        []byte
	downloaded int
	requested  int
	backlog    int
}

func (state *pieceProgress) readMessage() error {
	msg, err := state.client.Read() // this call blocks
	if err != nil {
		return err
	}

	if msg == nil { // keep-alive
		return nil
	}

	switch msg.ID {
	case message.MsgUnchoke:
		state.client.Choked = false
	case message.MsgChoke:
		state.client.Choked = true
	case message.MsgHave:
		index, err := message.ParseHave(msg)
		if err != nil {
			return err
		}
		state.client.Bitfield.SetPiece(index)
	case message.MsgPiece:
		n, err := message.ParsePiece(state.index, state.buf, msg)
		if err != nil {
			return err
		}
		state.downloaded += n
		state.backlog--
	}
	return nil
}

func attemptDownloadPiece(c *connection.Client, op *onePiece) ([]byte, error) {
	log.SetFlags(log.Lshortfile)
	state := pieceProgress{
		index:  op.index,
		client: c,
		buf:    make([]byte, op.length),
	}

	c.Conn.SetDeadline(time.Now().Add(35 * time.Second))
	defer c.Conn.SetDeadline(time.Time{}) // Disable the deadline

	for state.downloaded < op.length {
		// If unchoked, send requests until we have enough unfulfilled requests
		if !state.client.Choked {
			for state.backlog < MaxBacklog && state.requested < op.length {
				blockSize := MaxBlockSize
				// Last block might be shorter than the typical block
				if op.length-state.requested < blockSize {
					blockSize = op.length - state.requested
				}

				err := c.SendRequest(op.index, state.requested, blockSize)
				if err != nil {
					return nil, err
				}
				state.backlog++
				state.requested += blockSize
			}
		}

		err := state.readMessage()
		if err != nil {
			return nil, err
		}
	}

	return state.buf, nil
}

func checkIntegrity(pw *onePiece, buf []byte) error {
	hash := sha1.Sum(buf)
	if !bytes.Equal(hash[:], pw.hash[:]) {
		return fmt.Errorf("Index %d failed integrity check", pw.index)
	}
	return nil
}


func startDownload(torrent Torrent, peer peers.Peer, queue chan *onePiece, output chan *outputData){
	client, err := connection.NewClient(peer, torrent.OurId, torrent.InfoHash)
	if err!=nil{
		log.Printf("Cannot Handshake with peeer %s, Disconnecting", peer.IP)
		return
	}

	defer client.Conn.Close()
	log.Printf("Completed Handshake with ", peer)

	client.SendUnchoke()
	client.SendInterested()

	for op := range queue {
		if !client.Bitfield.HasPiece(op.index) {
			queue <- op
			continue
		}
		// Download the piece
		buf, err := attemptDownloadPiece(client, op)
		if err != nil {
			log.Println("one piece error", err)
			queue <- op // Put piece back on the queue
			return
		}

		err = checkIntegrity(op, buf)
		if err != nil {
			log.Printf("Piece #%d failed integrity check\n", op.index)
			queue <- op // Put piece back on the queue
			continue
		}

		client.SendHave(op.index)
		output <- &outputData{op.index, buf}
	}

}

func Download(torrent Torrent)([]byte, error){
	log.Println("Downloading file", torrent.Name)
	queue := make(chan *onePiece, len(torrent.PieceHashes))
	output := make(chan *outputData)

	for idx, hsh := range torrent.PieceHashes{
		begin := idx * torrent.PieceLength
		end := begin + torrent.PieceLength
		if end > torrent.Length {
			end = torrent.Length
		}
		length := end-begin

		queue <- &onePiece{
			index:  idx,
			hash:   hsh,
			length: length,
		}
	}

	for _, peer := range torrent.Peers{
		go startDownload(torrent, peer, queue, output)
	}

	// Collect results into a buffer until full
	buf := make([]byte, torrent.Length)
	donePieces := 0
	for donePieces < len(torrent.PieceHashes) {
		res := <- output
		begin := res.index * torrent.PieceLength
		end := begin + torrent.PieceLength
		if end > torrent.Length {
			end = torrent.Length
		}
		copy(buf[begin:end], res.out)
		donePieces++

		percent := float64(donePieces) / float64(len(torrent.PieceHashes)) * 100
		numWorkers := runtime.NumGoroutine() - 1 // subtract 1 for main thread
		log.Printf("(%0.2f%%) Downloaded piece #%d from %d peers\n", percent, res.index, numWorkers)
	}
	close(queue)

	return buf, nil
}