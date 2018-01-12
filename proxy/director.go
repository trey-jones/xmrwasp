package proxy

import (
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/trey-jones/xmrwasp/config"
)

var (
	directorInstance      *Director
	directorInstantiation = sync.Once{}
)

// Director might be refactored to "ProxyGroup"
type Director struct {
	statInterval time.Duration

	workers chan Worker

	currentProxyID uint64
	proxies        map[uint64]*Proxy
	newProxyMu     sync.Mutex

	// stat tracking only
	lastTotalShares uint64
}

func GetDirector() *Director {
	directorInstantiation.Do(func() {
		directorInstance = newDirector()
	})

	return directorInstance
}

func newDirector() *Director {
	d := &Director{
		statInterval: time.Duration(config.Get().StatInterval) * time.Second,

		proxies: make(map[uint64]*Proxy),
	}
	go d.run()

	return d
}

// Stats is a struct containing information about server uptime and activity, generated on demand
type Stats struct {
	Timestamp time.Time

	Alive     time.Duration
	Proxies   int
	Workers   int
	Shares    uint64
	NewShares uint64

	debug map[string]interface{}
}

func (d *Director) addProxy() *Proxy {
	p := New(d.nextProxyID())
	p.director = d
	d.proxies[p.ID] = p

	return p
}

func (d *Director) run() {
	statPrinter := time.NewTicker(d.statInterval)
	defer statPrinter.Stop()
	for {
		<-statPrinter.C
		d.printStats()
	}
}

func (d *Director) printStats() {
	stats := d.GetStats()

	fmt.Printf("Alive for %s \t|\t %v proxies \t|\t %v workers \t|\t %v shares (+%v)\n",
		stats.Alive, stats.Proxies, stats.Workers, stats.Shares, stats.NewShares)
}

func (d *Director) removeProxy(pr *Proxy) {
	delete(d.proxies, pr.ID)
}

func (d *Director) nextProxyID() uint64 {
	d.currentProxyID++
	return d.currentProxyID
}

// NextProxy gets the first available proxy that has room.
// If no proxy is available, a new one is created.
func (d *Director) NextProxy() *Proxy {
	// This takes care of the race, but might bottleneck - TODO revisit this later.
	// consider storing nextproxy until full/notready then getting a new one?  still a race...
	d.newProxyMu.Lock()
	defer d.newProxyMu.Unlock()
	var pr *Proxy
	for _, p := range d.proxies {
		if p.isReady() {
			pr = p
			break
		}
	}
	if pr == nil {
		// avoid locking in most cases by looping once first
		pr = d.addProxy()
	}

	return pr
}

func (d *Director) GetStats() *Stats {
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
	duration := time.Now().Sub(aliveSince).Truncate(1 * time.Second)

	stats := &Stats{
		Timestamp: time.Now(),
		Alive:     duration,
		Proxies:   totalProxies,
		Workers:   totalWorkers,
		Shares:    totalSharesSubmitted,
		NewShares: recentShares,
	}

	// if debug, populate debug

	return stats
}
