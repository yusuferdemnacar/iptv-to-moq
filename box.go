package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
)

// Box represents a generic MP4 box
type Box struct {
	Size [4]byte
	Type [4]byte
	Data []byte
}

// ReadBox reads a box from a byte slice
func ReadBox(r io.Reader) (*Box, error) {
	var size [4]byte
	var boxType [4]byte

	// Read box size
	if _, err := io.ReadFull(r, size[:]); err != nil {
		return nil, err
	}

	// Read box type
	if _, err := io.ReadFull(r, boxType[:]); err != nil {
		return nil, err
	}

	// Convert size bytes to uint32
	boxSize := binary.BigEndian.Uint32(size[:])

	// Read box data
	data := make([]byte, boxSize-8)
	if _, err := io.ReadFull(r, data); err != nil {
		return nil, err
	}

	return &Box{Size: size, Type: boxType, Data: data}, nil
}

// GetType returns the type of the box as a string
func (b *Box) GetType() string {
	return string(b.Type[:])
}

// GetSize returns the size of the box as an int
func (b *Box) GetSize() int {
	return int(binary.BigEndian.Uint32(b.Size[:]))
}

// GetHeader returns the first 8 bytes (size and type) of the box
func (b *Box) GetHeader() []byte {
	return append(b.Size[:], b.Type[:]...)
}

// GetData returns the data of the box
func (b *Box) GetData() []byte {
	return b.Data
}

// FindBox finds a child box by type
func FindBox(data []byte, boxType [4]byte) (*Box, error) {
	r := bytes.NewReader(data)
	for {
		box, err := ReadBox(r)
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if box.Type == boxType {
			return box, nil
		}
	}
	return nil, fmt.Errorf("box not found")
}

// getTrackIDFromTfhd extracts the track ID from a tfhd box
func (tfhdBox *Box) getTrackIDFromTfhd() uint32 {
	return binary.BigEndian.Uint32(tfhdBox.Data[4:8])
}

// getHandlerType extracts the handler type from an hdlr box
func (hdlrBox *Box) getHandlerType() string {
	return string(hdlrBox.Data[8:12])
}

// getMediaType determines if a moof box is audio or video
func (moofBox *Box) getMediaType(moovBox *Box) (string, error) {
	// Parse moov box to get track ID to handler type mapping
	trackIDToHandlerType, err := moovBox.parseMoovBox()
	if err != nil {
		return "", err
	}

	// Find the traf box inside the moof box
	trafBox, err := FindBox(moofBox.Data, [4]byte{'t', 'r', 'a', 'f'})
	if err != nil {
		return "", err
	}

	// Find the tfhd box inside the traf box
	tfhdBox, err := FindBox(trafBox.Data, [4]byte{'t', 'f', 'h', 'd'})
	if err != nil {
		return "", err
	}

	// Get the track ID from the tfhd box
	trackID := tfhdBox.getTrackIDFromTfhd()

	// Get the handler type using the track ID
	handlerType, exists := trackIDToHandlerType[trackID]
	if !exists {
		return "", fmt.Errorf("handler type not found for track ID %d", trackID)
	}

	switch handlerType {
	case "vide":
		return "video", nil
	case "soun":
		return "audio", nil
	default:
		return "unknown", nil
	}
}

// parseMoovBox parses the moov box and returns a mapping of track IDs to handler types
func (moovBox *Box) parseMoovBox() (map[uint32]string, error) {
	trackIDToHandlerType := make(map[uint32]string)

	r := bytes.NewReader(moovBox.Data)
	for {
		box, err := ReadBox(r)
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if string(box.Type[:]) == "trak" {
			// Find the tkhd box inside the trak box
			tkhdBox, err := FindBox(box.Data, [4]byte{'t', 'k', 'h', 'd'})
			if err != nil {
				return nil, err
			}
			// Parse the track ID from the tkhd box
			trackID := binary.BigEndian.Uint32(tkhdBox.Data[12:16])

			// Find the mdia box inside the trak box
			mdiaBox, err := FindBox(box.Data, [4]byte{'m', 'd', 'i', 'a'})
			if err != nil {
				return nil, err
			}

			// Find the hdlr box inside the mdia box
			hdlrBox, err := FindBox(mdiaBox.Data, [4]byte{'h', 'd', 'l', 'r'})
			if err != nil {
				return nil, err
			}

			// Get the handler type
			handlerType := hdlrBox.getHandlerType()

			// Map the track ID to the handler type
			trackIDToHandlerType[trackID] = handlerType
		}
	}

	return trackIDToHandlerType, nil
}
