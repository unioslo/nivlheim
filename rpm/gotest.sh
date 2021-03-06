#!/bin/bash
cd "$(dirname "$0")"
cd ../server/service
if [[ `pwd` == *"src/github"* ]]; then
    # Looks like there is a Go folder structure in place
    # ( $GOPATH/src/github.com/unioslo/nivlheim/... )
    go get
    go test -v
    exit
fi
# Move things into a Go folder structure
scratch=$(mktemp -d -t tmp.XXXXXXXXXX)
function finish {
    rm -rf "$scratch"
}
trap finish EXIT
mkdir -p $scratch/src/github.com/unioslo/nivlheim/server
ln -s -t $scratch/src/github.com/unioslo/nivlheim/server "`pwd`"
export GOPATH="$scratch"
export GOBIN="$GOPATH/bin"
cd $GOPATH
go get github.com/unioslo/nivlheim/server/service
go test -v github.com/unioslo/nivlheim/server/service
