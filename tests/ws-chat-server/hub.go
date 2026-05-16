package main

import (
	"sync"
)

type Hub struct {
	mu      sync.RWMutex
	rooms   map[string]map[*Client]bool
	clients map[*Client]struct{}
}

func NewHub() *Hub {
	return &Hub{
		rooms:   make(map[string]map[*Client]bool),
		clients: make(map[*Client]struct{}),
	}
}

func (h *Hub) Register(client *Client) {
	h.mu.Lock()
	h.clients[client] = struct{}{}
	h.mu.Unlock()
}

func (h *Hub) Unregister(client *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()

	delete(h.clients, client)

	for room := range client.rooms {
		if members, ok := h.rooms[room]; ok {
			delete(members, client)
			if len(members) == 0 {
				delete(h.rooms, room)
			}
		}
	}
}

func (h *Hub) JoinRoom(client *Client, room, nickname string) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	if _, ok := h.rooms[room]; !ok {
		h.rooms[room] = make(map[*Client]bool)
	}

	for c := range h.rooms[room] {
		if c.nickname == nickname {
			return errNicknameTaken
		}
	}

	h.rooms[room][client] = true
	client.rooms[room] = true
	client.nickname = nickname

	return nil
}

func (h *Hub) LeaveRoom(client *Client, room string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if members, ok := h.rooms[room]; ok {
		delete(members, client)
		if len(members) == 0 {
			delete(h.rooms, room)
		}
	}
	delete(client.rooms, room)
}

func (h *Hub) Broadcast(room string, msg Message, exclude *Client) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	members, ok := h.rooms[room]
	if !ok {
		return
	}

	for client := range members {
		if client == exclude {
			continue
		}
		client.Send(msg)
	}
}

func (h *Hub) ListRooms() []string {
	h.mu.RLock()
	defer h.mu.RUnlock()

	rooms := make([]string, 0, len(h.rooms))
	for name := range h.rooms {
		rooms = append(rooms, name)
	}
	return rooms
}

func (h *Hub) IsInRoom(client *Client, room string) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return client.rooms[room]
}

func (h *Hub) CloseAll() {
	h.mu.RLock()
	defer h.mu.RUnlock()

	for client := range h.clients {
		client.Close()
	}
}
