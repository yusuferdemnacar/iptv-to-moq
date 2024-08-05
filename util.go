package main

import (
	"context"
	"fmt"

	"github.com/mengelbart/moqtransport"
)

func sendObject(track *moqtransport.LocalTrack, groupID, objectID uint64, payload []byte) error {
	object := moqtransport.Object{
		GroupID:              groupID,
		ObjectID:             objectID,
		ForwardingPreference: moqtransport.ObjectForwardingPreferenceStream,
		Payload:              payload,
	}
	// fmt.Println("subscriber count:", track.SubscriberCount())
	if track.SubscriberCount() == 0 {
		return fmt.Errorf("no subscribers for track")
	}
	track.WriteObject(context.Background(), object)
	return nil
}
