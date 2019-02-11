package main

// Will read the new segments for the lowest quality segment, converting to the required format and sending off to the transcriber at gcp
//
// phase one (complete): will do simple json output w/ client polling and processing
// phase two (todo)    : will do live webvtt, letting hls.js do everything on the client side
//

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	speech "cloud.google.com/go/speech/apiv1"
	"github.com/fsnotify/fsnotify"
	speechpb "google.golang.org/genproto/googleapis/cloud/speech/v1"
)

const ffmpeg = "../../bin/ffmpeg"

var transcriptionHistory map[string]bool

func main() {

	transcriptionHistory = make(map[string]bool)

	source := "" // Path to video input source
	outputDirectory := "./content/"
	outputPath := outputDirectory + "%v/playlist.m3u8"

	segmentFilename := outputDirectory + "%v/%04d.m4s"

	cmd := exec.Command(
		ffmpeg,
		"-i", source,
		"-ignore_unknown",
		"-acodec", "aac",
		"-ar", "44100",
		"-ac", "2",
		"-async", "1",
		"-vsync", "-1",
		"-vcodec", "libx264",
		"-x264opts", "keyint=60:no-scenecut",
		"-profile:v", "high",
		"-level", "4.1",
		"-tune", "zerolatency",
		"-segment_list_flags", "live",
		"-flags", "+cgop",
		"-preset", "veryfast",
		"-bsf:a", "aac_adtstoasc",
		"-hls_segment_type", "fmp4",
		"-hls_time", "10",
		"-hls_list_size", "24",
		"-hls_flags", "delete_segments+omit_endlist",
		"-hls_segment_filename", segmentFilename,

		"-b:v:0", "1000k",
		"-s:v:0", "426x240",
		"-b:a:0", "64k",

		"-b:v:1", "1500k",
		"-s:v:1", "640x360",
		"-b:a:1", "128k",

		"-b:v:2", "2000k",
		"-s:v:2", "896x504",
		"-b:a:2", "192k",

		"-b:v:3", "5000k",
		"-s:v:3", "1280x720",
		"-b:a:3", "256k",

		"-map", "0:v", "-map", "0:a",
		"-map", "0:v", "-map", "0:a",
		"-map", "0:v", "-map", "0:a",
		"-map", "0:v", "-map", "0:a",

		"-var_stream_map", "v:0,a:0 v:1,a:1 v:2,a:2, v:3,a:3",
		"-master_pl_name", "master.m3u8",
		"-master_pl_publish_rate", "1",
		"-hide_banner",
		"-reconnect_at_eof",
		"-reconnect_streamed",
		"-reconnect_delay_max", "3",

		outputPath,
	)

	//
	// TODO better logging
	//

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// Starting the transcriber
	go transcriber()

	err := cmd.Run()
	if err != nil {
		fmt.Println("[main] could not start encoder, err: ", err)
	}
}

func transcriber() {

	// creates a new file watcher
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		fmt.Println("[transcriber] could not create new watcher: ", err)
		return
	}

	wd, err := os.Getwd()
	if err != nil {
		fmt.Println("[transcriber] error Getwd(), err: ", err)
		return
	}

	defer watcher.Close()

	done := make(chan bool)

	go func() {
		var ready = false

		for {
			select {
			// watch for events
			case event := <-watcher.Events:

				var shouldProcessAudio = ready && event.Op == fsnotify.Write && strings.Contains(event.Name, ".m4s")
				if shouldProcessAudio {
					segment := event.Name
					if !transcriptionHistory[segment] {
						transcriptionHistory[segment] = true
						processAudio(segment)
					} else {
						fmt.Println("[transcriber] Skipping processing of segment: ", segment)
					}
				}

				if event.Op == fsnotify.Create {

					// Hack, plz remove me
					if !ready {
						if strings.Contains(event.Name, "/content/0") {

							fmt.Println("[transcriber] adding new watch folder: ", wd+"/content/0/")
							if err := watcher.Add(wd + "/content/0/"); err != nil {
								fmt.Println("[transcriber] error adding content watcher: ", err)
							}

							// TODO remove original watcher

							ready = true
						}
					}

				}

			case err := <-watcher.Errors:
				fmt.Println("[transcriber] fsnotify error: ", err)
			}
		}
	}()

	if err := watcher.Add(wd + "/content/"); err != nil {
		fmt.Println("[transcriber] error adding initial watcher", err)
	}

	<-done
}

