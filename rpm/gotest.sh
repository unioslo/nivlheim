#!/bin/bash
cd "$(dirname "$0")"
cd ../server/service
go get
go test -v
