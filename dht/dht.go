package dht

import (
	"context"
	"net"
	"strconv"
	"time"

	"github.com/neoql/btlet/bencode"
)

var defaultBootstrap = []string{
	"router.bittorrent.com:6881",
	"router.utorrent.com:6881",
	"dht.transmissionbt.com:6881",
}

// Node is dht node.
type Node struct {
	Addr *net.UDPAddr
	ID   string
}

// MessageDisposer is used for dispose message.
type MessageDisposer interface {
	DisposeQuery(src *net.UDPAddr, transactionID string, q string, args bencode.RawMessage) error
	DisposeResponse(src *net.UDPAddr, transactionID string, resp bencode.RawMessage) error
	DisposeError(src *net.UDPAddr, transactionID string, code int, describe string) error
	DisposeUnknownMessage(src *net.UDPAddr,  message map[string]interface{}) error
}

// Core is the core of dht.
type Core struct {
	conn       *net.UDPConn
	maxWorkers int
}

// NewCore returns a new Core instance.
func NewCore(ip string, port int) (*Core, error) {
	addr, err := net.ResolveUDPAddr("udp", ip+":"+strconv.Itoa(port))
	if err != nil {
		return nil, err
	}

	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		return nil, err
	}

	return &Core{
		conn: conn,
	}, nil
}

// SetMaxWorkers set the max goroutine will be create to dispose dht message.
// If maxWorkers smaller than 0. it won't set upper limit.
func (core *Core) SetMaxWorkers(n int) {
	core.maxWorkers = n
}

// Addr returns the Addr of self.
func (core *Core) Addr() net.Addr {
	return core.conn.LocalAddr()
}

// Serv starts serving.
func (core *Core) Serv(ctx context.Context, disposer MessageDisposer) (err error) {
	defer core.conn.Close()
	
	var dispMsg func (disposer MessageDisposer, addr *net.UDPAddr, data []byte)
	if core.maxWorkers <= 0 {
		dispMsg = func (disposer MessageDisposer, addr *net.UDPAddr, data []byte)  {
			go core.disposeMessage(disposer, addr, data)
		}
	} else {
		lmt := make(chan struct{}, core.maxWorkers)
		dispMsg = func (disposer MessageDisposer, addr *net.UDPAddr, data []byte)  {
			lmt <- struct{}{}
			go func() {
				core.disposeMessage(disposer, addr, data)
				<-lmt
			}()
		}
	}

loop:
	for {
		select {
		case <-ctx.Done():
			break loop
		default:
		}

		buf := make([]byte, 8196)
		n, addr, err := core.conn.ReadFromUDP(buf)
		if err != nil {
			// TODO: handle error
			continue
		}

		dispMsg(disposer, addr, buf[:n])
	}
	return nil
}

// SendMessage will send message to the node.
func (core *Core) SendMessage(dst *net.UDPAddr, msg interface{}) error {
	data, err := bencode.Marshal(msg)
	if err != nil {
		// TODO: handle error
		return err
	}
	core.conn.SetWriteDeadline(time.Now().Add(time.Second * 15))
	_, err = core.conn.WriteToUDP(data, dst)
	if err != nil {
		// TODO: handle error
		return err
	}

	return nil
}

func (core *Core) disposeMessage(disposer MessageDisposer, addr *net.UDPAddr, data []byte) (err error) {
	defer func() {
		if e := recover(); e != nil {
			// TODO: handle error
			err = e.(error)
			return
		}
	}()

	var msg Message

	err = bencode.Unmarshal(data, &msg)
	if err != nil {
		return err
	}

	switch msg.Y {
	default:
	case "q":
		return disposer.DisposeQuery(addr, msg.TransactionID, msg.Q, msg.Args)
	case "r":
		return disposer.DisposeResponse(addr, msg.TransactionID, msg.Resp)
	case "e":
	}

	return nil
}

// Handle returns a handle
func (core *Core) Handle(nodeID string) Handle {
	return &handle{
		core:   core,
		nodeID: nodeID,
	}
}

type handle struct {
	core   *Core
	nodeID string
}

func (h *handle) SendMessage(dst *net.UDPAddr, msg interface{}) error {
	return h.core.SendMessage(dst, msg)
}

func (h *handle) NodeID() string {
	return h.nodeID
}
