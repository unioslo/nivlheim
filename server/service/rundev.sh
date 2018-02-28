#!/bin/bash
go run $(ls -1 *.go|grep -v test) --dev
