package main

import (
	"encoding/binary"
	"fmt"
	"log"
	"os/exec"

	"github.com/mengelbart/moqtransport"
)

type channelID string

type channel struct {
	ID channelID
}

func newChannel(id channelID) *channel {
	return &channel{
		ID: id,
	}
}

func (r *channel) subscribe(s *moqtransport.Session, srw moqtransport.SubscriptionResponseWriter) {

	track := moqtransport.NewLocalTrack(1, fmt.Sprintf("iptv-moq/%v", r.ID), "")

	err := s.AddLocalTrack(track)

	if err != nil {
		srw.Reject(1, err.Error())
		return
	}
	srw.Accept(track)
	cmd := exec.Command("ffmpeg", "-hide_banner", "-v", "quiet", "-re", "-i", string(r.ID), "-f", "mp4", "-c", "copy", "-bsf:a", "aac_adtstoasc", "-movflags", "cmaf+separate_moof+delay_moov+skip_trailer+frag_every_frame", "-")

	stdout, err := cmd.StdoutPipe()

	if err != nil {
		srw.Reject(1, err.Error())
		return
	}

	if err := cmd.Start(); err != nil {
		srw.Reject(1, err.Error())
		return
	}

	objectID := uint64(0)
	groupID := uint64(0)

	for {
		// get box size and type from the first 8 bytes
		boxHeader, err := completeRead(stdout, 8)
		if err != nil {
			log.Fatalf("Error reading box header: %v", err)
		}

		boxSize := binary.BigEndian.Uint32(boxHeader[:4])
		boxType := string(boxHeader[4:8])

		// reead the box data
		boxData, err := completeRead(stdout, int(boxSize-8))
		if err != nil {
			log.Fatalf("Error reading box data: %v", err)
		}

		// send ftyp and moov boxes as separate objects, and moof+mdat as a single object
		switch boxType {
		case "ftyp":
			// fmt.Printf("%v: ftyp box of size %d\n", r.ID, boxSize)
			payload := append(boxHeader, boxData...)
			sendObject(track, groupID, objectID, payload)
			objectID++

		case "moov":
			// fmt.Printf("%v: moov box of size %d\n", r.ID, boxSize)
			payload := append(boxHeader, boxData...)
			sendObject(track, groupID, objectID, payload)
			objectID++

		case "moof":
			// fmt.Printf("%v: moof box of size %d\n", r.ID, boxSize)
			moofPayload := append(boxHeader, boxData...)

			// read the next box to get the mdat box
			boxHeader, err = completeRead(stdout, 8)
			if err != nil {
				log.Fatalf("Error reading box header: %v", err)
			}

			mdatSize := binary.BigEndian.Uint32(boxHeader[:4])
			mdatType := string(boxHeader[4:8])
			if mdatType != "mdat" {
				log.Fatalf("Expected mdat box, got %s", mdatType)
			}

			mdatData, err := completeRead(stdout, int(mdatSize-8))
			if err != nil {
				log.Fatalf("Error reading mdat data: %v", err)
			}

			// fmt.Printf("%v: mdat box of size %d\n", r.ID, mdatSize)
			mdatPayload := append(boxHeader, mdatData...)

			// Create a single payload with both moof and mdat
			payload := append(moofPayload, mdatPayload...)
			// fmt.Printf(string(payload))
			sendObject(track, groupID, objectID, payload)
			objectID++

		default:
			fmt.Printf("Skipping box of type %s and size %d\n", boxType, boxSize)
			// skip the box
			continue
		}
	}
}
