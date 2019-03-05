// +build go1.10

package conn

import "net"

func NetPipe() (net.Conn, net.Conn) {
	return net.Pipe()
}
