package ltv

import (
	"fmt"
	"go_tools/common/network"
	"math/rand"
	"net"
	"os"
	"os/signal"
	"syscall"
	"testing"
	"time"
)

var serverConf = &LTVServerConfig{
	IP:               "127.0.0.1",
	Port:             30101,
	WsPort:           0,
	WsPath:           "/",
	Mode:             1,
	MaxConn:          10,
	UsedLittleEndian: true,
	TimerQueueSize:   100,
	Frequency:        100,
	Connection: &LTVConnectionConfig{
		Heartbeat:     5000,
		MaxHeartbeat:  10000,
		ReadTimeout:   1000,
		WriteTimeout:  1000,
		MaxIOReadSize: 1024 * 1024 * 10,
		SendQueueSize: 100,
	},
}

func TestServer(t *testing.T) {
	server := NewLTVServer(0, serverConf)
	server.SetHeartbeatFunc(func(conn network.IConnection) {
		fmt.Printf("heartbeat func called, connID:%d\n", conn.GetConnectionID())
		// 回一个心跳
		err := conn.SendToQueue([]byte("心跳回包"))
		if err != nil {
			return
		}
	})
	server.SetDispatchMsg(func(conn network.IConnection, packet network.IPacket) {
		fmt.Printf("dispatch msg called, packet:%s\n", string(packet.GetData()))
	})
	server.SetOnConnect(func(conn network.IConnection) {
		fmt.Printf("on connect called, connID:%d\n", conn.GetConnectionID())
	})
	server.SetOnDisconnect(func(conn network.IConnection) {
		fmt.Printf("on disconnect called, connID:%d\n", conn.GetConnectionID())
	})

	server.Serve()

	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGINT, syscall.SIGTERM)
	select {
	case <-c:
		server.Close()
		time.Sleep(1 * time.Second)
	}
}

func TestClient(t *testing.T) {
	conn, err := net.Dial("tcp", "127.0.0.1:30101")
	if err != nil {
		fmt.Println("client dial failed, err: ", err)
		return
	}
	protocolCoder := NewLTVProtocolCoder(serverConf.UsedLittleEndian)
	for {
		sec := rand.Int63() % serverConf.Connection.MaxHeartbeat
		time.Sleep(time.Duration(sec) * time.Millisecond)
		packet := NewLTVPacket(1, []byte("测试发包"))
		data, err := protocolCoder.Encode(packet)
		if err != nil {
			fmt.Printf("protocol encode packet failed")
		}
		_, err = conn.Write(data)
		if err != nil {
			fmt.Println("client write failed, err: ", err)
			return
		}
		buf := make([]byte, 10240)
		n, err := conn.Read(buf)
		if err != nil {
			fmt.Println("client read failed, err: ", err)
			return
		}
		fmt.Println("client read: ", string(buf[:n]))
	}
}
