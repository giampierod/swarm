package zookeeper

import (
	"path"
	"strings"
	"time"

	log "github.com/Sirupsen/logrus"
	"github.com/docker/swarm/discovery"
	"github.com/samuel/go-zookeeper/zk"
)

type ZkDiscoveryService struct {
	conn      *zk.Conn
	path      string
	heartbeat int
}

func init() {
	discovery.Register("zk", &ZkDiscoveryService{})
}

func (s *ZkDiscoveryService) Initialize(uris string, heartbeat int) error {
	var (
		// split here because uris can contain multiples ips
		// like `zk://192.168.0.1,192.168.0.2,192.168.0.3/path`
		parts = strings.SplitN(uris, "/", 2)
		ips   = strings.Split(parts[0], ",")
	)

	conn, _, err := zk.Connect(ips, time.Second)

	if err != nil {
		return err
	}

	s.conn = conn
	s.path = "/" + parts[1]
	s.heartbeat = heartbeat

	_, err = conn.Create(s.path, []byte{1}, 0, zk.WorldACL(zk.PermAll))
	if err != nil {
		// if key already existed, then skip
		if err != zk.ErrNodeExists {
			return err
		}
	}

	return nil
}

func (s *ZkDiscoveryService) Fetch() ([]*discovery.Node, error) {
	addrs, _, err := s.conn.Children(s.path)

	if err != nil {
		return nil, err
	}

	return s.createNodes(addrs), nil
}

func (s *ZkDiscoveryService) createNodes(addrs []string) (nodes []*discovery.Node) {
	nodes = make([]*discovery.Node, 0)
	if addrs == nil {
		return
	}

	for _, addr := range addrs {
		nodes = append(nodes, discovery.NewNode(addr))
	}
	return
}

func (s *ZkDiscoveryService) Watch(callback discovery.WatchCallback) {

	addrs, _, eventChan, err := s.conn.ChildrenW(s.path)
	if err != nil {
		log.Debugf("[ZK] Watch aborted")
		return
	}
	nodes := s.createNodes(addrs)
	callback(nodes)

	for e := range eventChan {
		if e.Type == zk.EventNodeChildrenChanged {
			log.Debugf("[ZK] Watch triggered")
			nodes, err := s.Fetch()
			if err == nil {
				callback(nodes)
			}
		}

	}

}

func (s *ZkDiscoveryService) Register(addr string) error {
	newpath := path.Join(s.path, addr)

	// check existing for the parent path first
	exist, _, err := s.conn.Exists(s.path)
	if err != nil {
		return err
	}

	// create parent first
	if exist == false {

		_, err = s.conn.Create(s.path, []byte{1}, 0, zk.WorldACL(zk.PermAll))
		if err != nil {
			return err
		}
		_, err = s.conn.Create(newpath, []byte(addr), 0, zk.WorldACL(zk.PermAll))
		return err

	} else {

		exist, _, err = s.conn.Exists(newpath)
		if err != nil {
			return err
		}

		if exist {
			err = s.conn.Delete(newpath, -1)
			if err != nil {
				return err
			}
		}

		_, err = s.conn.Create(newpath, []byte(addr), 0, zk.WorldACL(zk.PermAll))
		return err
	}

	return nil
}
