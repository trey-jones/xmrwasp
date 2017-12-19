package main

import (
	"sort"
	"sync"
	"time"

	"go.uber.org/zap"
)

var (
	directorInstance *Director
	oneTimeDirector  = sync.Once{}
)

type Director struct {
	statInterval time.Duration

	workers chan *Worker

	currentProxyID uint64
	proxies        map[uint64]*Proxy

	// stat tracking only
	lastTotalShares uint64
}

func GetDirector() *Director {
	oneTimeDirector.Do(func() {
		directorInstance = newDirector()
	})

	return directorInstance
}

func newDirector() *Director {
	d := &Director{
		statInterval: time.Duration(Config().StatInterval) * time.Second,
		workers:      make(chan *Worker, 1),
		proxies:      make(map[uint64]*Proxy),
	}
	go d.assignWorkers()

	return d
}

func (d *Director) assignWorkers() {
	statPrinter := time.NewTicker(d.statInterval)
	defer statPrinter.Stop()
	for {
		select {
		case w := <-d.workers:
			d.Use(w)
		case <-statPrinter.C:
			d.printStats()
		}
	}
}

func (d *Director) printStats() {
	totalProxies := 0
	totalWorkers := 0
	proxyIDs := make([]int, 0)
	var totalSharesSubmitted uint64
	var aliveSince time.Time
	for ID, p := range d.proxies {
		proxyIDs = append(proxyIDs, int(ID))
		totalProxies++
		totalWorkers += len(p.workers)
		totalSharesSubmitted += p.shares
	}
	recentShares := totalSharesSubmitted - d.lastTotalShares
	d.lastTotalShares = totalSharesSubmitted
	if len(proxyIDs) > 0 {
		sort.Ints(proxyIDs)
		oldestProxyID := proxyIDs[0]
		oldestProxy := d.proxies[uint64(oldestProxyID)]
		aliveSince = oldestProxy.aliveSince
	}
	duration := time.Now().Sub(aliveSince)

	zap.S().Infof("Alive for %s \t|\t %v proxies \t|\t %v workers \t|\t %v shares (+%v)",
		duration, totalProxies, totalWorkers, totalSharesSubmitted, recentShares)
}

func (d *Director) removeProxy(pr *Proxy) {
	delete(d.proxies, pr.ID)
}

func (d *Director) Use(w *Worker) {
	var pr *Proxy
	for _, p := range d.proxies {
		if p.Ready() {
			pr = p
			break
		}
	}
	if pr == nil {
		pr = NewProxy(d.nextProxyID())
		pr.director = d
		d.proxies[pr.ID] = pr
	}

	pr.Add(w)
}

func (d *Director) nextProxyID() uint64 {
	d.currentProxyID++
	return d.currentProxyID
}
