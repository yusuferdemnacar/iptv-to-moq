package main

import (
	"fmt"
	"log"
	"os/exec"

	"github.com/mengelbart/moqtransport"
)

type channelID string

type channel struct {
	ID         channelID
	videoTrack *moqtransport.LocalTrack
	audioTrack *moqtransport.LocalTrack
	session    *moqtransport.Session
}

func newChannel(id channelID) *channel {
	return &channel{
		ID:         id,
		videoTrack: nil,
		audioTrack: nil,
		session:    nil,
	}
}

func (r *channel) subscribe(s *moqtransport.Session, sub *moqtransport.Subscription, srw moqtransport.SubscriptionResponseWriter) {

	track := moqtransport.NewLocalTrack(fmt.Sprintf("iptv-moq/%v", r.ID), sub.TrackName)
	// TODO: send the video and audio tracks separately

	err := s.AddLocalTrack(track)
	if err != nil {
		srw.Reject(1, err.Error())
		return
	}

	if sub.TrackName == "video" {
		r.videoTrack = track
	} else if sub.TrackName == "audio" {
		r.audioTrack = track
	} else {
		srw.Reject(1, "invalid track name")
		return
	}
	r.session = s

	srw.Accept(track)
}

func (r *channel) serve() {

	cmd := exec.Command("ffmpeg", "-hide_banner", "-v", "quiet", "-re", "-i", string(r.ID), "-f", "mp4", "-c:v", "libx264", "-preset", "fast", "-tune", "zerolatency", "-c:a", "ac3", "-b:a", "192k", "-movflags", "cmaf+separate_moof+delay_moov+skip_trailer+frag_every_frame", "-")

	stdout, err := cmd.StdoutPipe()

	if err != nil {
		return
	}

	if err := cmd.Start(); err != nil {
		return
	}

	videoObjectID := uint64(0)
	audioObjectID := uint64(0)
	groupID := uint64(0)
	var moovBox *Box

	for {
		box, err := ReadBox(stdout)
		if err != nil {
			log.Fatalf("Error reading box: %v", err)
		}

		// send ftyp and moov boxes as separate objects, and moof+mdat as a single object
		switch box.GetType() {
		case "ftyp":
			// fmt.Printf("ftyp box of size %d\n", box.GetSize())
			payload := append(box.GetHeader(), box.GetData()...)
			err := sendObject(r.videoTrack, groupID, videoObjectID, payload)
			if err != nil {
				fmt.Printf("Error sending object: %v\n", err)
				return
			}
			videoObjectID++

		case "moov":
			// fmt.Printf("moov box of size %d\n", box.GetSize())
			payload := append(box.GetHeader(), box.GetData()...)
			err := sendObject(r.videoTrack, groupID, videoObjectID, payload)
			if err != nil {
				fmt.Printf("Error sending object: %v\n", err)
				return
			}
			moovBox = box
			videoObjectID++

		case "moof":
			moofPayload := append(box.GetHeader(), box.GetData()...)

			// read the next box to get the mdat box
			nextBox, err := ReadBox(stdout)
			if err != nil {
				log.Fatalf("Error reading next box: %v", err)
			}
			if nextBox.GetType() != "mdat" {
				log.Fatalf("Expected mdat box, got %s", nextBox.GetType())
			}

			mdatPayload := append(nextBox.GetHeader(), nextBox.GetData()...)

			// Create a single payload with both moof and mdat
			payload := append(moofPayload, mdatPayload...)
			// fmt.Printf(string(payload))
			mediaType, err := box.getMediaType(moovBox)
			if err != nil {
				log.Fatalf("Error getting media type: %v", err)
			}
			if mediaType == "video" {
				err := sendObject(r.videoTrack, groupID, videoObjectID, payload)
				if err != nil {
					return
				}
				// fmt.Printf("%v mooof box of size %d\n", mediaType, box.GetSize())
				// fmt.Printf("%v mdat box of size %d\n", mediaType, nextBox.GetSize())
				videoObjectID++
			} else if mediaType == "audio" {
				err := sendObject(r.audioTrack, groupID, videoObjectID, payload)
				if err != nil {
					return
				}
				// fmt.Printf("%v mooof box of size %d\n", mediaType, box.GetSize())
				// fmt.Printf("%v mdat box of size %d\n", mediaType, nextBox.GetSize())
				audioObjectID++
			} else {
				log.Fatalf("Unknown media type: %s", mediaType)
			}

		default:
			fmt.Printf("Skipping box of type %s and size %d\n", box.GetType(), box.GetSize())
			// skip the box
			continue
		}
	}
}
