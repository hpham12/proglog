package loadbalance

import (
	"strings"
	"sync"
	"sync/atomic"
	"google.golang.org/grpc/balancer"
	"google.golang.org/grpc/balancer/base"
)

var _ base.PickerBuilder = (*Picker)(nil)
var _ balancer.Picker = (*Picker)(nil)

type Picker struct {
	mu 			sync.RWMutex
	leader 		balancer.SubConn
	followers 	[]balancer.SubConn
	current 	uint64
}

// Pickers use the builder pattern just like resolvers. gRPC passes a map
// of subconnections with information about those subconnections to Build()
// to set up the picker - behind the scenes, gRPC connected to the addresses
// that our resolver discovered.
func (p *Picker) Build(buildInfo base.PickerBuildInfo) balancer.Picker {
	p.mu.Lock()
	defer p.mu.Unlock()
	var followers []balancer.SubConn
	for sc, scInfo := range buildInfo.ReadySCs {
		isLeader := scInfo.Address.Attributes.Value("is_leader").(bool)
		if isLeader {
			p.leader = sc
		} else {
			followers = append(followers, sc)
		}
	}

	p.followers = followers

	return p
}

// Pick the server to handle the request
func (p *Picker) Pick(info balancer.PickInfo) (balancer.PickResult, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	var result balancer.PickResult
	if strings.Contains(info.FullMethodName, "Produce") || len(p.followers) == 0 {
		result.SubConn = p.leader
	} else if strings.Contains(info.FullMethodName, "Consume") || len(p.followers) == 0 {
		result.SubConn = p.nextFollower()
	}
	if result.SubConn == nil {
		return result, balancer.ErrNoSubConnAvailable
	}

	return result, nil
}

// Choose the next follower using a round-robin algorithm
func (p *Picker) nextFollower() balancer.SubConn {
	cur := atomic.AddUint64(&p.current, uint64(1))
	len := uint64(len(p.followers))
	idx := int(cur % len)
	return p.followers[idx]
}

func init() {
	balancer.Register(
		base.NewBalancerBuilder(Name, &Picker{}, base.Config{}),
	)
}
