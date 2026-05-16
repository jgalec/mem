package main

import "encoding/json"

type MessageType string

const (
	TypeJoin     MessageType = "join"
	TypeChat     MessageType = "chat"
	TypeSystem   MessageType = "system"
	TypeRooms    MessageType = "rooms"
	TypeRoomList MessageType = "room_list"
	TypeError    MessageType = "error"
	TypeLeave    MessageType = "leave"
)

type Message struct {
	Type     MessageType `json:"type"`
	Nickname string      `json:"nickname,omitempty"`
	Room     string      `json:"room,omitempty"`
	Text     string      `json:"text,omitempty"`
	Rooms    []string    `json:"rooms,omitempty"`
}

func ParseMessage(data []byte) (Message, error) {
	var msg Message
	if err := json.Unmarshal(data, &msg); err != nil {
		return Message{}, err
	}
	return msg, nil
}

func MarshalMessage(msg Message) ([]byte, error) {
	return json.Marshal(msg)
}
