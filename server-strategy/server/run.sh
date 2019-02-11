#!/bin/bash
export GOOGLE_APPLICATION_CREDENTIALS="/Users/mcunningham/Development/video-speech-recognition/gcp/service-account.json"
rm -rf content
mkdir content

rm -rf tmp
go run encoder.go