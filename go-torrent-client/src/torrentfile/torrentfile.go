package torrentfile

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"crypto/sha1"
	"fmt"
	"github.com/jackpal/bencode-go"
	"go-torrent-client/src/download"
	"log"
	"math"
	"os"
)

type bencodeInfo struct{
	Pieces      string `bencode:"pieces"`
	PieceLength int    `bencode:"piece length"`
	Length      int    `bencode:"length"`
	Name        string `bencode:"name"`
}

type bencodeData struct{
	Announce string `bencode:"announce"`
	Info bencodeInfo `bencode:"info"`
}

type TorrentData struct{
	Announce string
	InfoHash    [20]byte
	PieceHashes [][]byte
	PieceLength int
	Length      int
	Name        string
}

func printTorrentInfo(data bencodeData){
	fmt.Println("Announcer is: ", data.Announce)
	fmt.Println("Name of torrent is: ", data.Info.Name)
	fmt.Println("Length of File is: ", data.Info.Length)
	fmt.Println("Length of each piece length is: ", data.Info.PieceLength)
	fmt.Println("No. of pieces are: ", math.Ceil(float64(data.Info.Length/data.Info.PieceLength)))
	fmt.Println("Pieces length is: ", len(data.Info.Pieces))
}

func getHashPieces(pcs string) [][]byte {
	noOfPcs := len(pcs)/20
	pcsBytes := []byte(pcs)  //Using byte slice because string itself is Byte String of ASCII

	if noOfPcs%20!=0{
		log.Fatalln("Something is wrong with the torrent File")
	}

	var hashes [][]byte
	i:=0

	for i < len(pcsBytes){
		hashes = append(hashes, pcsBytes[i:i+20])
		i+=20
	}

	return hashes
}

func makeTorrent(d *bencodeData) (TorrentData) {
	hashes := getHashPieces(d.Info.Pieces)
	bi := &d.Info

	//calculate sha1 hash of infohash, this hash is sent to tracker
	var infohash bytes.Buffer
	err := bencode.Marshal(&infohash, *bi)
	if err!=nil{
		log.Fatalln("Problem with InfoHash!")
		return TorrentData{}
	}

	ih := sha1.Sum(infohash.Bytes())

	t := TorrentData{
		Announce: 	d.Announce,
		InfoHash:    ih,
		PieceHashes: hashes,
		PieceLength: d.Info.PieceLength,
		Length:      d.Info.Length,
		Name:        d.Info.Name,
	}
	return t
}

func ParseTorrent(inFile string) (TorrentData) {
	file, err := os.Open(inFile)
	if err!=nil{
		log.Fatalln("Cannot Open Specified Path!")
	}
	defer file.Close()
	reader := bufio.NewReader(file)

	var myd bencodeData
	bencode.Unmarshal(reader, &myd)

	torrent:= makeTorrent(&myd)
	return torrent
}

func StartDownload(data TorrentData,saveTo string) error {
	var ourId [20]byte
	_, err := rand.Read(ourId[:])
	if err!=nil{
		return err
	}

	peers, err := getPeers(ourId, 6881, data)
	if err!=nil{
		return err
	}

	torrent := download.Torrent{
		Peers:       peers,
		OurId:       ourId,
		InfoHash:    data.InfoHash,
		PieceHashes: data.PieceHashes,
		PieceLength: data.PieceLength,
		Length:      data.Length,
		Name:        data.Name,
	}
	buf, err := download.Download(torrent)
	if err!=nil{
		return err
	}
	saver, err := os.Create(saveTo)
	if err != nil {
		return err
	}
	defer saver.Close()
	_, err = saver.Write(buf)
	if err != nil {
		return err
	}
	return nil
}