func processAudio(segmentPath string) {
	fmt.Println("[processAudio] for: ", segmentPath)
	_, segmentFilename := filepath.Split(segmentPath)

	wd, err := os.Getwd()
	if err != nil {
		fmt.Println("[processAudio] error Getwd(), err: ", err)
		return
	}

	// TODO parameterize
	err = fileExists("tmp")
	if err != nil {
		os.Mkdir("tmp", 0777)
		fmt.Println("[processAudio] made tmp directory")
	}

	// Reading the segment into memory
	mdat, err := ioutil.ReadFile(segmentPath)
	if err != nil {
		fmt.Printf("[processAudio] failed to read file: ", err)
		return
	}

	// Reading the init segment into memory
	// TODO refactor so we don't assume the lowest quality level is 0, and the filename pattern
	initSegmentPath := fmt.Sprintf("%v/content/0/init_0.mp4", wd)
	init, err := ioutil.ReadFile(initSegmentPath)
	if err != nil {
		fmt.Printf("[processAudio] failed to read init segment: ", err)
		return
	}

	blob := append(init, mdat...)

	inputMP4Path := fmt.Sprintf("%v/tmp/input_%v.mp4", wd, segmentFilename)

	err = ioutil.WriteFile(inputMP4Path, blob, 0777)
	if err != nil {
		fmt.Printf("[processAudio] Could not write input_%v.mp4, err: %v \n", segmentFilename, err)
		return
	}

	// Extracting the audio stream from mp4 -> aac
	inputAACPath := fmt.Sprintf("%v/tmp/input_%v.aac", wd, segmentFilename)

	cmd := exec.Command(
		ffmpeg,
		"-i", inputMP4Path,
		"-vn",
		"-acodec", "copy",
		inputAACPath,
	)

	err = cmd.Run()
	if err != nil {
		fmt.Printf("[processAudio] Could not extract audio stream from path %v , err: %v \n", inputMP4Path, err)
		return
	}

	outputOGGPath := fmt.Sprintf("%v/tmp/output_%v.ogg", wd, segmentFilename)

	// lossy audio is required to be encoded with libopus, according to API docs
	cmd = exec.Command(
		ffmpeg,
		"-i", inputAACPath,
		"-acodec", "libopus",
		"-b:a", "64000", // 64k
		"-ar", "16000", // 16kHz (required)
		"-ac", "1",
		outputOGGPath,
	)

	err = cmd.Run()
	if err != nil {
		fmt.Println("[processAudio] Could not transcode audio stream from aac -> ogg opus: ", err)
		return
	}

	// Reads the audio file into memory.
	data, err := ioutil.ReadFile(outputOGGPath)
	if err != nil {
		fmt.Println("[processAudio] Failed to read file: ", err)
		return
	}

	ctx := context.Background()

	// Creates a client.
	client, err := speech.NewClient(ctx)
	if err != nil {
		fmt.Println("[processAudio] Could not create speech client: ", err)
		return
	}

	// Detects speech in the audio file.
	resp, err := client.Recognize(ctx, &speechpb.RecognizeRequest{
		// TODO parameterize
		Config: &speechpb.RecognitionConfig{
			Encoding:              speechpb.RecognitionConfig_OGG_OPUS,
			SampleRateHertz:       16000,
			LanguageCode:          "en-US",
			EnableWordTimeOffsets: true,
		},
		Audio: &speechpb.RecognitionAudio{
			AudioSource: &speechpb.RecognitionAudio_Content{Content: data},
		},
	})

	if err != nil {
		fmt.Println("[processAudio] Error transcribing audio data: ", err)
	} else {
		fmt.Println("[processAudio] Successfully transcribed audio for segment: ", segmentPath)
		writeTranscriptionForSegment(resp, segmentPath)
	}

	// cleanup(inputMP4Path, inputAACPath, outputOGGPath)
}

func writeTranscriptionForSegment(data *speechpb.RecognizeResponse, path string) error {
	fmt.Println("[writeTranscriptionForSegment] Transcription received: ", data)

	filepath := fmt.Sprintf("%v.json", path)
	raw, err := json.Marshal(data)
	if err != nil {
		fmt.Println("[writeTranscriptionForSegment] Could not convert speech response to byte array: ", err)
		return err
	}

	err = ioutil.WriteFile(filepath, raw, 0644)
	if err != nil {
		fmt.Printf("[writeTranscriptionForSegment] Could not write to path %v, error was %v \n", filepath, err)
		return err
	}

	return nil
}

func fileExists(path string) error {
	_, err := os.Stat(path)

	if os.IsNotExist(err) {
		return err
	}

	return nil
}

func cleanup() {
	// TODO cleanup out-of-window working files as
	//      it ensures the ability to re-encode from working files if
	//      a problem is encountered in the first pass
}
