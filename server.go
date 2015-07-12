/*
server.go implements RFC 6143 §7.6 Server-to-Client Messages.
See http://tools.ietf.org/html/rfc6143#section-7.6 for more info.
*/
package vnc

import (
	"encoding/binary"
	"fmt"
	"io"
	"log"
)

const (
	FramebufferUpdate = uint8(iota)
	SetColorMapEntries
	Bell
	ServerCutText
)

// A ServerMessage implements a message sent from the server to the client.
type ServerMessage interface {
	// The type of the message that is sent down on the wire.
	Type() uint8

	// Read reads the contents of the message from the reader. At the point
	// this is called, the message type has already been read from the reader.
	// This should return a new ServerMessage that is the appropriate type.
	Read(*ClientConn, io.Reader) (ServerMessage, error)
}

// Rectangle represents a rectangle of pixel data.
type Rectangle struct {
	X, Y, Width, Height uint16
	Enc                 Encoding
}

type FramebufferUpdateMessage struct {
	Msg     uint8
	Pad     [1]byte
	NumRect uint16
	Rects   []Rectangle
}

func NewFramebufferUpdateMessage(rects []Rectangle) *FramebufferUpdateMessage {
	return &FramebufferUpdateMessage{
		Msg:     FramebufferUpdate,
		NumRect: uint16(len(rects)),
		Rects:   rects,
	}
}

func (m *FramebufferUpdateMessage) Type() uint8 {
	return m.Msg
}

func (m *FramebufferUpdateMessage) Read(c *ClientConn, r io.Reader) (ServerMessage, error) {
	// if c.debug {
	// 	log.Print("FramebufferUpdateMessage.Read()")
	// }

	// Read off the padding
	if _, err := io.ReadFull(r, m.Pad[:]); err != nil {
		return nil, err
	}

	var numRects uint16
	if err := binary.Read(r, binary.BigEndian, &numRects); err != nil {
		return nil, err
	}

	// Build the map of encodings supported
	encMap := make(map[int32]Encoding)
	for _, e := range c.Encodings() {
		encMap[e.Type()] = e
	}

	// We must always support the raw encoding
	encMap[Raw] = NewRawEncoding([]Color{})

	rects := make([]Rectangle, numRects)
	for i := uint16(0); i < numRects; i++ {
		var encodingType int32

		rect := &rects[i]
		data := []interface{}{
			&rect.X,
			&rect.Y,
			&rect.Width,
			&rect.Height,
			&encodingType,
		}
		for _, val := range data {
			if err := binary.Read(r, binary.BigEndian, val); err != nil {
				return nil, err
			}
		}
		enc, ok := encMap[encodingType]
		if !ok {
			return nil, fmt.Errorf("unsupported encoding type: %d", encodingType)
		}

		var err error
		rect.Enc, err = enc.Read(c, rect, r)
		if err != nil {
			return nil, err
		}
	}

	return NewFramebufferUpdateMessage(rects), nil
}

// SetColorMapEntries is sent by the server to set values into
// the color map. This message will automatically update the color map
// for the associated connection, but contains the color change data
// if the consumer wants to read it.
//
// See RFC 6143 Section 7.6.2

// Color represents a single color in a color map.
type Color struct {
	R, G, B uint16
}

type SetColorMapEntriesMessage struct {
	FirstColor uint16
	Colors     []Color
}

func (*SetColorMapEntriesMessage) Type() uint8 {
	return SetColorMapEntries
}

func (*SetColorMapEntriesMessage) Read(c *ClientConn, r io.Reader) (ServerMessage, error) {
	if c.debug {
		log.Print("SetColorMapEntriesMessage.Read()")
	}

	// Read off the padding
	var padding [1]byte
	if _, err := io.ReadFull(r, padding[:]); err != nil {
		return nil, err
	}

	var result SetColorMapEntriesMessage
	if err := binary.Read(r, binary.BigEndian, &result.FirstColor); err != nil {
		return nil, err
	}

	var numColors uint16
	if err := binary.Read(r, binary.BigEndian, &numColors); err != nil {
		return nil, err
	}

	result.Colors = make([]Color, numColors)
	for i := uint16(0); i < numColors; i++ {

		color := &result.Colors[i]
		data := []interface{}{
			&color.R,
			&color.G,
			&color.B,
		}

		for _, val := range data {
			if err := binary.Read(r, binary.BigEndian, val); err != nil {
				return nil, err
			}
		}

		// Update the connection's color map
		c.colorMap[result.FirstColor+i] = *color
	}

	return &result, nil
}

// Bell signals that an audible bell should be made on the client.
//
// See RFC 6143 Section 7.6.3
type BellMessage struct{}

func (*BellMessage) Type() uint8 {
	return Bell
}

func (*BellMessage) Read(c *ClientConn, _ io.Reader) (ServerMessage, error) {
	if c.debug {
		log.Print("BellMessage.Read()")
	}

	return new(BellMessage), nil
}

// ServerCutText indicates the server has new text in the cut buffer.
//
// See RFC 6143 Section 7.6.4
type ServerCutTextMessage struct {
	Text string
}

func (*ServerCutTextMessage) Type() uint8 {
	return ServerCutText
}

func (*ServerCutTextMessage) Read(c *ClientConn, r io.Reader) (ServerMessage, error) {
	if c.debug {
		log.Print("ServerCutTextMessage.Read()")
	}

	// Read off the padding
	var padding [1]byte
	if _, err := io.ReadFull(r, padding[:]); err != nil {
		return nil, err
	}

	var textLength uint32
	if err := binary.Read(r, binary.BigEndian, &textLength); err != nil {
		return nil, err
	}

	textBytes := make([]uint8, textLength)
	if err := binary.Read(r, binary.BigEndian, &textBytes); err != nil {
		return nil, err
	}

	return &ServerCutTextMessage{string(textBytes)}, nil
}
