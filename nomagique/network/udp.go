package network

import (
	"context"
	"net"
	"time"

	"github.com/theapemachine/errnie"
	"golang.org/x/net/ipv4"
)

/*
UDPMulticast provides LAN-scoped broadcast transport over native UDP multicast.
One write maps to one datagram. Listener mode joins the multicast group and
receives from any member. Dialer mode sends datagrams to the group.
*/
type UDPMulticast struct {
	sub              *net.UDPConn
	pub              *net.UDPConn
	group            *net.UDPAddr
	timeout          time.Duration
	readPollDeadline time.Duration
	ctx              context.Context
}

type udpMulticastOption func(*UDPMulticast) error

/*
NewUDPMulticast constructs a native UDP multicast transport.
Use UDPMulticastWithListener on receivers and UDPMulticastWithDialer on senders.
*/
func NewUDPMulticast(ctx context.Context, opts ...udpMulticastOption) (*UDPMulticast, error) {
	udp := &UDPMulticast{
		timeout:          10 * time.Second,
		readPollDeadline: 50 * time.Millisecond,
		ctx:              ctx,
	}

	for _, opt := range opts {
		if err := opt(udp); err != nil {
			return nil, err
		}
	}

	return udp, nil
}

/*
Read receives the next UDP datagram from the joined multicast group.
*/
func (udp *UDPMulticast) Read(p []byte) (n int, err error) {
	if udp.sub == nil {
		return 0, errnie.Err(errnie.IO, "[udp] not bound", nil)
	}

	for {
		if err := udp.sub.SetReadDeadline(time.Now().Add(udp.readPollDeadline)); err != nil {
			return 0, err
		}

		if n, _, err = udp.sub.ReadFromUDP(p); err == nil {
			return n, nil
		}

		if udp.ctx.Err() != nil {
			return n, udp.ctx.Err()
		}

		// Continue on timeout error to support cancellation check
	}
}

/*
Write publishes one UDP datagram to the multicast group.
*/
func (udp *UDPMulticast) Write(p []byte) (int, error) {
	if udp.pub == nil {
		return 0, errnie.Err(errnie.IO, "[udp] not bound", nil)
	}

	if err := udp.pub.SetWriteDeadline(time.Now().Add(udp.timeout)); err != nil {
		return 0, err
	}

	return udp.pub.Write(p)
}

/*
Close shuts down both the multicast listener and publisher.
*/
func (udp *UDPMulticast) Close() error {
	if udp.pub != nil {
		_ = udp.pub.Close()
	}
	if udp.sub != nil {
		_ = udp.sub.Close()
	}
	return nil
}

/*
UDPMulticastWithListener joins the multicast group and binds a receiving socket.
The same transport also creates a sender socket so listener-side writes work.
*/
func UDPMulticastWithListener(group string, iface string) udpMulticastOption {
	return func(udp *UDPMulticast) error {
		groupAddress, err := net.ResolveUDPAddr("udp4", group)
		if err != nil {
			return err
		}

		var networkInterface *net.Interface
		if iface != "" {
			networkInterface, err = net.InterfaceByName(iface)
			if err != nil {
				return err
			}
		}

		listener, err := net.ListenMulticastUDP("udp4", networkInterface, groupAddress)
		if err != nil {
			return err
		}

		if err := listener.SetReadBuffer(1 << 20); err != nil {
			_ = listener.Close()
			return err
		}

		publisher, err := net.DialUDP("udp4", nil, groupAddress)
		if err != nil {
			_ = listener.Close()
			return err
		}

		if networkInterface != nil {
			if err := ipv4.NewPacketConn(publisher).SetMulticastInterface(networkInterface); err != nil {
				_ = listener.Close()
				_ = publisher.Close()
				return err
			}
		}

		if err := ipv4.NewPacketConn(publisher).SetMulticastLoopback(true); err != nil {
			_ = listener.Close()
			_ = publisher.Close()
			return err
		}

		udp.group = groupAddress
		udp.sub = listener
		udp.pub = publisher
		return nil
	}
}

/*
UDPMulticastWithDialer opens a sender socket connected to the multicast group.
*/
func UDPMulticastWithDialer(group string) udpMulticastOption {
	return func(udp *UDPMulticast) error {
		groupAddress, err := net.ResolveUDPAddr("udp4", group)
		if err != nil {
			return err
		}

		publisher, err := net.DialUDP("udp4", nil, groupAddress)
		if err != nil {
			return err
		}

		if err := ipv4.NewPacketConn(publisher).SetMulticastLoopback(true); err != nil {
			_ = publisher.Close()
			return err
		}

		udp.group = groupAddress
		udp.pub = publisher
		return nil
	}
}

/*
UDPMulticastWithReadPollDeadline sets how long each ReadFromUDP spin waits
before refreshing context cancellation.
*/
func UDPMulticastWithReadPollDeadline(deadline time.Duration) udpMulticastOption {
	return func(udp *UDPMulticast) error {
		if deadline > 0 {
			udp.readPollDeadline = deadline
		}
		return nil
	}
}

/*
UDPMulticastWithContext replaces the default background context.
*/
func UDPMulticastWithContext(ctx context.Context) udpMulticastOption {
	return func(udp *UDPMulticast) error {
		udp.ctx = ctx
		return nil
	}
}

/*
UDPMulticastWithAeronDir is retained for API stability and is a no-op.
*/
func UDPMulticastWithAeronDir(dir string) udpMulticastOption {
	return func(udp *UDPMulticast) error {
		return nil
	}
}
