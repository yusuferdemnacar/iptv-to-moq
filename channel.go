package main

import (
	"fmt"
	"os/exec"
	"sync"
	"time"

	"github.com/mengelbart/moqtransport"
)

type channel struct {
	ID           string
	videoTrack   *moqtransport.LocalTrack
	audioTrack   *moqtransport.LocalTrack
	ftypBox      *Box
	moovBox      *Box
	sessions     []*moqtransport.Session
	sessionsLock sync.Mutex
}

func newChannel(channelID string, ftypBox *Box, moovBox *Box) *channel {
	return &channel{
		ID:         channelID,
		videoTrack: moqtransport.NewLocalTrack(fmt.Sprintf("iptv-moq/%v", channelID), "video"),
		audioTrack: moqtransport.NewLocalTrack(fmt.Sprintf("iptv-moq/%v", channelID), "audio"),
		ftypBox:    ftypBox,
		moovBox:    moovBox,
		sessions:   []*moqtransport.Session{},
	}
}

func (c *channel) subscriberCount() int {
	return c.videoTrack.SubscriberCount()
}

func (c *channel) subscribe(s *moqtransport.Session, sub *moqtransport.Subscription, srw moqtransport.SubscriptionResponseWriter) {

	var track *moqtransport.LocalTrack

	if sub.TrackName == "video" {
		track = c.videoTrack
	} else if sub.TrackName == "audio" {
		track = c.audioTrack
	} else {
		srw.Reject(1, "invalid track name")
		return
	}

	err := s.AddLocalTrack(track)
	if err != nil {
		srw.Reject(1, err.Error())
		return
	}

	c.sessionsLock.Lock()
	defer c.sessionsLock.Unlock()
	c.sessions = append(c.sessions, s)

	srw.Accept(track)

	if sub.TrackName == "video" {
		ftypPayload := append(c.ftypBox.GetHeader(), c.ftypBox.GetData()...)
		moovPayload := append(c.moovBox.GetHeader(), c.moovBox.GetData()...)
		err := sendObject(c.videoTrack, 0, 0, ftypPayload)
		if err != nil {
			fmt.Printf("Error sending ftyp box: %v", err)
		}
		err = sendObject(c.videoTrack, 0, 1, moovPayload)
		if err != nil {
			fmt.Printf("Error sending moov box: %v", err)
		}
	}

}

func getInitBoxes(channelID string) (*Box, *Box, error) {

	var fytpBox *Box
	var moovBox *Box

	cmd := exec.Command("ffmpeg", "-hide_banner", "-v", "quiet", "-re", "-i", channelID, "-f", "mp4", "-c:v", "libx264", "-preset", "fast", "-tune", "zerolatency", "-c:a", "ac3", "-b:a", "192k", "-movflags", "cmaf+separate_moof+delay_moov+skip_trailer+frag_every_frame", "-")

	stdout, err := cmd.StdoutPipe()

	if err != nil {
		return nil, nil, err
	}

	if err := cmd.Start(); err != nil {
		return nil, nil, err
	}

	time.Sleep(2 * time.Second)

	for {
		box, err := ReadBox(stdout)
		if err != nil {
			fmt.Printf("Error reading box: %v\n", err)
			return nil, nil, err
		}

		switch box.GetType() {
		case "ftyp":
			fytpBox = box
		case "moov":
			moovBox = box
		}

		if fytpBox != nil && moovBox != nil {
			break
		}
	}

	return fytpBox, moovBox, nil
}

func (c *channel) serveMoofMdat() {

	cmd := exec.Command("ffmpeg", "-hide_banner", "-v", "quiet", "-re", "-i", string(c.ID), "-f", "mp4", "-c:v", "libx264", "-preset", "fast", "-tune", "zerolatency", "-c:a", "ac3", "-b:a", "192k", "-movflags", "cmaf+separate_moof+delay_moov+skip_trailer+frag_every_frame", "-")

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

	for {
		box, err := ReadBox(stdout)
		if err != nil {
			fmt.Printf("Error reading box: %v\n", err)
		}

		// send ftyp and moov boxes as separate objects, and moof+mdat as a single object
		if box.GetType() == "moof" {
			moofPayload := append(box.GetHeader(), box.GetData()...)

			// read the next box to get the mdat box
			nextBox, err := ReadBox(stdout)
			if err != nil {
				fmt.Printf("Error reading next box: %v", err)
			}
			if nextBox.GetType() != "mdat" {
				fmt.Printf("Expected mdat box, got %s", nextBox.GetType())
			}

			mdatPayload := append(nextBox.GetHeader(), nextBox.GetData()...)

			// Create a single payload with both moof and mdat
			payload := append(moofPayload, mdatPayload...)
			// fmt.Printf(string(payload))
			mediaType, err := box.getMediaType(c.moovBox)
			if err != nil {
				fmt.Printf("Error getting media type: %v", err)
				return
			}
			if mediaType == "video" {
				err := sendObject(c.videoTrack, groupID, videoObjectID, payload)
				if err != nil {
					return
				}
				// fmt.Printf("%v mooof box of size %d\n", mediaType, box.GetSize())
				// fmt.Printf("%v mdat box of size %d\n", mediaType, nextBox.GetSize())
				videoObjectID++
			} else if mediaType == "audio" {
				err := sendObject(c.audioTrack, groupID, videoObjectID, payload)
				if err != nil {
					return
				}
				// fmt.Printf("%v mooof box of size %d\n", mediaType, box.GetSize())
				// fmt.Printf("%v mdat box of size %d\n", mediaType, nextBox.GetSize())
				audioObjectID++
			} else {
				fmt.Printf("Unknown media type: %s", mediaType)
			}
		}
	}
}
