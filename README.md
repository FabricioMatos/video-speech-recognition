# Live Adaptive Video Speech Recognition

This is a PoC that demonstrates different end-to-end implementations of auto-generated subtitles sourced from chunks of MP4 video data.

## Strategies
- There are examples of two implementations here - a client-focused one and a server-focused one.

**The recommended strategy is a server-focused one** as it requires less processing and bandwidth usage for all parties.

A server-focused strategy is one that has direct access to the encoder or only its output for augmentation. Audio data is retrieved directly from the encoder output where it is then sent for transcription. Delivery of the transcripts can be performed in a number of ways, the most spec-compliant way would be live `WebVTT` segments.

A client-focused strategy can be implemented on any playback source but still has a small backend component in play. Audio data is sent from the client's browser to the backend component where it is then sent for transcription. Once the timed transcription is recieved, it is then translated to `WebVTT` cues to allow native rendering capabilities offered by the browser.

See *Known Issues* below for details on current limitations/issues

## Instructions
It's required to have a GCP service account setup
https://cloud.google.com/video-intelligence/docs/common/auth

Tested on macOS 10.14 Mojave w/ `ffmpeg` 4.1

- Place `ffmpeg` binary in a new directory `bin`
- Ensure path/filename is correct in scripts
- Ensure Golang code dependencies are resolved
- Retrieve a GCP service account and place under `gcp`
- Ensure path to GCP service account json file is correct in scripts

- For client strategy:
  - Invoke `./run.sh` in both `client` and `server` directories

- For server strategy:
  - Provide playback source URL in `server/encoder.go`
  - Invoke `./run.sh` in strategy root and `run.sh`  in `server` directory

## Tech Used
- [hls.js](https://github.com/video-dev/hls.js) for browser HLS playback, and used to access remuxed chunks of mp4 data (for client-focused strategy)
- Golang for backend components
- FFmpeg for video transcoding
- Google Cloud Speech Recognition - receives the chunks of mp4 data for transcription

## Known Issues
- (example) Client Strategy - synchronization of translated `WebVTT` cues to timing in video may be offset temporarily

## Roadmap
- Live WebVTT support for server strategy example, currently using a custom delivery method for initial phase.
Pending resolution of: https://github.com/jwplayer/hls.js/pull/192
- Pass data with `ffmpeg` via stdin/out rather than writing/reading to disk
- Integrate [Mozilla DeepSpeech](https://github.com/mozilla/DeepSpeech) provider, [pending work on exposing timed word offsets in audio](https://discourse.mozilla.org/t/speech-to-text-json-result-with-time-per-word/32681)

## Notes
- You may need to tweak the encoding settings for compatibility and/or optimal performance
