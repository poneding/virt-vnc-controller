#!/bin/bash

# build binaries
go mod download
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o bin/linux_amd64/controller
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o bin/linux_arm64/controller

# build docker images
docker buildx create --name mybuilder --use
docker buildx build --push --platform linux/amd64,linux/arm64 -t poneding/virt-vnc-controller .

# clean up
rm -rf bin
