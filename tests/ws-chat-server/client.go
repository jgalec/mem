package main

import (
	"errors"
	"log"
	"net"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

var (
	errNicknameTaken = errors.New("nickname already taken in room")
	errNotInRoom     = errors.New("not in room")
	errEmptyField    = errors.New("nickname and room must not be empty")
)

const (
	writeWait      = 10 * time.Second
	pongWait       = 60 * time.Second
	pingPeriod     = (pongWait * 9) / 10
	maxMessageSize = 512
)

type Client struct {
	hub      *Hub
	conn     *websocket.Conn
	send     chan Message
	nickname string
	rooms    map[string]bool
	mu       sync.Mutex
}

func NewClient(hub *Hub, conn *websocket.Conn) *Client {
	return &Client{
		hub:   hub,
		conn:  conn,
		send:  make(chan Message, 16),
		rooms: make(map[string]bool),
	}
}

func (c *Client) Close() {
	c.conn.Close()
}

func (c *Client) Send(msg Message) {
	select {
	case c.send <- msg:
	default:
	}
}

func (c *Client) ReadPump() {
	defer func() {
		c.hub.Unregister(c)
		c.conn.Close()
	}()

	c.conn.SetReadLimit(maxMessageSize)
	c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	for {
		_, data, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
				log.Printf("read error: %v", err)
			}
			return
		}

		msg, err := ParseMessage(data)
		if err != nil {
			c.Send(Message{Type: TypeError, Text: "invalid message format"})
			continue
		}

		c.handleMessage(msg)
	}
}

func (c *Client) WritePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()

	for {
		select {
		case msg, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			data, err := MarshalMessage(msg)
			if err != nil {
				log.Printf("marshal error: %v", err)
				continue
			}

			if err := c.conn.WriteMessage(websocket.TextMessage, data); err != nil {
				log.Printf("write error: %v", err)
				return
			}
		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

func (c *Client) handleMessage(msg Message) {
	switch msg.Type {
	case TypeJoin:
		c.handleJoin(msg)
	case TypeChat:
		c.handleChat(msg)
	case TypeLeave:
		c.handleLeave(msg)
	case TypeRooms:
		c.handleRooms()
	default:
		c.Send(Message{Type: TypeError, Text: "unknown message type: " + string(msg.Type)})
	}
}

func (c *Client) handleJoin(msg Message) {
	if msg.Nickname == "" || msg.Room == "" {
		c.Send(Message{Type: TypeError, Text: "nickname and room are required"})
		return
	}

	if err := c.hub.JoinRoom(c, msg.Room, msg.Nickname); err != nil {
		c.Send(Message{Type: TypeError, Text: err.Error()})
		return
	}

	c.hub.Broadcast(msg.Room, Message{
		Type:     TypeSystem,
		Room:     msg.Room,
		Text:     msg.Nickname + " has joined the room",
		Nickname: "system",
	}, c)
}

func (c *Client) handleChat(msg Message) {
	if msg.Text == "" {
		c.Send(Message{Type: TypeError, Text: "text must not be empty"})
		return
	}

	if msg.Room == "" {
		c.Send(Message{Type: TypeError, Text: "room is required"})
		return
	}

	if !c.hub.IsInRoom(c, msg.Room) {
		c.Send(Message{Type: TypeError, Text: "not in room: " + msg.Room})
		return
	}

	c.hub.Broadcast(msg.Room, Message{
		Type:     TypeChat,
		Nickname: c.nickname,
		Room:     msg.Room,
		Text:     msg.Text,
	}, c)
}

func (c *Client) handleLeave(msg Message) {
	room := msg.Room
	if room == "" {
		c.Send(Message{Type: TypeError, Text: "room is required"})
		return
	}

	if !c.hub.IsInRoom(c, room) {
		c.Send(Message{Type: TypeError, Text: "not in room: " + room})
		return
	}

	nickname := c.nickname
	c.hub.LeaveRoom(c, room)

	c.hub.Broadcast(room, Message{
		Type:     TypeSystem,
		Room:     room,
		Text:     nickname + " has left the room",
		Nickname: "system",
	}, nil)

	if len(c.rooms) == 0 {
		c.conn.Close()
	}
}

func (c *Client) handleRooms() {
	rooms := c.hub.ListRooms()
	c.Send(Message{
		Type:  TypeRoomList,
		Rooms: rooms,
	})
}

func init() {
	var _ net.Conn
}
