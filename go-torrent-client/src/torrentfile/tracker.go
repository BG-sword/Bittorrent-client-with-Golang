package torrentfile

import (
	"github.com/jackpal/bencode-go"
	"go-torrent-client/src/peers"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

type TrackerResponse struct {
	Interval int    `bencode:"interval"`
	Peers    string `bencode:"peers"`
}

func buildTracker(ourId [20]byte, port uint16, data TorrentData)(string, error){
	tracker, err := url.Parse(data.Announce)
	if err!=nil{
		return "", err
	}
	params := url.Values{
		"info_hash":  []string{string(data.InfoHash[:])},
		"peer_id":    []string{string(ourId[:])},
		"port":       []string{strconv.Itoa(int(port))},
		"uploaded":   []string{"0"},
		"downloaded": []string{"0"},
		"compact":    []string{"1"},
		"left":       []string{strconv.Itoa(data.Length)},
	}
	tracker.RawQuery = params.Encode()
	return tracker.String(), nil
}

func getPeers(ourId [20]byte, port uint16, data TorrentData) ([]peers.Peer, error){
	tracker, err := buildTracker(ourId, port, data)
	if err != nil {
		return nil, err
	}

	c := &http.Client{Timeout: 15 * time.Second}
	resp, err := c.Get(tracker)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	trackerResp := TrackerResponse{}
	err = bencode.Unmarshal(resp.Body, &trackerResp)
	if err != nil {
		return nil, err
	}

	return peers.Unmarshal([]byte(trackerResp.Peers))
}
