// Design notes: This package was originally designed to have 3
// channels for reading that represented stdin, stdout, stderr, also a
// channel that contained the exit code.  We don't actually use that
// anywhere in the product.  In this implementation we just pass data
// through to gRPC so just pass on the raw json and let the other side
// figure it out.
package wsstream

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/golang/glog"
	"github.com/gorilla/websocket"
)

const (
	BytesProtocol = "milpa.bytes"
)

var (
	wsBufSize = 1000
)

type FrameType string

const (
	FrameTypeMessage  FrameType = "Message"
	FrameTypeExitCode FrameType = "ExitCode"
)

type Frame struct {
	Protocol string    `json:"protocol"`
	Type     FrameType `json:"type"`
	Channel  uint32    `json:"channel"`
	Message  []byte    `json:"message"`
	ExitCode uint32    `json:"exitCode"`
}

type WebsocketParams struct {
	// Time allowed to write the file to the client.
	writeWait time.Duration
	// Time we're allowed to wait for a pong reply
	pongWait time.Duration
	// Send pings to client with this period. Must be less than pongWait.
	pingPeriod time.Duration
}

type WSStream struct {
	// The reader will close(closed), that's how we detect the
	// connection has been shut down
	closed chan struct{}
	// If we want to close this from the server side, we fire
	// a message into closeMsgChan, that'll write a close
	// message from the write loop
	closeMsgChan chan struct{}
	// writeChan is used internally to pump messages to the write
	// loop, this ensures we only write from one goroutine (writing is
	// not threadsafe).
	writeChan chan []byte
	// We write the received messages to readRawChan,
	// standard readChans are nil.
	readChan chan []byte
	// Websocket parameters
	params WebsocketParams
	// The underlying gorilla websocket object
	conn *websocket.Conn
}

func NewWSStream(conn *websocket.Conn) *WSStream {
	ws := &WSStream{
		readChan:     make(chan []byte, wsBufSize),
		closed:       make(chan struct{}),
		writeChan:    make(chan []byte, wsBufSize),
		closeMsgChan: make(chan struct{}),
		params: WebsocketParams{
			writeWait:  10 * time.Second,
			pongWait:   15 * time.Second,
			pingPeriod: 10 * time.Second,
		},
		conn: conn,
	}
	go ws.StartReader()
	go ws.StartWriteLoop()
	return ws
}

func (ws *WSStream) Closed() <-chan struct{} {
	return ws.closed
}

// CloseAndCleanup must be called.  Should be called by the user of
// the stream in response to hearing about the close from selecting on
// Closed().  Should only be called once but MUST be called by the
// user of the WSStream or else we'll leak the open WS connection.
func (ws *WSStream) CloseAndCleanup() error {
	select {
	case <-ws.closed:
		// if we've already closed the conn then dont' try to write on
		// the conn.
	default:
		// If we haven't already closed the connection (ws.closed),
		// then write a closed message, wait for it to be sent and
		// then close the underlying connection
		ws.closeMsgChan <- struct{}{}
		<-ws.closeMsgChan
	}

	// It's possible we want to wrap this in a sync.Once but for now,
	// all clients are pretty clean since they just create the
	// websocket and defer ws.Close()
	return ws.conn.Close()
}

func (ws *WSStream) Read() <-chan []byte {
	return ws.readChan
}

func (ws *WSStream) WriteMsg(channel int, msg []byte) error {
	return ws.write(FrameTypeMessage, channel, msg, uint32(0))
}

func (ws *WSStream) WriteExit(code uint32) error {
	return ws.write(FrameTypeExitCode, 0, nil, code)
}

func (ws *WSStream) write(frameType FrameType, channel int, msg []byte, code uint32) error {
	select {
	case <-ws.closed:
		return fmt.Errorf("Cannot write to a closed websocket")
	default:
		f := Frame{
			Protocol: BytesProtocol,
			Type:     frameType,
			Channel:  uint32(channel),
			Message:  msg,
			ExitCode: code,
		}
		b, err := json.Marshal(f)
		if err != nil {
			return err
		}
		ws.writeChan <- b
	}
	return nil
}

func (ws *WSStream) StartReader() {
	ws.conn.SetReadDeadline(time.Now().Add(ws.params.pongWait))
	ws.conn.SetPongHandler(func(string) error {
		ws.conn.SetReadDeadline(time.Now().Add(ws.params.pongWait))
		return nil
	})
	for {
		_, msg, err := ws.conn.ReadMessage()
		if err != nil {
			if !websocket.IsCloseError(err, websocket.CloseNormalClosure) {
				glog.Errorln("Closing connection after error:", err)
			}
			close(ws.closed)
			return
		}
		ws.readChan <- msg
	}
}

func (ws *WSStream) StartWriteLoop() {
	pingTicker := time.NewTicker(ws.params.pingPeriod)
	defer pingTicker.Stop()
	for {
		select {
		case <-ws.closed:
			return
		case msg := <-ws.writeChan:
			_ = ws.conn.SetWriteDeadline(time.Now().Add(ws.params.writeWait))
			err := ws.conn.WriteMessage(websocket.TextMessage, msg)
			if err != nil {
				glog.Errorln("Error writing msg:", err)
			}
		case <-ws.closeMsgChan:
			_ = ws.conn.WriteMessage(websocket.CloseMessage,
				websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
			ws.closeMsgChan <- struct{}{}
		case <-pingTicker.C:
			_ = ws.conn.SetWriteDeadline(time.Now().Add(ws.params.writeWait))
			err := ws.conn.WriteMessage(websocket.PingMessage, []byte{})
			if err != nil {
				if !websocket.IsCloseError(err, websocket.CloseNormalClosure) {
					glog.Errorln("Abnormal error in ping loop:", err)
				}
				return
			}
		}
	}
}
