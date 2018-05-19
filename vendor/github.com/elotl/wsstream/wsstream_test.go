package wsstream

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
)

const (
	stdinMsg  = "stdinMsg"
	stdoutMsg = "stdoutMsg"
	stderrMsg = "stderrMsg"
)

func unpackMsg(msg []byte) (uint32, string, error) {
	f := Frame{}
	err := json.Unmarshal(msg, &f)
	if err != nil {
		return 0, "", fmt.Errorf("Corrupted message: %s", err)
	}

	if f.Type == FrameTypeMessage {
		return f.Channel, string(f.Message), nil
	} else {
		err := fmt.Errorf("Unexpected websocket frame type: %s", f.Type)
		return 0, "", err
	}
}

func readChanTimeout(c <-chan []byte, t time.Duration) (uint32, string, error) {
	select {
	case m := <-c:
		return unpackMsg(m)
	case <-time.After(t):
		return uint32(0), "", fmt.Errorf("timeout")
	}
}

func makeWSHandler(t *testing.T) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		wsUpgrader := websocket.Upgrader{
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
		}

		ws, err := wsUpgrader.Upgrade(w, r, nil)
		if err != nil {
			if _, ok := err.(websocket.HandshakeError); !ok {
				log.Println(err)
			}
			return
		}
		runServer(t, ws)
	}
}

func runServer(t *testing.T, conn *websocket.Conn) {
	ws := NewWSStream(conn)
	defer ws.CloseAndCleanup()
	c, val, err := readChanTimeout(ws.Read(), 3*time.Second)
	assert.NoError(t, err)
	assert.Equal(t, uint32(0), c)
	assert.Equal(t, stdinMsg, val)
	err = ws.WriteMsg(1, []byte(stdoutMsg))
	assert.NoError(t, err)
	err = ws.WriteMsg(2, []byte(stderrMsg))
	assert.NoError(t, err)
	time.Sleep(100 * time.Millisecond)
}

func runClient(t *testing.T, conn *websocket.Conn) {
	wsc := NewWSStream(conn)
	defer wsc.CloseAndCleanup()
	err := wsc.WriteMsg(0, []byte(stdinMsg))
	assert.NoError(t, err)
	c, val, err := readChanTimeout(wsc.Read(), 3*time.Second)
	assert.NoError(t, err)
	assert.Equal(t, uint32(1), c)
	assert.Equal(t, stdoutMsg, val)
	c, val, err = readChanTimeout(wsc.Read(), 3*time.Second)
	assert.NoError(t, err)
	assert.Equal(t, uint32(2), c)
	assert.Equal(t, stderrMsg, string(val))
	time.Sleep(150 * time.Millisecond)
}

func TestWSStream(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(makeWSHandler(t)))
	u := url.URL{Scheme: "ws", Host: s.Listener.Addr().String(), Path: "/foo"}
	c, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	assert.NoError(t, err)

	runClient(t, c)
}
