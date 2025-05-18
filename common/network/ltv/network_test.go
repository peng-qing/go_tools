package ltv

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"testing"
	"time"

	"go_tools/common/network"
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

var clientConf = &LTVClientConfig{
	IP:               "127.0.0.1",
	Port:             30101,
	Mode:             1,
	UsedLittleEndian: true,
	Connection: &LTVConnectionConfig{
		Heartbeat:     5000,
		MaxHeartbeat:  10000,
		ReadTimeout:   1000,
		WriteTimeout:  1000,
		MaxIOReadSize: 1024 * 1024 * 10,
		SendQueueSize: 100,
	},
}

const (
	Heartbeat = 1 // 心跳包类型
)

var serverHbCount = 0
var clientHbCount = 0

//var dispatcher = NewLTVDispatcher(1, 99999, int64(100*time.Millisecond))

func TestServer(t *testing.T) {

	server := NewLTVServer(0, serverConf)
	// 设置心跳回调
	server.SetHeartbeatFunc(func(conn network.IConnection) {
		fmt.Printf("[Server] heartbeat func called, connID:%d\n", conn.GetConnectionID())
		hbMsg := make([]byte, 0)
		serverHbCount++
		hbPacket := NewLTVPacket(Heartbeat, fmt.Appendf(hbMsg, "第 %d 次心跳", serverHbCount))
		if err := conn.SendPacketToQueue(hbPacket); err != nil {
			fmt.Println("[Server] send heartbeat packet failed, err: ", err)
		}
	})
	// 设置消息处理回调
	server.SetDispatchMsg(func(conn network.IConnection, packet network.IPacket) {
		fmt.Printf("[Server] dispatch msg called, packet:%s\n", string(packet.GetData()))
	})
	server.SetOnConnect(func(conn network.IConnection) {
		fmt.Printf("[Server] on connect called, connID:%d\n", conn.GetConnectionID())
	})
	server.SetOnDisconnect(func(conn network.IConnection) {
		fmt.Printf("[Server] on disconnect called, connID:%d\n", conn.GetConnectionID())
	})

	server.Serve()

	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGINT, syscall.SIGTERM)
	select {
	case <-c:
		server.Close()
		break
	}

	time.Sleep(1 * time.Second)
	fmt.Println("[Server] exit success.....")
}

func TestClient(t *testing.T) {
	client := NewLTVClient(clientConf)
	client.SetOnConnect(func(conn network.IConnection) {
		fmt.Println("[Client] on connect called")
	})
	client.SetOnDisconnect(func(conn network.IConnection) {
		fmt.Println("[Client] on disconnect called")
	})
	client.SetHeartbeatFunc(func(conn network.IConnection) {
		fmt.Println("[Client] heartbeat func called")
		hbMsg := make([]byte, 0)
		clientHbCount++
		hbPacket := NewLTVPacket(Heartbeat, fmt.Appendf(hbMsg, "第 %d 次心跳", clientHbCount))
		if err := conn.SendPacketToQueue(hbPacket); err != nil {
			fmt.Println("[Client] send heartbeat packet failed, err: ", err)
		}
	})
	client.SetDispatchMsg(func(conn network.IConnection, packet network.IPacket) {
		fmt.Printf("[Client] dispatch msg called, packet:%s\n", string(packet.GetData()))
	})

	client.Start()

	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGINT, syscall.SIGTERM)
	select {
	case <-c:
		client.Close()
		break
	}

	time.Sleep(1 * time.Second)
	fmt.Println("[Client] exit success.....")
}
