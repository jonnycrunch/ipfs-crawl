package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"log"
	"time"

	host "github.com/libp2p/go-libp2p-host"
	dht "github.com/libp2p/go-libp2p-kad-dht"
	peer "github.com/libp2p/go-libp2p-peer"
	pstore "github.com/libp2p/go-libp2p-peerstore"
)

const WORKERS = 16

type Crawler struct {
	ctx context.Context
	h   host.Host
	dht *dht.IpfsDHT

	peers map[peer.ID]struct{}
	work  chan pstore.PeerInfo
}

func NewCrawler(ctx context.Context, h host.Host, dht *dht.IpfsDHT) *Crawler {
	c := &Crawler{ctx: ctx, h: h, dht: dht,
		peers: make(map[peer.ID]struct{}),
		work:  make(chan pstore.PeerInfo, WORKERS),
	}

	for i := 0; i < WORKERS; i++ {
		go c.worker()
	}

	return c
}

func (c *Crawler) Crawl() {
	anchor := make([]byte, 32)
	for {

		_, err := rand.Read(anchor)
		if err != nil {
			log.Fatal(err)
		}

		str := base64.RawStdEncoding.EncodeToString(anchor)
		c.crawlFromAnchor(str)

		select {
		case <-time.After(60 * time.Second):
		case <-c.ctx.Done():
			return
		}
	}
}

func (c *Crawler) crawlFromAnchor(key string) {
	log.Printf("Crawling from anchor %s\n", key)

	pch, err := c.dht.GetClosestPeers(c.ctx, key)
	if err != nil {
		log.Fatal(err)
	}

	for p := range pch {
		c.crawlPeer(p)
	}
}

func (c *Crawler) crawlPeer(p peer.ID) {
	_, ok := c.peers[p]
	if ok {
		return
	}

	log.Printf("Crawling peer %s\n", p.Pretty())

	ctx, cancel := context.WithTimeout(c.ctx, 60*time.Second)
	pi, err := c.dht.FindPeer(ctx, p)
	cancel()

	if err != nil {
		log.Printf("Peer not found: %s", p.Pretty())
		return
	}

	c.peers[p] = struct{}{}
	c.work <- pi

	ctx, cancel = context.WithTimeout(c.ctx, 60*time.Second)
	pch, err := c.dht.FindPeersConnectedToPeer(ctx, p)
	cancel()

	if err != nil {
		log.Printf("Can't find peers connected to peer %s: %s", p.Pretty(), err.Error())
		return
	}

	for pip := range pch {
		c.crawlPeer(pip.ID)
	}
}

func (c *Crawler) worker() {
	for {
		select {
		case pi, ok := <-c.work:
			if !ok {
				return
			}
			c.tryConnect(pi)

		case <-c.ctx.Done():
			return
		}
	}
}

func (c *Crawler) tryConnect(pi pstore.PeerInfo) {
	log.Printf("Connecting to %s (%d)", pi.ID.Pretty(), len(pi.Addrs))

	ctx, cancel := context.WithTimeout(c.ctx, 60*time.Second)
	defer cancel()

	err := c.h.Connect(ctx, pi)
	if err != nil {
		log.Printf("FAILED to connect to %s: %s", pi.ID.Pretty(), err.Error())
	} else {
		log.Printf("CONNECTED to %s", pi.ID.Pretty())
	}
}