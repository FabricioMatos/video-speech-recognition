package main

import (
	"context"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/exec"

	speech "cloud.google.com/go/speech/apiv1"
	"github.com/gorilla/websocket"
	speechpb "google.golang.org/genproto/googleapis/cloud/speech/v1"
)

var wsupgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,

	// Disabling weird CORS stuff
	CheckOrigin: func(h *http.Request) bool {
		return true
	},
}

var addr = flag.String("addr", "localhost:9090", "http service address")

func main() {
	flag.Parse()
	http.HandleFunc("/speech-recognition", handler)
	log.Fatal(http.ListenAndServe(*addr, nil))
}

func handler(w http.ResponseWriter, r *http.Request) {
	c, err := wsupgrader.Upgrade(w, r, nil)
	if err != nil {
		fmt.Printf("Failed to set websocket upgrade: %+v", err)
		return
	}
	defer c.Close()
	for {
		_, message, err := c.ReadMessage()
		if err != nil {
			log.Println("read:", err)
			break
		}
		// log.Printf("recv: %s", message)
		processAudio(message, c)
		if err != nil {
			log.Println("write:", err)
			break
		}
	}
}

func processAudio(raw []byte, conn *websocket.Conn) {
	var err error

	err = fileExists("tmp")
	if err != nil {
		os.Mkdir("tmp", 0644)
		fmt.Println("Made tmp directory")
	}

	err = ioutil.WriteFile("tmp/input.mp4", raw, 0644)
	if err != nil {
		fmt.Println("Could not write input.mp4")
		return
	}

	ctx := context.Background()

	// Creates a client.
	client, err := speech.NewClient(ctx)
	if err != nil {
		fmt.Printf("Could not create speech client %v", err)
		return
	}

	filename := "tmp/input.mp4"

	// Extracting the audio stream from mp4 -> aac
	cmd := exec.Command(
		"./bin/ffmpeg.exe",
		"-i", filename,
		"-vn",
		"-acodec", "copy",
		"tmp/input.aac",
	)

	err = cmd.Run()
	if err != nil {
		fmt.Println("Could not extract audio stream from input.mp4")
		cleanup()
		return
	}

	filename = "tmp/input.aac"

	// lossy audio is required to be encoded with libopus, according to API docs
	cmd = exec.Command(
		"./bin/ffmpeg.exe",
		"-i", filename,
		"-acodec", "libopus",
		"-b:a", "64000", // 64k
		"-ar", "16000", // 16kHz (required)
		"-ac", "1",
		"tmp/output.ogg",
	)

	err = cmd.Run()
	if err != nil {
		fmt.Println("Could not transcode audio stream from aac -> ogg opus")
		cleanup()
		return
	}

	filename = "tmp/output.ogg"

	// Reads the audio file into memory.
	data, err := ioutil.ReadFile(filename)
	if err != nil {
		fmt.Printf("Failed to read file: %v", err)
		cleanup()
		return
	}

	// Detects speech in the audio file.
	resp, err := client.Recognize(ctx, &speechpb.RecognizeRequest{
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
		fmt.Printf("Error transcribing audio data %v", err)
		cleanup()
		return
	}

	log.Println("Successfully transcribed audio")
	cleanup()
	conn.WriteJSON(resp)
}

func fileExists(path string) error {
	_, err := os.Stat(path)

	if os.IsNotExist(err) {
		return err
	}

	return nil
}

func cleanup() {
	_ = os.Remove("tmp/input.mp4")
	fmt.Println("Removed input.mp4")

	_ = os.Remove("tmp/input.aac")
	fmt.Println("Removed input.aac")

	_ = os.Remove("tmp/output.ogg")
	fmt.Println("Removed input.ogg")
}
