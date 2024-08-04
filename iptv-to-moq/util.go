package main

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/mengelbart/moqtransport"
)

func completeRead(stdout io.Reader, size int) ([]byte, error) {
	var buf []byte
	remaining := size

	for remaining > 0 {
		chunk := make([]byte, remaining)
		n, err := io.ReadFull(stdout, chunk)
		if err != nil {
			if err == io.EOF && n > 0 {
				// Append the partially read data and return
				buf = append(buf, chunk[:n]...)
				return buf, nil
			} else if err == io.EOF {
				fmt.Println("EOF reached, waiting for more data...")
				time.Sleep(1 * time.Second)
				continue
			} else {
				return nil, fmt.Errorf("error reading box data: %v", err)
			}
		}
		buf = append(buf, chunk[:n]...)
		remaining -= n
	}

	return buf, nil
}

func sendObject(track *moqtransport.LocalTrack, groupID, objectID uint64, payload []byte) {
	object := moqtransport.Object{
		GroupID:              groupID,
		ObjectID:             objectID,
		ObjectSendOrder:      objectID,
		ForwardingPreference: moqtransport.ObjectForwardingPreferenceStream,
		Payload:              payload,
	}
	track.WriteObject(context.Background(), object)
}
