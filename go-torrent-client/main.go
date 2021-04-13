package main

import (
	"go-torrent-client/src/torrentfile"
	"log"
	"os"
)

func main(){
	openFrom := os.Args[1]
	saveTo := os.Args[2]
	torrent := torrentfile.ParseTorrent(openFrom)

	err := torrentfile.StartDownload(torrent, saveTo)
	if err!=nil{
		log.Fatalln("Cannot Download!", err)
	}
}